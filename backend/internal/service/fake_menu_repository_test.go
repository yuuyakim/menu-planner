package service_test

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// fakeMenuRepository はメモリ上の献立マスタ。実装が service.MenuRepository を
// 満たすことをコンパイル時に保証する。
var _ service.MenuRepository = (*fakeMenuRepository)(nil)

// errFakeMenuNotFound は fake における「献立が存在しない」を表す。
// 本物の repository.ErrMenuNotFound の代役。service は repository を知らない
// （依存関係逆転）ため、実物の番兵ではなくテスト固有の番兵で代用し、
// service がエラーの同一性を保ったまま返すことだけを検証する。
var errFakeMenuNotFound = errors.New("献立が見つかりません(fake)")

// fakeMenuRepository は Postgres 実装の代わりにメモリ上で絞り込みを行う。
// 本物と同じ「条件に合う献立を返す / 該当が無ければ空スライス」という契約を守る。
type fakeMenuRepository struct {
	menus []domain.Menu
	// err が非nilならすべての呼び出しがこのエラーを返す。
	err error
	// lastFilter は最後に FindByFilter に渡された条件。
	lastFilter domain.MenuFilter
	// filterCalls は FindByFilter が呼ばれた回数。
	filterCalls int
	// lastID は最後に FindByID に渡されたID。
	lastID domain.MenuID
	// idCalls は FindByID が呼ばれた回数。
	idCalls int
	// idsCalls は FindByIDs が呼ばれた回数。
	idsCalls int
}

func newFakeMenuRepository(menus ...domain.Menu) *fakeMenuRepository {
	return &fakeMenuRepository{menus: menus}
}

func (r *fakeMenuRepository) FindByID(_ context.Context, id domain.MenuID) (*domain.Menu, error) {
	r.idCalls++
	r.lastID = id
	if r.err != nil {
		return nil, r.err
	}
	for _, m := range r.menus {
		if m.ID == id {
			return &m, nil
		}
	}
	// 本物の repository と同じく、存在しないことを示す番兵を包んで返す。
	return nil, fmt.Errorf("%w: %s", errFakeMenuNotFound, id)
}

func (r *fakeMenuRepository) FindByIDs(_ context.Context, ids []domain.MenuID) ([]domain.Menu, error) {
	r.idsCalls++
	if r.err != nil {
		return nil, r.err
	}
	found := []domain.Menu{}
	for _, m := range r.menus {
		if slices.Contains(ids, m.ID) {
			found = append(found, m)
		}
	}
	return found, nil
}

func (r *fakeMenuRepository) FindByFilter(_ context.Context, f domain.MenuFilter) ([]domain.Menu, error) {
	r.filterCalls++
	r.lastFilter = f
	if r.err != nil {
		return nil, r.err
	}

	found := []domain.Menu{}
	for _, m := range r.menus {
		if f.Genre != nil && m.Genre != *f.Genre {
			continue
		}
		if f.Difficulty != nil && m.Difficulty != *f.Difficulty {
			continue
		}
		if slices.Contains(f.ExcludeIDs, m.ID) {
			continue
		}
		found = append(found, m)
	}
	return found, nil
}

// newMenu はテスト用の献立を組み立てる。
func newMenu(name string, g domain.Genre, d domain.Difficulty) domain.Menu {
	return domain.Menu{
		ID:          domain.NewMenuID(),
		Name:        name,
		NameKana:    "てすと",
		Genre:       g,
		Difficulty:  d,
		Description: name + "の説明",
	}
}

func genrePtr(g domain.Genre) *domain.Genre                { return &g }
func difficultyPtr(d domain.Difficulty) *domain.Difficulty { return &d }
