package handler_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/handler"
	"github.com/yuuyakim/menu-planner/backend/internal/logctx"
)

// loggingEcho はロギングミドルウェアだけを載せた echo と、ログの出力先を返す。
// h はテストごとに差し替えるハンドラ。
func loggingEcho(h echo.HandlerFunc) (*echo.Echo, *bytes.Buffer) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	// RequestID が先に走って X-Request-Id を載せる。ロギングはそれを読む。
	e.Use(middleware.RequestID())
	e.Use(handler.RequestLogging(logger))
	e.Add(http.MethodGet, "/x", h)
	e.Add(http.MethodPost, "/x", h)
	return e, &buf
}

// logLines は捕捉したJSONログを1行ずつパースして返す。
func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	sc := bufio.NewScanner(buf)
	for sc.Scan() {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal(sc.Bytes(), &m), "ログ行がJSONでない: %s", sc.Text())
		lines = append(lines, m)
	}
	return lines
}

func TestRequestLogging_RequestIDPropagatesToAllLogs(t *testing.T) {
	// ハンドラ（および下位層を模した context 経由）のログにも、
	// アクセスログにも、同じ request_id が乗ることを確かめる。
	e, buf := loggingEcho(func(c echo.Context) error {
		// 下位層（service 相当）は context から logger を取り出して使う。
		logctx.From(c.Request().Context()).Info("下位層のログ")
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(echo.HeaderXRequestID, "test-req-id-123")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	lines := logLines(t, buf)
	require.GreaterOrEqual(t, len(lines), 2, "下位層のログとアクセスログの2行以上が出る")
	for _, l := range lines {
		assert.Equal(t, "test-req-id-123", l["request_id"], "全ログに request_id が乗る: %v", l)
	}
}

func TestRequestLogging_DoesNotLogPassword(t *testing.T) {
	// リクエスト本文にパスワードが含まれていても、ログに出してはいけない。
	const password = "super-secret-password-9137"
	e, buf := loggingEcho(func(c echo.Context) error {
		// 本文を読み切る（本番のハンドラと同様）。それでもログには出ない。
		_, _ = io.ReadAll(c.Request().Body)
		return c.String(http.StatusOK, "ok")
	})

	body := `{"email":"a@example.com","password":"` + password + `"}`
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.NotContains(t, buf.String(), password, "パスワードがログに漏れている")
}

func TestRequestLogging_DoesNotLogToken(t *testing.T) {
	// Cookie / Authorization に載ったトークンをログに出してはいけない。
	const token = "eyJhbGciOiJdummy.token.value-4471"
	e, buf := loggingEcho(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Cookie", "access_token="+token)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.NotContains(t, buf.String(), token, "トークンがログに漏れている")
}
