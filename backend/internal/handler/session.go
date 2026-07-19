package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

const (
	// accessCookieName / refreshCookieName は認証トークンを載せる Cookie 名。
	accessCookieName  = "access_token"
	refreshCookieName = "refresh_token"

	// refreshCookiePath はリフレッシュトークンを送る範囲。長寿命なので
	// /auth 配下（refresh / logout）にだけ送り、他のAPIには載せない。
	refreshCookiePath = APIBasePath + "/auth"
)

// issueSession は userID のアクセス／リフレッシュトークンを発行し、Cookie にして返す。
// ログイン・サインアップ・リフレッシュの成功時に使う。
func (h *AuthHandler) issueSession(c echo.Context, userID string) error {
	access, err := h.tokens.Issue(userID)
	if err != nil {
		return err
	}
	refresh, err := h.tokens.IssueRefresh(userID)
	if err != nil {
		return err
	}

	// Cookie の寿命はトークンの寿命に揃える。ブラウザ側の保持期間が
	// トークンの有効期間とずれないようにするため。
	c.SetCookie(authCookie(accessCookieName, access, "/", int(h.tokens.AccessTTL().Seconds())))
	c.SetCookie(authCookie(refreshCookieName, refresh, refreshCookiePath, int(h.tokens.RefreshTTL().Seconds())))
	return nil
}

// refreshAccess はリフレッシュせずアクセストークンだけを再発行して Cookie を更新する。
func (h *AuthHandler) refreshAccess(c echo.Context, userID string) error {
	access, err := h.tokens.Issue(userID)
	if err != nil {
		return err
	}
	c.SetCookie(authCookie(accessCookieName, access, "/", int(h.tokens.AccessTTL().Seconds())))
	return nil
}

// clearSession は認証 Cookie を失効させる。ログアウトで使う。
func clearSession(c echo.Context) {
	// パスは発行時と揃える。揃っていないとブラウザが別 Cookie とみなし消えない。
	c.SetCookie(expiredCookie(accessCookieName, "/"))
	c.SetCookie(expiredCookie(refreshCookieName, refreshCookiePath))
}

// authCookie は認証用の Cookie を組み立てる。
// HttpOnly + Secure + SameSite=Lax は spec.md 11章のセキュリティ要件。
//   - HttpOnly: JS から読めないようにし、XSS でのトークン窃取を防ぐ。
//   - Secure: HTTPS でのみ送る（localhost は例外的に http でも許される）。
//   - SameSite=Lax: 他サイトからの POST に Cookie を付けず CSRF を抑える。
func authCookie(name, value, path string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}

// expiredCookie は同名・同パスの Cookie を即時失効させるための値。
func expiredCookie(name, path string) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     path,
		MaxAge:   -1, // 即時削除
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}
