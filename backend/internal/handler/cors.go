package handler

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// CORS はフロントのオリジン（frontendOrigin）だけにクロスオリジンアクセスを許す
// ミドルウェアを返す（spec.md 11章）。
//
// Cookie で認証するため AllowCredentials を有効にする。ワイルドカード "*" は
// AllowCredentials と併用できないうえ、意図せず全オリジンを許してしまうため使わない。
// 許可オリジンを1つに絞ることで、他サイトからの資格情報付きリクエストを弾く。
func CORS(frontendOrigin string) echo.MiddlewareFunc {
	return middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{frontendOrigin},
		AllowCredentials: true,
	})
}
