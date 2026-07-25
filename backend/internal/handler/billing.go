package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// BillingUseCase は課金APIが必要とする操作。実装は service.BillingService。
type BillingUseCase interface {
	Preview(ctx context.Context, userID string) (service.PreviewResult, error)
	CreateCheckoutSession(ctx context.Context, userID string) (string, error)
	HandleWebhook(ctx context.Context, payload []byte, sigHeader string) error
	Subscription(ctx context.Context, userID string) (service.SubscriptionView, error)
	CreatePortalSession(ctx context.Context, userID string) (string, error)
}

// BillingHandler は課金APIの受け口。
type BillingHandler struct {
	svc    BillingUseCase
	tokens *auth.JWT
}

// NewBillingHandler は BillingHandler を生成する。
func NewBillingHandler(svc BillingUseCase, tokens *auth.JWT) *BillingHandler {
	return &BillingHandler{svc: svc, tokens: tokens}
}

// RegisterRoutes はルーティングを登録する。
// preview / checkout-session / subscription / portal-session は本人の情報を
// 扱うため認証必須。
// webhook は Stripe が直接叩くため認証を付けず、署名検証で守る。
func (h *BillingHandler) RegisterRoutes(e *echo.Echo) {
	g := e.Group(APIBasePath)
	requireAuth := RequireAuth(h.tokens)
	g.GET("/billing/preview", h.Preview, requireAuth)
	g.POST("/billing/checkout-session", h.CreateCheckoutSession, requireAuth)
	g.POST("/billing/webhook", h.Webhook)
	g.GET("/billing/subscription", h.Subscription, requireAuth)
	g.POST("/billing/portal-session", h.PortalSession, requireAuth)
}

// previewDTO は申込確認画面の表示値。
type previewDTO struct {
	Price              int    `json:"price"`
	Currency           string `json:"currency"`
	TrialDays          int    `json:"trialDays"`
	TrialEligible      bool   `json:"trialEligible"`
	FirstBillingAt     string `json:"firstBillingAt"`
	PlanManagementPath string `json:"planManagementPath"`
}

// Preview は申込確認画面の表示値を返す。
//
//	GET /api/v1/billing/preview
//
// 未認証は401。
func (h *BillingHandler) Preview(c echo.Context) error {
	userID, ok := UserIDFromContext(c)
	if !ok {
		return auth.ErrTokenInvalid
	}

	p, err := h.svc.Preview(c.Request().Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, previewDTO{
		Price:              p.Price,
		Currency:           p.Currency,
		TrialDays:          p.TrialDays,
		TrialEligible:      p.TrialEligible,
		FirstBillingAt:     p.FirstBillingAt.Format(time.RFC3339),
		PlanManagementPath: p.PlanManagementPath,
	})
}

// CreateCheckoutSession は Checkout セッションを作り URL を返す。
//
//	POST /api/v1/billing/checkout-session
//
// 既にプレミアムの利用者が試みると409、未認証は401。
func (h *BillingHandler) CreateCheckoutSession(c echo.Context) error {
	userID, ok := UserIDFromContext(c)
	if !ok {
		return auth.ErrTokenInvalid
	}

	url, err := h.svc.CreateCheckoutSession(c.Request().Context(), userID)
	if err != nil {
		return err // ErrAlreadySubscribed は problem マッピングで 409
	}
	return c.JSON(http.StatusOK, map[string]string{"url": url})
}

// subscriptionDTO はプラン管理画面の表示値。
type subscriptionDTO struct {
	Plan              string  `json:"plan"`
	Status            string  `json:"status"`
	CurrentPeriodEnd  *string `json:"currentPeriodEnd"`
	CancelAtPeriodEnd bool    `json:"cancelAtPeriodEnd"`
	HasPortal         bool    `json:"hasPortal"`
}

// Subscription は現在のプラン状態を返す。
//
//	GET /api/v1/billing/subscription
//
// 未認証は401。
func (h *BillingHandler) Subscription(c echo.Context) error {
	userID, ok := UserIDFromContext(c)
	if !ok {
		return auth.ErrTokenInvalid
	}

	v, err := h.svc.Subscription(c.Request().Context(), userID)
	if err != nil {
		return err
	}
	dto := subscriptionDTO{
		Plan: v.Plan, Status: v.Status,
		CancelAtPeriodEnd: v.CancelAtPeriodEnd, HasPortal: v.HasPortal,
	}
	if !v.CurrentPeriodEnd.IsZero() {
		s := v.CurrentPeriodEnd.Format(time.RFC3339)
		dto.CurrentPeriodEnd = &s
	}
	return c.JSON(http.StatusOK, dto)
}

// PortalSession は顧客ポータルのセッションを作り URL を返す。
//
//	POST /api/v1/billing/portal-session
//
// Stripe 顧客が無い場合は409、未認証は401。
func (h *BillingHandler) PortalSession(c echo.Context) error {
	userID, ok := UserIDFromContext(c)
	if !ok {
		return auth.ErrTokenInvalid
	}

	url, err := h.svc.CreatePortalSession(c.Request().Context(), userID)
	if err != nil {
		return err // ErrNoBillingCustomer は problem マッピングで 409
	}
	return c.JSON(http.StatusOK, map[string]string{"url": url})
}

// Webhook は Stripe からの Webhook を受け取り、加入状態を同期する。
//
//	POST /api/v1/billing/webhook
//
// Stripe が直接叩くため認証は付けない。署名検証は service.HandleWebhook が行う。
// problem+json ではなく素の応答を返す（Stripe は RFC 7807 を解釈しないため）。
func (h *BillingHandler) Webhook(c echo.Context) error {
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.NoContent(http.StatusBadRequest)
	}
	sig := c.Request().Header.Get("Stripe-Signature")
	if err := h.svc.HandleWebhook(c.Request().Context(), body, sig); err != nil {
		if errors.Is(err, service.ErrWebhookSignature) {
			// 署名不正・本文解釈失敗は再送させない意味で 400。
			return c.NoContent(http.StatusBadRequest)
		}
		// こちらの不調（DB障害など）は Stripe に再送してほしいので 500。
		return c.NoContent(http.StatusInternalServerError)
	}
	return c.NoContent(http.StatusOK)
}
