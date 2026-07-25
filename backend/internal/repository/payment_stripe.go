package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/checkout/session"
	"github.com/stripe/stripe-go/v86/subscription"
	"github.com/stripe/stripe-go/v86/webhook"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// StripePaymentGateway は service.PaymentGateway の Stripe 実装。
// stripe-go への依存はこのファイルだけに閉じ込める。
type StripePaymentGateway struct {
	webhookSecret string
	priceID       string
	trialDays     int64
}

// NewStripePaymentGateway は Stripe の秘密鍵を設定し gateway を生成する。
func NewStripePaymentGateway(secretKey, webhookSecret, priceID string, trialDays int64) *StripePaymentGateway {
	stripe.Key = secretKey
	return &StripePaymentGateway{webhookSecret: webhookSecret, priceID: priceID, trialDays: trialDays}
}

func (g *StripePaymentGateway) CreateCheckoutSession(ctx context.Context, p service.CheckoutParams) (string, error) {
	params := &stripe.CheckoutSessionParams{
		Mode:              stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		SuccessURL:        stripe.String(p.SuccessURL),
		CancelURL:         stripe.String(p.CancelURL),
		ClientReferenceID: stripe.String(p.UserID),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{Price: stripe.String(g.priceID), Quantity: stripe.Int64(1)},
		},
		SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
			Metadata: map[string]string{"user_id": p.UserID},
		},
	}
	if p.WithTrial {
		params.SubscriptionData.TrialPeriodDays = stripe.Int64(g.trialDays)
	}
	if p.CustomerID != "" {
		params.Customer = stripe.String(p.CustomerID)
	}
	params.Context = ctx

	s, err := session.New(params)
	if err != nil {
		return "", fmt.Errorf("Checkout セッションの作成に失敗しました: %w", err)
	}
	return s.URL, nil
}

func (g *StripePaymentGateway) ParseWebhookEvent(payload []byte, sigHeader string) (service.WebhookEvent, error) {
	event, err := webhook.ConstructEvent(payload, sigHeader, g.webhookSecret)
	if err != nil {
		return service.WebhookEvent{}, fmt.Errorf("Webhook 署名の検証に失敗しました: %w", err)
	}

	switch event.Type {
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			return service.WebhookEvent{}, fmt.Errorf("subscription の解釈に失敗しました: %w", err)
		}
		return normalizeSubscription(&sub, event.Type == "customer.subscription.deleted"), nil

	case "checkout.session.completed":
		var cs stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &cs); err != nil {
			return service.WebhookEvent{}, fmt.Errorf("checkout session の解釈に失敗しました: %w", err)
		}
		if cs.Subscription == nil || cs.Subscription.ID == "" {
			return service.WebhookEvent{}, nil // 加入を伴わない（想定外）→無視
		}
		// session は加入の詳細を含まないため取得する（subscription.* を取りこぼした場合の保険）。
		sub, err := subscription.Get(cs.Subscription.ID, nil)
		if err != nil {
			return service.WebhookEvent{}, fmt.Errorf("subscription の取得に失敗しました: %w", err)
		}
		ev := normalizeSubscription(sub, false)
		if ev.UserID == "" {
			ev.UserID = cs.ClientReferenceID
		}
		return ev, nil

	default:
		return service.WebhookEvent{}, nil // 対象外→無視
	}
}

// normalizeSubscription は Stripe の Subscription を WebhookEvent に写す。
func normalizeSubscription(sub *stripe.Subscription, deleted bool) service.WebhookEvent {
	ev := service.WebhookEvent{
		Type:              "subscription",
		UserID:            sub.Metadata["user_id"],
		SubscriptionID:    sub.ID,
		CancelAtPeriodEnd: sub.CancelAtPeriodEnd,
	}
	if sub.Customer != nil {
		ev.CustomerID = sub.Customer.ID
	}
	// current_period_end は加入 item 側にある（Stripe API の変更）。
	if sub.Items != nil && len(sub.Items.Data) > 0 {
		ev.CurrentPeriodEnd = time.Unix(sub.Items.Data[0].CurrentPeriodEnd, 0).UTC()
	}
	if deleted {
		ev.Status = domain.SubscriptionCanceled
	} else {
		ev.Status = mapStripeStatus(sub.Status)
	}
	return ev
}

// mapStripeStatus は Stripe の status をドメインの状態に写す。
// active / trialing / past_due 以外はすべてアクセスを与えない側（canceled）に倒す。
func mapStripeStatus(s stripe.SubscriptionStatus) domain.SubscriptionStatus {
	switch s {
	case stripe.SubscriptionStatusActive:
		return domain.SubscriptionActive
	case stripe.SubscriptionStatusTrialing:
		return domain.SubscriptionTrialing
	case stripe.SubscriptionStatusPastDue:
		return domain.SubscriptionPastDue
	default:
		return domain.SubscriptionCanceled
	}
}
