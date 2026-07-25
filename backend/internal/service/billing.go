package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/logctx"
)

// ErrAlreadySubscribed は既にプレミアムの利用者が加入を試みたことを表す。
var ErrAlreadySubscribed = errors.New("既にプレミアムに加入しています")

// ErrWebhookSignature は Webhook の署名検証・本文解釈に失敗したことを表す。
// handler はこれを 400 に写し、Stripe に再送させない。
var ErrWebhookSignature = errors.New("Webフックの検証に失敗しました")

// planManagementPath は解約導線の文言。特商法・利用規約が名指しする画面名。
const planManagementPath = "アカウント設定 > プランの管理"

// PreviewResult は申込確認画面の表示値。
type PreviewResult struct {
	Price              int
	Currency           string
	TrialDays          int
	TrialEligible      bool
	FirstBillingAt     time.Time
	PlanManagementPath string
}

// BillingService は加入の申込導線を担う。加入状態の権威は Webhook（HandleWebhook）。
type BillingService struct {
	entitlements Entitlements
	store        SubscriptionStore
	gateway      PaymentGateway
	successURL   string
	cancelURL    string
	trialDays    int
	now          func() time.Time
}

// NewBillingService は BillingService を生成する。
func NewBillingService(
	entitlements Entitlements, store SubscriptionStore, gateway PaymentGateway,
	successURL, cancelURL string, trialDays int, now func() time.Time,
) *BillingService {
	if now == nil {
		now = time.Now
	}
	return &BillingService{
		entitlements: entitlements, store: store, gateway: gateway,
		successURL: successURL, cancelURL: cancelURL, trialDays: trialDays, now: now,
	}
}

// Preview は申込確認画面の表示値を返す。
func (s *BillingService) Preview(ctx context.Context, userID string) (PreviewResult, error) {
	eligible, _, err := s.trialEligibility(ctx, userID)
	if err != nil {
		return PreviewResult{}, err
	}
	first := s.now()
	if eligible {
		first = first.Add(time.Duration(s.trialDays) * 24 * time.Hour)
	}
	return PreviewResult{
		Price: 300, Currency: "jpy", TrialDays: s.trialDays,
		TrialEligible: eligible, FirstBillingAt: first,
		PlanManagementPath: planManagementPath,
	}, nil
}

// CreateCheckoutSession は Checkout セッションを作り URL を返す。
//
// 二重加入防止: 既存の加入行があり、それが canceled でなければ
// （active/trialing/past_due のいずれでも）Stripe 側の加入がまだ生きている
// ということなので、たとえ entitlement が既に free に落ちていても
// （past_due が猶予を超えた場合など）再申込を許さない。二重課金を防ぐため。
// entitlement==premium のチェックは無くても上記だけで十分だが、
// 安全側の保険としてそのまま残す。
func (s *BillingService) CreateCheckoutSession(ctx context.Context, userID string) (string, error) {
	ent, err := s.entitlements.For(ctx, userID)
	if err != nil {
		return "", err
	}
	if ent.Plan() == domain.PlanPremium {
		return "", ErrAlreadySubscribed
	}
	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return "", err
	}
	sub, err := s.store.Find(ctx, uid)
	eligible := true
	customerID := ""
	switch {
	case err == nil:
		if sub.Status != domain.SubscriptionCanceled {
			return "", ErrAlreadySubscribed
		}
		eligible = false
		customerID = sub.ProviderCustomerID
	case errors.Is(err, ErrSubscriptionNotFound):
		// 加入行が一度も無い＝初回。トライアル適格。
	default:
		return "", err
	}
	return s.gateway.CreateCheckoutSession(ctx, CheckoutParams{
		UserID:     userID,
		CustomerID: customerID,
		WithTrial:  eligible,
		SuccessURL: s.successURL,
		CancelURL:  s.cancelURL,
	})
}

// trialEligibility は「トライアル適格か」と「再利用する Customer ID」を返す。
// 加入行が一度も無ければ適格（初回）。行があれば非適格で、その顧客IDを再利用する。
func (s *BillingService) trialEligibility(ctx context.Context, userID string) (bool, string, error) {
	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return false, "", err
	}
	sub, err := s.store.Find(ctx, uid)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return true, "", nil
		}
		return false, "", err
	}
	return false, sub.ProviderCustomerID, nil
}

// HandleWebhook は Webhook を検証し、加入状態を subscriptions に同期する。
func (s *BillingService) HandleWebhook(ctx context.Context, payload []byte, sigHeader string) error {
	ev, err := s.gateway.ParseWebhookEvent(payload, sigHeader)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrWebhookSignature, err)
	}
	if ev.Type == "" {
		return nil // 対象外イベント
	}
	if ev.UserID == "" {
		// 紐付けられない＝こちらの不具合。再送しても直らないのでログして無視する。
		logctx.From(ctx).WarnContext(ctx, "Webフックに user_id が無いため無視します",
			slog.String("subscription_id", ev.SubscriptionID))
		return nil
	}
	uid, err := domain.ParseUserID(ev.UserID)
	if err != nil {
		logctx.From(ctx).WarnContext(ctx, "Webフックの user_id が不正なため無視します",
			slog.String("user_id", ev.UserID))
		return nil
	}

	// 古い/別サブスクのイベントによる上書き防止（out-of-order 対策）。
	// 現在の行が「別の」サブスクで、かつそれが今この瞬間プレミアムを与えている
	// （＝新しく加入し直した後）なら、遅れて届いた古いイベントで上書きしない。
	// 手動付与（ProviderSubscriptionID 空）は対象外＝Stripeイベントに乗っ取られてよい。
	existing, err := s.store.Find(ctx, uid)
	if err != nil && !errors.Is(err, ErrSubscriptionNotFound) {
		return err
	}
	existingFound := err == nil
	if existingFound &&
		existing.ProviderSubscriptionID != "" &&
		existing.ProviderSubscriptionID != ev.SubscriptionID &&
		existing.GivesPremiumAt(s.now()) {
		logctx.From(ctx).WarnContext(ctx,
			"古い/別サブスクのイベントのため無視します（現行の有効な加入を保護）",
			slog.String("user_id", ev.UserID),
			slog.String("incoming_subscription_id", ev.SubscriptionID),
			slog.String("existing_subscription_id", existing.ProviderSubscriptionID))
		return nil
	}

	if err := s.store.Upsert(ctx, domain.Subscription{
		UserID:                 uid,
		Plan:                   domain.PlanPremium,
		Status:                 ev.Status,
		CurrentPeriodEnd:       ev.CurrentPeriodEnd,
		CancelAtPeriodEnd:      ev.CancelAtPeriodEnd,
		Provider:               domain.ProviderStripe,
		ProviderSubscriptionID: ev.SubscriptionID,
		ProviderCustomerID:     ev.CustomerID,
	}); err != nil {
		// ここが失敗すると PAID な加入がDBに同期されず、handler は 500 を返す
		// （Stripe が再送する）。診断できるよう原因をログに残す。
		logctx.From(ctx).ErrorContext(ctx, "Webフックの加入更新（Upsert）に失敗しました",
			slog.String("subscription_id", ev.SubscriptionID),
			slog.String("user_id", ev.UserID),
			slog.Any("error", err))
		return err
	}
	return nil
}
