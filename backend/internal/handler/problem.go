package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

// problemBaseURI はエラー種別のURIの接頭辞。
// RFC 7807 はURIが解決可能であることを要求しないが、種別の識別子として一意にする。
const problemBaseURI = "https://menu-planner.example.com/probs/"

// ProblemContentType は RFC 7807 が定めるメディアタイプ。
const ProblemContentType = "application/problem+json"

// Problem は RFC 7807 の Problem Details for HTTP APIs。
type Problem struct {
	// Type はエラー種別を識別するURI。
	Type string `json:"type"`
	// Title は人間が読める短い要約。同じ Type なら同じ Title になる。
	Title string `json:"title"`
	// Status はHTTPステータスコード。
	Status int `json:"status"`
	// Detail はこの発生に固有の説明。外部に漏らせない情報を含めてはならない。
	Detail string `json:"detail,omitempty"`
}

// problemMapping はドメインのエラーとHTTP表現の対応。
// 上から順に errors.Is で判定するため、より具体的なものを先に置く。
var problemMapping = []struct {
	err    error
	status int
	typ    string
	title  string
}{
	{domain.ErrInvalidGenre, http.StatusBadRequest, "invalid-genre", "不正なジャンルです"},
	{domain.ErrInvalidDifficulty, http.StatusBadRequest, "invalid-difficulty", "不正な難易度です"},
	{domain.ErrInvalidMenuID, http.StatusBadRequest, "invalid-menu-id", "不正な献立IDです"},
	{domain.ErrInvalidMenu, http.StatusBadRequest, "invalid-menu", "不正な献立です"},
	{repository.ErrMenuNotFound, http.StatusNotFound, "menu-not-found", "献立が見つかりません"},
}

// ErrorHandler は echo のカスタムエラーハンドラを返す。
// 全てのエラーレスポンスを RFC 7807 形式に統一する。
func ErrorHandler() echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		// ヘッダ送信済みなら何もできない
		if c.Response().Committed {
			return
		}

		p := toProblem(err)

		if p.Status >= http.StatusInternalServerError {
			// 500系は原因を調べられるようログにだけ残し、レスポンスには含めない
			slog.Error("サーバエラー",
				"error", err,
				"path", c.Request().URL.Path,
				"method", c.Request().Method,
			)
		}

		c.Response().Header().Set(echo.HeaderContentType, ProblemContentType)
		if err := c.JSON(p.Status, p); err != nil {
			slog.Error("エラーレスポンスの書き込みに失敗しました", "error", err)
		}
	}
}

// toProblem はエラーを Problem に変換する。
func toProblem(err error) Problem {
	for _, m := range problemMapping {
		if errors.Is(err, m.err) {
			return Problem{
				Type:   problemBaseURI + m.typ,
				Title:  m.title,
				Status: m.status,
				Detail: err.Error(),
			}
		}
	}

	// echo が生成するエラー(404 / 405 など)はそのステータスを尊重する
	var he *echo.HTTPError
	if errors.As(err, &he) {
		return Problem{
			Type:   problemBaseURI + "http-error",
			Title:  http.StatusText(he.Code),
			Status: he.Code,
		}
	}

	// 未知のエラーは詳細を外部に出さない。
	// DBの認証情報や内部構造が漏れることを防ぐため、Detail は空にする。
	return Problem{
		Type:   problemBaseURI + "internal",
		Title:  "サーバ内部でエラーが発生しました",
		Status: http.StatusInternalServerError,
	}
}
