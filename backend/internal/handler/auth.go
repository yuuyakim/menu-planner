package handler

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// UserSignUpper はサインアップを抽象化する。実装は service.AuthService。
// handler が必要とする操作だけを宣言し、テストで差し替えられるようにする。
type UserSignUpper interface {
	SignUp(ctx context.Context, email, password string) (domain.User, error)
}

// UserLoginner はログイン（認証情報の照合）を抽象化する。実装は service.AuthService。
type UserLoginner interface {
	Login(ctx context.Context, email, password string) (domain.User, error)
}

// CurrentUserGetter は現在のユーザーの取得を抽象化する。実装は service.AuthService。
type CurrentUserGetter interface {
	CurrentUser(ctx context.Context, userID string) (domain.User, error)
}

// GoogleUpserter は Google 認証ユーザーの取得・作成を抽象化する。実装は service.AuthService。
type GoogleUpserter interface {
	UpsertGoogleUser(ctx context.Context, g service.GoogleUser) (domain.User, error)
}

// AuthUseCase は認証APIが必要とする操作をまとめたもの。
type AuthUseCase interface {
	UserSignUpper
	UserLoginner
	CurrentUserGetter
	GoogleUpserter
}

// GoogleAuthenticator は Google の認可URL生成とコード交換を抽象化する。
// 実装は auth.GoogleOAuth。テストで差し替えられるようにする。
type GoogleAuthenticator interface {
	Configured() bool
	AuthCodeURL(state, verifier string) string
	Exchange(ctx context.Context, code, verifier string) (auth.GoogleIdentity, error)
}

// AuthHandler は認証APIの受け口。
type AuthHandler struct {
	svc          AuthUseCase
	tokens       *auth.JWT
	google       GoogleAuthenticator
	frontendURL  string
	entitlements service.Entitlements
}

// NewAuthHandler は AuthHandler を生成する。
// tokens はトークンの発行・検証、google は Google SSO、frontendURL は
// Google ログイン完了後に戻すフロントのURL、entitlements はプランの問い合わせ。
func NewAuthHandler(
	s AuthUseCase, tokens *auth.JWT, google GoogleAuthenticator, frontendURL string,
	entitlements service.Entitlements,
) *AuthHandler {
	return &AuthHandler{
		svc: s, tokens: tokens, google: google, frontendURL: frontendURL,
		entitlements: entitlements,
	}
}

// RegisterRoutes は認証APIのルーティングを登録する。
// mw はグループ全体に前置するミドルウェア（レート制限など）。
func (h *AuthHandler) RegisterRoutes(e *echo.Echo, mw ...echo.MiddlewareFunc) {
	g := e.Group(APIBasePath, mw...)
	g.POST("/auth/signup", h.SignUp)
	g.POST("/auth/login", h.Login)
	g.POST("/auth/refresh", h.Refresh)
	g.POST("/auth/logout", h.Logout)
	g.GET("/auth/google", h.GoogleStart)
	g.GET("/auth/google/callback", h.GoogleCallback)
	// /auth/me は認証必須。RequireAuth を通ったリクエストだけ来る。
	g.GET("/auth/me", h.Me, RequireAuth(h.tokens))
}

// signupRequest は POST /auth/signup のリクエスト（spec.md 5.2）。
// 表示名は受け取らない。メールのローカル部から導出する。
type signupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// userDTO はユーザーのAPI表現。パスワードやハッシュは決して含めない。
type userDTO struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Plan        string `json:"plan"`
}

// userResponse はユーザー1件を返すエンドポイントの共通レスポンス。
type userResponse struct {
	User userDTO `json:"user"`
}

// SignUp はメールとパスワードで新規ユーザーを登録する。
//
//	POST /api/v1/auth/signup
//
// 成功時は 201。Cookie やトークンの発行はこの段階では行わない（5-E / 5-F）。
func (h *AuthHandler) SignUp(c echo.Context) error {
	var req signupRequest
	if err := c.Bind(&req); err != nil {
		return err
	}

	user, err := h.svc.SignUp(c.Request().Context(), req.Email, req.Password)
	if err != nil {
		return err
	}

	// 登録と同時にログイン状態にする。認証 Cookie を発行する。
	if err := h.issueSession(c, user.ID.String()); err != nil {
		return err
	}

	// サインアップ直後は必ず free（有料プランへの加入経路が無い）。
	// entitlements を引く必要はない。
	return c.JSON(http.StatusCreated, userResponse{User: toUserDTO(user, domain.PlanFree)})
}

// loginRequest は POST /auth/login のリクエスト（spec.md 5.2）。
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login はメールとパスワードで認証する。
//
//	POST /api/v1/auth/login
//
// 成功時は 200。認証情報が正しくなければ 401（存在しないメールもパスワード
// 違いも同じ）。Cookie / トークンの発行はこの段階では行わない（5-E / 5-F）。
func (h *AuthHandler) Login(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return err
	}

	user, err := h.svc.Login(c.Request().Context(), req.Email, req.Password)
	if err != nil {
		return err
	}

	if err := h.issueSession(c, user.ID.String()); err != nil {
		return err
	}

	// ログインは既存利用者を通す経路であり、プレミアムでありうる。
	// entitlements を引いて実際のプランを返す。
	ent, err := h.entitlements.For(c.Request().Context(), user.ID.String())
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, userResponse{User: toUserDTO(user, ent.Plan())})
}

// Refresh はリフレッシュトークンでアクセストークンを再発行する。
//
//	POST /api/v1/auth/refresh
//
// リフレッシュ Cookie が無い・無効・期限切れならすべて 401。成功時は
// アクセス Cookie を差し替えて 204（本体は無い。フロントは元の要求を再送する）。
func (h *AuthHandler) Refresh(c echo.Context) error {
	cookie, err := c.Cookie(refreshCookieName)
	if err != nil {
		// Cookie が無いのも「認証されていない」ので 401 に丸める。
		return auth.ErrTokenInvalid
	}

	claims, err := h.tokens.VerifyRefresh(cookie.Value)
	if err != nil {
		return err
	}

	if err := h.refreshAccess(c, claims.UserID); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// Logout は認証 Cookie を失効させる。
//
//	POST /api/v1/auth/logout
//
// 未ログインでも成功扱い（冪等）。トークンは stateless なので、サーバ側では
// Cookie を消すことでログアウトとする。
func (h *AuthHandler) Logout(c echo.Context) error {
	clearSession(c)
	return c.NoContent(http.StatusNoContent)
}

// Me は現在のユーザー情報を返す。
//
//	GET /api/v1/auth/me
//
// RequireAuth を前置しているので、ここに来る時点で認証済み。
func (h *AuthHandler) Me(c echo.Context) error {
	userID, ok := UserIDFromContext(c)
	if !ok {
		// ミドルウェアを通っていれば必ず入っている。保険として 401。
		return auth.ErrTokenInvalid
	}

	user, err := h.svc.CurrentUser(c.Request().Context(), userID)
	if err != nil {
		return err
	}

	ent, err := h.entitlements.For(c.Request().Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, userResponse{User: toUserDTO(user, ent.Plan())})
}

func toUserDTO(u domain.User, plan domain.Plan) userDTO {
	return userDTO{
		ID:          u.ID.String(),
		Email:       u.Email.String(),
		DisplayName: u.DisplayName,
		Plan:        plan.String(),
	}
}
