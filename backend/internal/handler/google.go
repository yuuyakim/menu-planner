package handler

import (
	"crypto/subtle"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

const (
	// oauthStateCookieName / oauthVerifierCookieName は Google 認可フローの
	// 途中状態を保持する Cookie。コールバックで照合するために持たせる。
	oauthStateCookieName    = "oauth_state"
	oauthVerifierCookieName = "oauth_verifier"

	// oauthCookiePath は認可フローの Cookie を送る範囲。/auth 配下
	// （google / google/callback）にだけ送る。
	oauthCookiePath = APIBasePath + "/auth"

	// oauthCookieTTL は認可フローの猶予。ユーザーが Google の画面で
	// 操作する間だけ持てばよいので短くする。
	oauthCookieTTL = 10 * time.Minute
)

// GoogleStart は Google の認可画面へリダイレクトする。
//
//	GET /api/v1/auth/google
//
// PKCE の verifier と CSRF 対策の state を生成し、Cookie に保存してから
// Google の認可URLへ 302 で送る。state と verifier はコールバックで照合する。
func (h *AuthHandler) GoogleStart(c echo.Context) error {
	if !h.google.Configured() {
		// 設定が無い環境（GOOGLE_CLIENT_ID 未設定）では利用できない。
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Google ログインは利用できません")
	}

	verifier := auth.GenerateVerifier()
	state, err := auth.GenerateState()
	if err != nil {
		return err
	}

	// 途中状態はブラウザ側にだけ持たせる（サーバは状態を持たない）。
	// SameSite=Lax なので、Google からのトップレベル遷移（GET）でも送られる。
	c.SetCookie(oauthCookie(oauthStateCookieName, state))
	c.SetCookie(oauthCookie(oauthVerifierCookieName, verifier))

	return c.Redirect(http.StatusFound, h.google.AuthCodeURL(state, verifier))
}

// GoogleCallback は Google からの認可コードを受け取り、ログインを完了する。
//
//	GET /api/v1/auth/google/callback?code=...&state=...
//
// state を Cookie と照合（CSRF対策）し、コードを本人情報に交換して、ユーザーを
// 取得または作成し、認証 Cookie を発行してフロントへ戻す。state 不一致・
// verifier 欠落・交換失敗・メール未確認はすべて 401 に丸める。
func (h *AuthHandler) GoogleCallback(c echo.Context) error {
	if !h.google.Configured() {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Google ログインは利用できません")
	}

	// CSRF対策: 送られてきた state が、開始時に Cookie に保存した値と一致すること。
	stateCookie, err := c.Cookie(oauthStateCookieName)
	if err != nil || stateCookie.Value == "" {
		return auth.ErrGoogleAuthFailed
	}
	// 一致判定は定数時間で行う（長さ違いは 0 が返る）。
	if subtle.ConstantTimeCompare([]byte(c.QueryParam("state")), []byte(stateCookie.Value)) != 1 {
		return auth.ErrGoogleAuthFailed
	}

	verifierCookie, err := c.Cookie(oauthVerifierCookieName)
	if err != nil || verifierCookie.Value == "" {
		return auth.ErrGoogleAuthFailed
	}

	code := c.QueryParam("code")
	if code == "" {
		return auth.ErrGoogleAuthFailed
	}

	identity, err := h.google.Exchange(c.Request().Context(), code, verifierCookie.Value)
	if err != nil {
		return err
	}
	// 未確認メールでの紐付けは乗っ取りに使えるため受け入れない。
	if !identity.EmailVerified {
		return auth.ErrGoogleAuthFailed
	}

	user, err := h.svc.UpsertGoogleUser(c.Request().Context(), service.GoogleUser{
		Sub:         identity.Sub,
		Email:       identity.Email,
		DisplayName: identity.Name,
	})
	if err != nil {
		return err
	}

	// 使い終わった途中状態の Cookie は消し、認証 Cookie を発行する。
	clearOAuthCookies(c)
	if err := h.issueSession(c, user.ID.String()); err != nil {
		return err
	}

	// ログイン済みの状態でフロントに戻す。
	return c.Redirect(http.StatusFound, h.frontendURL)
}

// oauthCookie は認可フローの途中状態を保持する Cookie を組み立てる。
func oauthCookie(name, value string) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     oauthCookiePath,
		MaxAge:   int(oauthCookieTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}

// clearOAuthCookies は認可フローの途中状態の Cookie を失効させる。
func clearOAuthCookies(c echo.Context) {
	for _, name := range []string{oauthStateCookieName, oauthVerifierCookieName} {
		c.SetCookie(&http.Cookie{
			Name:     name,
			Value:    "",
			Path:     oauthCookiePath,
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})
	}
}
