package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
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
	{domain.ErrInvalidHistoryID, http.StatusBadRequest, "invalid-history-id", "不正な履歴IDです"},
	{domain.ErrInvalidSavedWeeklyMenuID, http.StatusBadRequest, "invalid-saved-weekly-menu-id", "不正な保存IDです"},
	{domain.ErrInvalidMenu, http.StatusBadRequest, "invalid-menu", "不正な献立です"},
	{repository.ErrMenuNotFound, http.StatusNotFound, "menu-not-found", "献立が見つかりません"},
	{service.ErrHistoryNotFound, http.StatusNotFound, "history-not-found", "履歴が見つかりません"},
	// 自分のお気に入りに無い献立の削除。他人のものを消そうとした場合もここに来る
	// （所有者を明かす 403 は他人の登録内容を漏らすため返さない）。
	{service.ErrFavoriteNotFound, http.StatusNotFound, "favorite-not-found", "お気に入りが見つかりません"},
	// 他人の保存を消そうとした場合もここに来る。403 だと保存の存在を明かすため返さない。
	{service.ErrSavedWeeklyMenuNotFound, http.StatusNotFound, "saved-weekly-menu-not-found", "保存した週間献立が見つかりません"},
	{service.ErrHistoryForbidden, http.StatusForbidden, "history-forbidden", "この履歴を操作する権限がありません"},
	{service.ErrInvalidDay, http.StatusBadRequest, "invalid-day", "不正な日の指定です"},
	// 買い物リストの献立指定が0件、または上限（7件）超過。
	{service.ErrInvalidMenuIDs, http.StatusBadRequest, "invalid-menu-ids", "献立の指定が不正です"},
	// 指定された献立の中に存在しないものがある。黙って除くと
	// 「頼んだ献立の食材が抜けたリスト」を渡すことになるため 404 で知らせる。
	{service.ErrMenuNotFoundInList, http.StatusNotFound, "menu-not-found", "指定された献立が見つかりません"},
	{service.ErrInvalidWeek, http.StatusBadRequest, "invalid-week", "不正な週間献立です"},
	{domain.ErrInvalidEmail, http.StatusBadRequest, "invalid-email", "不正なメールアドレスです"},
	{auth.ErrPasswordTooShort, http.StatusBadRequest, "password-too-short", "パスワードが短すぎます"},
	{auth.ErrPasswordTooLong, http.StatusBadRequest, "password-too-long", "パスワードが長すぎます"},
	// メールの重複は「今の状態と競合する」ので 409。
	{service.ErrEmailTaken, http.StatusConflict, "email-taken", "メールアドレスは既に登録されています"},
	// お気に入りの重複も同様に 409。既にある状態との競合であって入力の誤りではない。
	{service.ErrFavoriteExists, http.StatusConflict, "favorite-exists", "この献立は既にお気に入りに登録されています"},
	// 保存した週間献立が上限（10件）に達している。履歴のように押し出さず断るため、
	// 入力の誤り（400）ではなく今の状態との競合（409）にする（spec.md 2.8）。
	{service.ErrSavedWeeklyMenuLimitReached, http.StatusConflict, "saved-weekly-menu-limit-reached", "保存できる週間献立は10件までです"},
	// 認証失敗。存在しないメールもパスワード違いも同じ 401 に丸める。
	{service.ErrInvalidCredentials, http.StatusUnauthorized, "invalid-credentials", "メールアドレスまたはパスワードが正しくありません"},
	// トークンが無効（欠落・期限切れ・改竄・種別違い）。内訳は明かさず一律 401。
	{auth.ErrTokenInvalid, http.StatusUnauthorized, "token-invalid", "認証が必要です"},
	// 有効なトークンが指すユーザーが居ない＝セッション不正。再ログインを促す 401。
	{service.ErrUserNotFound, http.StatusUnauthorized, "user-not-found", "認証が必要です"},
	// Google 認証の失敗（state不一致・コード交換失敗・メール未確認）。内訳は明かさず 401。
	{auth.ErrGoogleAuthFailed, http.StatusUnauthorized, "google-auth-failed", "Google認証に失敗しました"},
	// リクエストは正しいが条件に合う献立が無い状態。構文は正しいので400ではなく422。
	{service.ErrNoMenuFound, http.StatusUnprocessableEntity, "no-menu-found", "条件に合う献立が見つかりません"},
	// 外部の検索APIの不調。自分の障害ではないので500ではなく502で上流起因だと示す。
	{service.ErrRecipeSearchFailed, http.StatusBadGateway, "recipe-search-failed", "レシピの取得に失敗しました"},
	// レート制限超過。時間をおけば回復するので 429（Retry-After は middleware が付ける）。
	{ErrRateLimited, http.StatusTooManyRequests, "rate-limited", "リクエストが多すぎます"},
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

		// ブラウザのトップレベル遷移（Googleログインの開始やコールバック）で
		// エラーになった場合、problem+json をそのまま返すと利用者の画面に
		// 生のJSONが表示されてしまう（Issue #81）。ログイン画面へ戻し、
		// 理由をクエリで渡してフロントに文言を出させる。
		//
		// フロントと backend は同一オリジンで配信されるため、相対パスで返せば
		// ブラウザがフロントのオリジンに解決する。ここでフロントのURLを
		// 知る必要が無い。
		if prefersHTML(c.Request()) {
			kind := strings.TrimPrefix(p.Type, problemBaseURI)
			target := "/login?error=" + url.QueryEscape(kind)
			if err := c.Redirect(http.StatusFound, target); err != nil {
				LoggerFrom(c).Error("エラー時のリダイレクトに失敗しました", "error", err)
			}
			return
		}

		if p.Status >= http.StatusInternalServerError {
			// 500系は原因を調べられるようログにだけ残し、レスポンスには含めない
			LoggerFrom(c).Error("サーバエラー",
				"error", err,
				"path", c.Request().URL.Path,
				"method", c.Request().Method,
			)
		}

		c.Response().Header().Set(echo.HeaderContentType, ProblemContentType)
		if err := c.JSON(p.Status, p); err != nil {
			LoggerFrom(c).Error("エラーレスポンスの書き込みに失敗しました", "error", err)
		}
	}
}

// prefersHTML はリクエストがブラウザの画面遷移かを判定する。
//
// ブラウザはトップレベル遷移で `Accept: text/html,...` を送る。一方
// アプリ内の fetch は既定で `*/*` を送るため、これで両者を分けられる。
// 「HTMLを受け取れる相手か」で判断するので、判定を外しても
// APIクライアント側は従来どおり problem+json を受け取る。
func prefersHTML(req *http.Request) bool {
	return strings.Contains(req.Header.Get(echo.HeaderAccept), "text/html")
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
