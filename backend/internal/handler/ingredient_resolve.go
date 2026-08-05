package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"unicode/utf8"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// 入力の上限（設計 4.1）。冷蔵庫の中身を書く用途に対して十分に広く、
// かつ LLM に投げるトークンが青天井にならない値。
const (
	maxResolveTextLen = 200
	maxResolveWordNum = 20
)

// IngredientResolveUseCase は手持ちの食材テキストを食材に解決する。
type IngredientResolveUseCase interface {
	Resolve(ctx context.Context, text string, policy service.ResolvePolicy) (service.ResolveResult, error)
}

// ResolveQuotaChecker は今日の残枠を読む。
type ResolveQuotaChecker interface {
	Check(ctx context.Context, s service.ResolveSubject) (bool, service.DegradedReason)
}

// IngredientResolveHandler は食材テキスト解決のHTTP境界。
type IngredientResolveHandler struct {
	svc   IngredientResolveUseCase
	quota ResolveQuotaChecker
	// ipHashSecret は IP を復元できない値にするための鍵（設計 6.2）。
	ipHashSecret []byte
	tokens       *auth.JWT
}

// NewIngredientResolveHandler は IngredientResolveHandler を生成する。
func NewIngredientResolveHandler(
	svc IngredientResolveUseCase,
	quota ResolveQuotaChecker,
	ipHashSecret string,
	tokens *auth.JWT,
) *IngredientResolveHandler {
	return &IngredientResolveHandler{
		svc: svc, quota: quota, ipHashSecret: []byte(ipHashSecret), tokens: tokens,
	}
}

// RegisterRoutes は解決APIのルーティングを登録する。
//
// **既存の /menus/search-by-ingredients には手を入れない**（設計 3.8）。
// 新機能を独立したエンドポイントに閉じ込めることで、最悪これを無効化する
// だけで元の状態に戻せる。
func (h *IngredientResolveHandler) RegisterRoutes(e *echo.Echo, mw ...echo.MiddlewareFunc) {
	g := e.Group(APIBasePath, mw...)
	// OptionalAuth を付けるのは、ログイン中の利用者を IP ではなくユーザーIDで
	// 数えるため。未認証でも通る（拒否はしない）。
	g.POST("/ingredients/resolve", h.Resolve, OptionalAuth(h.tokens))
}

// resolveRequest は POST /ingredients/resolve のリクエストボディ。
type resolveRequest struct {
	Text string `json:"text"`
}

// resolvedWordDTO は解決できた語1件。
type resolvedWordDTO struct {
	Word       string        `json:"word"`
	Ingredient ingredientDTO `json:"ingredient"`
}

// resolveResponse は解決結果。
type resolveResponse struct {
	Resolved   []resolvedWordDTO `json:"resolved"`
	Unresolved []string          `json:"unresolved"`
	Degraded   bool              `json:"degraded"`
	// DegradedReason は Degraded が立った理由。立っていなければ出さない。
	// 画面はこれで文言を選ぶ（設計 10章）。
	DegradedReason string `json:"degradedReason,omitempty"`
}

// Resolve は手持ちの食材テキストを食材に対応づける。
//
//	POST /api/v1/ingredients/resolve  {"text": "豚こま、玉ねぎ、マツタケ"}
//
// 未認証でも使える（spec.md 2.9 の検索と同じ扱い）。
// **LLM が落ちても 502 にはしない。** ①完全一致・②キャッシュで解けた分を
// 200 で返し、degraded を立てる（設計 3.6）。
func (h *IngredientResolveHandler) Resolve(c echo.Context) error {
	var req resolveRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "リクエストの形式が不正です")
	}

	// 長さの検証は service に入る前に済ませる。LLM に渡す前に落とすことが
	// コスト面の一次防御になるため（設計 8章）。
	if utf8.RuneCountInString(req.Text) > maxResolveTextLen {
		return echo.NewHTTPError(http.StatusBadRequest,
			"食材のテキストが長すぎます（最大200文字）")
	}
	if len(domain.SplitIngredientWords(req.Text)) > maxResolveWordNum {
		return echo.NewHTTPError(http.StatusBadRequest,
			"食材の数が多すぎます（最大20件）")
	}

	subject := h.subjectFor(c)
	allow, reason := h.quota.Check(c.Request().Context(), subject)

	result, err := h.svc.Resolve(c.Request().Context(), req.Text, service.ResolvePolicy{
		AllowLLM: allow, DenyReason: reason, Subject: subject,
	})
	if err != nil {
		return err
	}

	resolved := make([]resolvedWordDTO, 0, len(result.Resolved))
	for _, r := range result.Resolved {
		resolved = append(resolved, resolvedWordDTO{
			Word: r.Word, Ingredient: toIngredientDTO(r.Ingredient),
		})
	}
	unresolved := result.Unresolved
	if unresolved == nil {
		// 0件でも null にしない。フロントが length を見られるようにする。
		unresolved = []string{}
	}

	return c.JSON(http.StatusOK, resolveResponse{
		Resolved: resolved, Unresolved: unresolved, Degraded: result.Degraded,
		DegradedReason: string(result.Reason),
	})
}

// subjectFor は数え上げのキーを決める。
//
// ログイン中はユーザーID、非ログインは IP のHMAC（設計 6.2）。
// ブラウザ保存で数えないのは、シークレットウィンドウで無限にリセットできるため。
func (h *IngredientResolveHandler) subjectFor(c echo.Context) service.ResolveSubject {
	if userID, ok := UserIDFromContext(c); ok {
		return service.ResolveSubject{Scope: service.ScopeUser, Subject: userID}
	}
	return service.ResolveSubject{
		Scope:   service.ScopeIP,
		Subject: hashIP(h.ipHashSecret, c.RealIP()),
	}
}

// hashIP は IP を元に戻せない文字列にする。
//
// 数を数えるにはこれで足り、生のIPを保存しないため
// プライバシーポリシーの改定が要らない（設計 6.2）。
func hashIP(secret []byte, ip string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(ip))
	return hex.EncodeToString(mac.Sum(nil))
}
