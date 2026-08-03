package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// ErrSavedWeeklyMenuLimitReached は保存の上限に達していることを表す（409）。
//
// 件数は fmt.Errorf("%w: …") でラップして足す
// （同ファイルの toSavedDays が ErrInvalidWeek に対して取っているのと同じ形）。
// handler は Detail に err.Error() を入れるので、利用者には上限の件数と
// 次に取るべき行動がそのまま届く。
var ErrSavedWeeklyMenuLimitReached = errors.New("保存できる週間献立の上限に達しました")

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

	// Find は本人の保存を1件、中身の7日分も含めて返す。
	// 他人のもの・存在しないものは ErrSavedWeeklyMenuNotFound を返す（存在を明かさない）。
	Find(ctx context.Context, userID domain.UserID, id domain.SavedWeeklyMenuID) (domain.SavedWeeklyMenu, error)
}

// SavedDayInput は保存する1日分の指定。APIの生の値をそのまま受ける。
type SavedDayInput struct {
	Day    int
	MenuID string
}

// SavedWeeklyMenuService は週間献立の保存を担う。
type SavedWeeklyMenuService struct {
	store        SavedWeeklyMenuStore
	entitlements Entitlements
}

// NewSavedWeeklyMenuService は SavedWeeklyMenuService を生成する。
func NewSavedWeeklyMenuService(
	store SavedWeeklyMenuStore, entitlements Entitlements,
) *SavedWeeklyMenuService {
	return &SavedWeeklyMenuService{store: store, entitlements: entitlements}
}

// Save は認証済みユーザーの週間献立を1件保存する。
//
// 7日分ちょうどでなければ ErrInvalidWeek（400）、日の範囲外・重複は ErrInvalidDay（400）、
// 献立IDが壊れていれば domain.ErrInvalidMenuID（400）、
// 上限に達していれば ErrSavedWeeklyMenuLimitReached（409）。
func (s *SavedWeeklyMenuService) Save(
	ctx context.Context, userID string, input []SavedDayInput,
) (domain.SavedWeeklyMenuID, error) {
	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return domain.SavedWeeklyMenuID{}, ErrUserNotFound
	}

	days, err := toSavedDays(input)
	if err != nil {
		return domain.SavedWeeklyMenuID{}, err
	}

	// 上限は全員50件で一律（domain.Entitlement.SavedWeeklyMenuLimit、サブスク撤廃により統一）。
	ent, err := s.entitlements.For(ctx, userID)
	if err != nil {
		return domain.SavedWeeklyMenuID{}, err
	}
	limit := ent.SavedWeeklyMenuLimit()

	// 上限は保存の直前に見る（spec.md 2.8）。
	//
	// **数えてから書くまでの間に別のリクエストが割り込む余地は残る。**
	// 同一利用者が同時に保存を送ると上限+1件目が通りうるが、上限は
	// 「際限なく溜めない」ための目安であり、そこまでの厳密さは要らないと判断した。
	// DB制約で締めるには件数を数えるトリガか排他ロックが要り、割に合わない。
	count, err := s.store.Count(ctx, uid)
	if err != nil {
		return domain.SavedWeeklyMenuID{}, err
	}
	if count >= limit {
		// 文言はここで組み立てきる。フロントが件数を持つと二重管理になり、
		// 上限値を変えたときに古い数値のまま出続ける食い違いが起きる（spec.md 2.11）。
		return domain.SavedWeeklyMenuID{}, fmt.Errorf(
			"%w: 保存できるのは%d件までです。「保存した週間献立」から古いものを削除してください",
			ErrSavedWeeklyMenuLimitReached, limit)
	}

	return s.store.Save(ctx, uid, days)
}

// List は認証済みユーザーの保存を新しい順に、中身の7日分も含めて返す。
//
// 上限は最大でも50件と小さいので全件返す。ページングは設けない。
func (s *SavedWeeklyMenuService) List(
	ctx context.Context, userID string,
) ([]domain.SavedWeeklyMenu, error) {
	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return s.store.List(ctx, uid)
}

// Delete は認証済みユーザーの保存を1件削除する。
// 自分のものでなければ ErrSavedWeeklyMenuNotFound（404）。
func (s *SavedWeeklyMenuService) Delete(ctx context.Context, userID, id string) error {
	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return ErrUserNotFound
	}
	sid, err := domain.ParseSavedWeeklyMenuID(id)
	if err != nil {
		return err
	}
	return s.store.Delete(ctx, uid, sid)
}

// toSavedDays は API の生の指定を検証して DayMenu に直す。
//
// 献立の中身はここでは引かない。存在するかどうかは保存時の外部キーが判定し、
// repository が ErrMenuNotFound に変換する。事前に SELECT で確かめる方式は、
// 確認と INSERT の間に他の操作が割り込むと破綻するため採らない（お気に入りと同じ）。
func toSavedDays(input []SavedDayInput) ([]domain.DayMenu, error) {
	if len(input) != domain.WeekLength {
		return nil, fmt.Errorf("%w: %d日分の指定です（%d日分が必要）",
			ErrInvalidWeek, len(input), domain.WeekLength)
	}

	days := make([]domain.DayMenu, 0, domain.WeekLength)
	seen := make(map[int]bool, domain.WeekLength)
	for _, in := range input {
		if in.Day < 1 || in.Day > domain.WeekLength {
			return nil, fmt.Errorf("%w: %d", ErrInvalidDay, in.Day)
		}
		if seen[in.Day] {
			return nil, fmt.Errorf("%w: %d日目が重複しています", ErrInvalidDay, in.Day)
		}
		seen[in.Day] = true

		mid, err := domain.ParseMenuID(in.MenuID)
		if err != nil {
			return nil, err
		}
		days = append(days, domain.DayMenu{Day: in.Day, Menu: domain.Menu{ID: mid}})
	}
	return days, nil
}
