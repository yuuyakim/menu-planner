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
func (s *BillingService) CreateCheckoutSession(ctx context.Context, userID string) (string, error) {
	ent, err := s.entitlements.For(ctx, userID)
	if err != nil {
		return "", err
	}
	if ent.Plan() == domain.PlanPremium {
		return "", ErrAlreadySubscribed
	}
	eligible, customerID, err := s.trialEligibility(ctx, userID)
	if err != nil {
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
		return fmt.Errorf("%w: %v", ErrWebhookSignature, err)
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
	return s.store.Upsert(ctx, domain.Subscription{
		UserID:                 uid,
		Plan:                   domain.PlanPremium,
		Status:                 ev.Status,
		CurrentPeriodEnd:       ev.CurrentPeriodEnd,
		CancelAtPeriodEnd:      ev.CancelAtPeriodEnd,
		Provider:               domain.ProviderStripe,
		ProviderSubscriptionID: ev.SubscriptionID,
		ProviderCustomerID:     ev.CustomerID,
	})
}
