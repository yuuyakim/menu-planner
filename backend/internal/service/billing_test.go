package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// --- fakes ---

type fakeStore struct {
	sub      domain.Subscription
	found    bool
	upserted *domain.Subscription
}

func (f *fakeStore) Find(_ context.Context, _ domain.UserID) (domain.Subscription, error) {
	if !f.found {
		return domain.Subscription{}, service.ErrSubscriptionNotFound
	}
	return f.sub, nil
}
func (f *fakeStore) Upsert(_ context.Context, sub domain.Subscription) error {
	f.upserted = &sub
	return nil
}

type fakeEnt struct{ plan domain.Plan }

func (f fakeEnt) For(_ context.Context, _ string) (domain.Entitlement, error) {
	return domain.NewEntitlement(f.plan), nil
}

type fakeGateway struct {
	lastParams service.CheckoutParams
	url        string
	event      service.WebhookEvent
	parseErr   error
}

func (f *fakeGateway) CreateCheckoutSession(_ context.Context, p service.CheckoutParams) (string, error) {
	f.lastParams = p
	return f.url, nil
}
func (f *fakeGateway) ParseWebhookEvent(_ []byte, _ string) (service.WebhookEvent, error) {
	return f.event, f.parseErr
}

const validUID = "11111111-1111-1111-1111-111111111111"

func newBilling(store service.SubscriptionStore, ent service.Entitlements, gw service.PaymentGateway) *service.BillingService {
	now := func() time.Time { return time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC) }
	return service.NewBillingService(ent, store, gw, "https://app/checkout/complete", "https://app/checkout", 5, now)
}

// --- tests ---

func TestBilling_CreateCheckoutSession_AlreadyPremium(t *testing.T) {
	svc := newBilling(&fakeStore{}, fakeEnt{plan: domain.PlanPremium}, &fakeGateway{})
	_, err := svc.CreateCheckoutSession(context.Background(), validUID)
	if !errors.Is(err, service.ErrAlreadySubscribed) {
		t.Fatalf("premium は already-subscribed。got %v", err)
	}
}

func TestBilling_CreateCheckoutSession_FirstTimeGetsTrial(t *testing.T) {
	gw := &fakeGateway{url: "https://stripe/session"}
	svc := newBilling(&fakeStore{found: false}, fakeEnt{plan: domain.PlanFree}, gw)
	url, err := svc.CreateCheckoutSession(context.Background(), validUID)
	if err != nil {
		t.Fatalf("CreateCheckoutSession: %v", err)
	}
	if url != "https://stripe/session" {
		t.Errorf("url = %q", url)
	}
	if !gw.lastParams.WithTrial {
		t.Error("初回はトライアル付与すべき")
	}
	if gw.lastParams.CustomerID != "" {
		t.Error("初回は Customer 再利用しない")
	}
}

func TestBilling_CreateCheckoutSession_ReturningReusesCustomerNoTrial(t *testing.T) {
	store := &fakeStore{found: true, sub: domain.Subscription{
		Status: domain.SubscriptionCanceled, ProviderCustomerID: "cus_old",
	}}
	gw := &fakeGateway{url: "https://stripe/session"}
	svc := newBilling(store, fakeEnt{plan: domain.PlanFree}, gw)
	if _, err := svc.CreateCheckoutSession(context.Background(), validUID); err != nil {
		t.Fatalf("CreateCheckoutSession: %v", err)
	}
	if gw.lastParams.WithTrial {
		t.Error("再申込はトライアル無し")
	}
	if gw.lastParams.CustomerID != "cus_old" {
		t.Errorf("Customer を再利用すべき。got %q", gw.lastParams.CustomerID)
	}
}

func TestBilling_Preview_FirstTime(t *testing.T) {
	svc := newBilling(&fakeStore{found: false}, fakeEnt{plan: domain.PlanFree}, &fakeGateway{})
	got, err := svc.Preview(context.Background(), validUID)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if !got.TrialEligible || got.Price != 300 || got.TrialDays != 5 {
		t.Errorf("preview 不正: %+v", got)
	}
	want := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC) // now + 5日
	if !got.FirstBillingAt.Equal(want) {
		t.Errorf("FirstBillingAt = %v, want %v", got.FirstBillingAt, want)
	}
}

func TestBilling_HandleWebhook_UpsertsSubscription(t *testing.T) {
	store := &fakeStore{}
	gw := &fakeGateway{event: service.WebhookEvent{
		Type: "subscription", UserID: validUID, SubscriptionID: "sub_1",
		CustomerID: "cus_1", Status: domain.SubscriptionActive,
		CurrentPeriodEnd: time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
	}}
	svc := newBilling(store, fakeEnt{plan: domain.PlanFree}, gw)
	if err := svc.HandleWebhook(context.Background(), []byte("{}"), "sig"); err != nil {
		t.Fatalf("HandleWebhook: %v", err)
	}
	if store.upserted == nil {
		t.Fatal("upsert されていない")
	}
	if store.upserted.Provider != domain.ProviderStripe ||
		store.upserted.Plan != domain.PlanPremium ||
		store.upserted.ProviderSubscriptionID != "sub_1" ||
		store.upserted.ProviderCustomerID != "cus_1" {
		t.Errorf("upsert 内容が不正: %+v", *store.upserted)
	}
}

func TestBilling_HandleWebhook_SignatureError(t *testing.T) {
	gw := &fakeGateway{parseErr: errors.New("bad sig")}
	svc := newBilling(&fakeStore{}, fakeEnt{plan: domain.PlanFree}, gw)
	err := svc.HandleWebhook(context.Background(), []byte("{}"), "sig")
	if !errors.Is(err, service.ErrWebhookSignature) {
		t.Fatalf("署名エラーは ErrWebhookSignature。got %v", err)
	}
}

func TestBilling_HandleWebhook_IgnoredEvent(t *testing.T) {
	store := &fakeStore{}
	gw := &fakeGateway{event: service.WebhookEvent{Type: ""}}
	svc := newBilling(store, fakeEnt{plan: domain.PlanFree}, gw)
	if err := svc.HandleWebhook(context.Background(), []byte("{}"), "sig"); err != nil {
		t.Fatalf("HandleWebhook: %v", err)
	}
	if store.upserted != nil {
		t.Error("対象外イベントは upsert しない")
	}
}
