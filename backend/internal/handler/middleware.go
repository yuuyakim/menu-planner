package handler

import (
	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// userIDContextKey は認証済みユーザーのIDを echo コンテキストに置く際のキー。
// 文字列の衝突を避けるため接頭辞を付ける。
const userIDContextKey = "auth.userID"

// RequireAuth はアクセストークンの Cookie を検証し、通ったリクエストだけを先へ通す。
// 検証済みの userID をコンテキストに載せ、後続ハンドラが UserIDFromContext で取り出せる。
//
// 認証が要らないエンドポイント（献立検索など）はこのミドルウェアで包まない。
// 包まなければ未認証でも 200 のまま通る。
func RequireAuth(tokens *auth.JWT) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cookie, err := c.Cookie(accessCookieName)
			if err != nil {
				// Cookie が無いのも「認証されていない」なので 401 に丸める。
				return auth.ErrTokenInvalid
			}
			claims, err := tokens.Verify(cookie.Value)
			if err != nil {
				// 期限切れ・改竄・種別違いはすべて ErrTokenInvalid → 401。
				return err
			}
			c.Set(userIDContextKey, claims.UserID)
			return next(c)
		}
	}
}

// OptionalAuth はアクセストークンがあれば検証して userID をコンテキストに載せるが、
// 無くても・無効でも拒否せず先へ通す。
//
// 献立検索のように「未認証でも使えるが、ログイン中なら履歴を活かしたい」
// エンドポイントに使う。RequireAuth との違いは、認証が無くても 401 にしないこと。
func OptionalAuth(tokens *auth.JWT) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if cookie, err := c.Cookie(accessCookieName); err == nil {
				// 無効なトークンは「未認証」として扱い、素通りさせる。
				if claims, err := tokens.Verify(cookie.Value); err == nil {
					c.Set(userIDContextKey, claims.UserID)
				}
			}
			return next(c)
		}
	}
}

// UserIDFromContext は RequireAuth / OptionalAuth が載せた userID を取り出す。
// 載っていなければ ok=false。
func UserIDFromContext(c echo.Context) (string, bool) {
	v, ok := c.Get(userIDContextKey).(string)
	return v, ok && v != ""
}

// RequirePremium は premium 加入者だけを通すミドルウェア。RequireAuth の後段に置く。
//
// userID が無ければ 401（RequireAuth が先に通っていない配線ミス、または未認証）。
// premium でなければ 403（ErrPremiumRequired）。エンタイトルメントの引き当てに
// 失敗したら、その err をそのまま返す（500 系）。
//
// サブスク撤廃（2026-08-02）により、現在この分岐には到達しない。
// 配管は復活に備えて残しているため、戻せばここが再び効く。
func RequirePremium(entitlements service.Entitlements) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userID, ok := UserIDFromContext(c)
			if !ok {
				return auth.ErrTokenInvalid
			}
			ent, err := entitlements.For(c.Request().Context(), userID)
			if err != nil {
				return err
			}
			if !ent.CanUseWeeklyPlanning() {
				return service.ErrPremiumRequired
			}
			return next(c)
		}
	}
}
