package repository_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/webhook"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

const testWebhookSecret = "whsec_test_secret"

// signedEvent は指定のイベント種別と本文で署名済みペイロードを作る。
func signedEvent(t *testing.T, eventType string, obj any) ([]byte, string) {
	t.Helper()
	raw, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	event := map[string]any{
		"id":          "evt_test",
		"object":      "event",
		"api_version": stripe.APIVersion,
		"type":        eventType,
		"data":        map[string]any{"object": json.RawMessage(raw)},
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: payload, Secret: testWebhookSecret})
	return signed.Payload, signed.Header
}

func TestStripeGateway_ParseWebhookEvent_SubscriptionUpdated(t *testing.T) {
	gw := repository.NewStripePaymentGateway("sk_test", testWebhookSecret, "price_test", 5)
	periodEnd := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)

	sub := map[string]any{
		"id":                   "sub_123",
		"status":               "trialing",
		"cancel_at_period_end": false,
		"customer":             map[string]any{"id": "cus_123"},
		"metadata":             map[string]any{"user_id": "user-abc"},
		"items": map[string]any{
			"data": []map[string]any{{"current_period_end": periodEnd.Unix()}},
		},
	}
	payload, header := signedEvent(t, "customer.subscription.updated", sub)

	ev, err := gw.ParseWebhookEvent(payload, header)
	if err != nil {
		t.Fatalf("ParseWebhookEvent: %v", err)
	}
	if ev.Type != "subscription" {
		t.Errorf("Type = %q, want subscription", ev.Type)
	}
	if ev.UserID != "user-abc" || ev.SubscriptionID != "sub_123" || ev.CustomerID != "cus_123" {
		t.Errorf("紐付けが不正: %+v", ev)
	}
	if ev.Status != domain.SubscriptionTrialing {
		t.Errorf("Status = %q, want trialing", ev.Status)
	}
	if !ev.CurrentPeriodEnd.Equal(periodEnd) {
		t.Errorf("CurrentPeriodEnd = %v, want %v", ev.CurrentPeriodEnd, periodEnd)
	}
}

func TestStripeGateway_ParseWebhookEvent_SubscriptionDeleted(t *testing.T) {
	gw := repository.NewStripePaymentGateway("sk_test", testWebhookSecret, "price_test", 5)
	sub := map[string]any{
		"id": "sub_123", "status": "active",
		"metadata": map[string]any{"user_id": "user-abc"},
		"items":    map[string]any{"data": []map[string]any{{"current_period_end": int64(0)}}},
	}
	payload, header := signedEvent(t, "customer.subscription.deleted", sub)

	ev, err := gw.ParseWebhookEvent(payload, header)
	if err != nil {
		t.Fatalf("ParseWebhookEvent: %v", err)
	}
	if ev.Status != domain.SubscriptionCanceled {
		t.Errorf("deleted は canceled にする。got %q", ev.Status)
	}
}

func TestStripeGateway_ParseWebhookEvent_BadSignature(t *testing.T) {
	gw := repository.NewStripePaymentGateway("sk_test", testWebhookSecret, "price_test", 5)
	if _, err := gw.ParseWebhookEvent([]byte(`{}`), "t=1,v1=deadbeef"); err == nil {
		t.Error("署名不正はエラーにすべき")
	}
}

func TestStripeGateway_ParseWebhookEvent_IgnoredType(t *testing.T) {
	gw := repository.NewStripePaymentGateway("sk_test", testWebhookSecret, "price_test", 5)
	payload, header := signedEvent(t, "invoice.created", map[string]any{"id": "in_1"})
	ev, err := gw.ParseWebhookEvent(payload, header)
	if err != nil {
		t.Fatalf("ParseWebhookEvent: %v", err)
	}
	if ev.Type != "" {
		t.Errorf("対象外イベントは Type 空にする。got %q", ev.Type)
	}
}
