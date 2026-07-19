package handler

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
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
