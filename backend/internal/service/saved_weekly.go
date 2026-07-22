package service

import (
	"context"
	"errors"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// SavedWeeklyMenuLimit は1ユーザーが保存できる週間献立の件数（spec.md 2.8）。
//
// 履歴のように FIFO で押し出さないため、上限は「静かに消える件数」ではなく
// 「保存を断る境目」になる。保存は利用者の明示的な操作であり、
// 黙って消えると保存という行為の意味が壊れる。
const SavedWeeklyMenuLimit = 10

// ErrSavedWeeklyMenuLimitReached は保存の上限に達していることを表す（409）。
// 古いものを消してもらうため、押し出さずに断る。
var ErrSavedWeeklyMenuLimitReached = errors.New("保存できる週間献立は10件までです。古いものを削除してください")

// ErrSavedWeeklyMenuNotFound は指定の保存が見つからないことを表す（404）。
//
// 他人の保存を消そうとした場合もこれになる。「他人のものだ」と伝える 403 は、
// 存在そのものを明かすことになるため返さない（お気に入りと同じ扱い）。
var ErrSavedWeeklyMenuNotFound = errors.New("保存した週間献立が見つかりません")

// SavedWeeklyMenuStore は保存した週間献立の永続化を抽象化する。実装は internal/repository。
type SavedWeeklyMenuStore interface {
	// Save は1週間分をまとめて保存し、採番したIDを返す。
	// days は7日分ちょうどで、day の重複が無いことを呼び出し側が保証する。
	// 存在しない献立を含む場合は repository.ErrMenuNotFound を返す。
	Save(ctx context.Context, userID domain.UserID, days []domain.DayMenu) (domain.SavedWeeklyMenuID, error)

	// List はユーザーの保存を新しい順に、中身の7日分も含めて返す。
	// 該当が無ければ空スライス（nil ではない）。
	List(ctx context.Context, userID domain.UserID) ([]domain.SavedWeeklyMenu, error)

	// Count はユーザーの保存件数を返す。上限判定に使う。
	Count(ctx context.Context, userID domain.UserID) (int, error)

	// Delete は保存を1件削除する。
	// 該当が無い、または他人のものであれば ErrSavedWeeklyMenuNotFound を返す。
	Delete(ctx context.Context, userID domain.UserID, id domain.SavedWeeklyMenuID) error
}
