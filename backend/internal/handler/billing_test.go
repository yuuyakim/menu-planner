package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/handler"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// fakeBilling は handler.BillingUseCase を満たす。
type fakeBilling struct {
	previewErr error

	url       string
	createErr error

	handleErr    error
	handleCalled bool

	view      service.SubscriptionView
	viewErr   error
	portalURL string
	portalErr error
}

func (f *fakeBilling) Preview(context.Context, string) (service.PreviewResult, error) {
	return service.PreviewResult{
		Price: 300, Currency: "jpy", TrialDays: 5, TrialEligible: true,
		PlanManagementPath: "アカウント設定 > プランの管理",
	}, f.previewErr
}

func (f *fakeBilling) CreateCheckoutSession(context.Context, string) (string, error) {
	return f.url, f.createErr
}

func (f *fakeBilling) HandleWebhook(_ context.Context, _ []byte, _ string) error {
	f.handleCalled = true
	return f.handleErr
}

func (f *fakeBilling) Subscription(context.Context, string) (service.SubscriptionView, error) {
	return f.view, f.viewErr
}

func (f *fakeBilling) CreatePortalSession(context.Context, string) (string, error) {
	return f.portalURL, f.portalErr
}

// billingApp は BillingHandler のみを積んだ echo アプリを組み立てる。
// saved_shopping_list_test.go の savedShoppingListApp に倣う。
func billingApp(t *testing.T, svc handler.BillingUseCase) (*echo.Echo, *auth.JWT) {
	t.Helper()
	tokens, err := auth.NewJWT([]byte(authTestSecret))
	require.NoError(t, err)
	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewBillingHandler(svc, tokens).RegisterRoutes(e)
	return e, tokens
}

// doBillingRequest は Cookie の有無を選べる汎用ヘルパ。access が空なら未認証。
func doBillingRequest(
	t *testing.T, e *echo.Echo, method, path, access, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	var r *strings.Reader
	if body == "" {
		r = strings.NewReader("")
	} else {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	if body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	if access != "" {
		req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestBillingHandler_CheckoutSession_RequiresAuth(t *testing.T) {
	t.Parallel()

	svc := &fakeBilling{}
	e, _ := billingApp(t, svc)

	rec := doBillingRequest(t, e, http.MethodPost, "/api/v1/billing/checkout-session", "", "")

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestBillingHandler_CheckoutSession_AlreadySubscribed(t *testing.T) {
	t.Parallel()

	svc := &fakeBilling{createErr: service.ErrAlreadySubscribed}
	e, tokens := billingApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := doBillingRequest(t, e, http.MethodPost, "/api/v1/billing/checkout-session", access, "")

	require.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "already-subscribed")
}

func TestBillingHandler_CheckoutSession_ReturnsURL(t *testing.T) {
	t.Parallel()

	svc := &fakeBilling{url: "https://stripe/x"}
	e, tokens := billingApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := doBillingRequest(t, e, http.MethodPost, "/api/v1/billing/checkout-session", access, "")

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		URL string `json:"url"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "https://stripe/x", body.URL)
}

func TestBillingHandler_Preview_RequiresAuth(t *testing.T) {
	t.Parallel()

	svc := &fakeBilling{}
	e, _ := billingApp(t, svc)

	rec := doBillingRequest(t, e, http.MethodGet, "/api/v1/billing/preview", "", "")

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestBillingHandler_Preview_200(t *testing.T) {
	t.Parallel()

	svc := &fakeBilling{}
	e, tokens := billingApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := doBillingRequest(t, e, http.MethodGet, "/api/v1/billing/preview", access, "")

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Price              int    `json:"price"`
		Currency           string `json:"currency"`
		TrialDays          int    `json:"trialDays"`
		TrialEligible      bool   `json:"trialEligible"`
		FirstBillingAt     string `json:"firstBillingAt"`
		PlanManagementPath string `json:"planManagementPath"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, 300, body.Price)
	assert.Equal(t, "jpy", body.Currency)
	assert.Equal(t, 5, body.TrialDays)
	assert.True(t, body.TrialEligible)
	assert.NotEmpty(t, body.FirstBillingAt)
	assert.Equal(t, "アカウント設定 > プランの管理", body.PlanManagementPath)
}

func TestBillingHandler_Subscription_RequiresAuth(t *testing.T) {
	t.Parallel()

	svc := &fakeBilling{}
	e, _ := billingApp(t, svc)

	rec := doBillingRequest(t, e, http.MethodGet, "/api/v1/billing/subscription", "", "")

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestBillingHandler_Subscription_ReturnsView(t *testing.T) {
	t.Parallel()

	svc := &fakeBilling{view: service.SubscriptionView{
		Plan: "premium", Status: "active", HasPortal: true,
	}}
	e, tokens := billingApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := doBillingRequest(t, e, http.MethodGet, "/api/v1/billing/subscription", access, "")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "premium")
	assert.Contains(t, rec.Body.String(), "active")
	assert.Contains(t, rec.Body.String(), `"hasPortal":true`)
}

func TestBillingHandler_PortalSession_NoBillingCustomer(t *testing.T) {
	t.Parallel()

	svc := &fakeBilling{portalErr: service.ErrNoBillingCustomer}
	e, tokens := billingApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := doBillingRequest(t, e, http.MethodPost, "/api/v1/billing/portal-session", access, "")

	require.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "no-billing-customer")
}

func TestBillingHandler_PortalSession_ReturnsURL(t *testing.T) {
	t.Parallel()

	svc := &fakeBilling{portalURL: "https://stripe/portal"}
	e, tokens := billingApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := doBillingRequest(t, e, http.MethodPost, "/api/v1/billing/portal-session", access, "")

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		URL string `json:"url"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "https://stripe/portal", body.URL)
}

func TestBillingHandler_Webhook_BadSignature(t *testing.T) {
	t.Parallel()

	svc := &fakeBilling{handleErr: fmt.Errorf("%w: x", service.ErrWebhookSignature)}
	e, _ := billingApp(t, svc)

	rec := doBillingRequest(t, e, http.MethodPost, "/api/v1/billing/webhook", "", `{"type":"x"}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.True(t, svc.handleCalled)
}

func TestBillingHandler_Webhook_OK(t *testing.T) {
	t.Parallel()

	svc := &fakeBilling{}
	e, _ := billingApp(t, svc)

	rec := doBillingRequest(t, e, http.MethodPost, "/api/v1/billing/webhook", "", `{"type":"x"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, svc.handleCalled)
}

func TestBillingHandler_Webhook_ProcessingError(t *testing.T) {
	t.Parallel()

	svc := &fakeBilling{handleErr: errors.New("db down")}
	e, _ := billingApp(t, svc)

	rec := doBillingRequest(t, e, http.MethodPost, "/api/v1/billing/webhook", "", `{"type":"x"}`)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.True(t, svc.handleCalled)
}
