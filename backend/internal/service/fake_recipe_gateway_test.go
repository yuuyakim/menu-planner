package service_test

import (
	"context"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

var _ service.RecipeSearchGateway = (*fakeRecipeGateway)(nil)

// fakeRecipeGateway は外部検索APIの代わりに定型のリンクを返す。
type fakeRecipeGateway struct {
	links []domain.RecipeLink
	err   error
	// lastMenuName / lastLimit は最後に Search に渡された値。
	lastMenuName string
	lastLimit    int
	calls        int
	// block が非nilなら、閉じられるまで Search が返らない（締め切りの検証用）。
	block chan struct{}
}

func newFakeRecipeGateway(links ...domain.RecipeLink) *fakeRecipeGateway {
	return &fakeRecipeGateway{links: links}
}

func (g *fakeRecipeGateway) Search(ctx context.Context, menuName string, limit int) ([]domain.RecipeLink, error) {
	g.calls++
	g.lastMenuName = menuName
	g.lastLimit = limit

	if g.block != nil {
		// 呼び出し側の締め切りが先に来るかを確かめるため、待たされる gateway を模す。
		select {
		case <-g.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if g.err != nil {
		return nil, g.err
	}
	return g.links, nil
}

// newRecipeLink はテスト用のリンクを組み立てる。
func newRecipeLink(title, url string) domain.RecipeLink {
	link, err := domain.NewRecipeLink(title, url, title+"の説明")
	if err != nil {
		panic(err)
	}
	return link
}
