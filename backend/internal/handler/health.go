// Package handler はHTTPリクエストの受け口を提供する。
// リクエストの検証とDTO変換のみを担い、ドメインロジックは service に委ねる。
package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// HealthHandler は死活監視用のエンドポイントを提供する。
type HealthHandler struct{}

// NewHealthHandler は HealthHandler を生成する。
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// Health はサーバが応答可能であることを返す。
// DBなど依存先の疎通確認は含めない（コンテナの起動判定に使うため、
// 依存先の障害でコンテナが落ちないようにする）。
func (h *HealthHandler) Health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
