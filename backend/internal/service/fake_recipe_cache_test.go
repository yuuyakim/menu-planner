package service_test

import (
	"context"
	"time"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

var _ service.RecipeLinkCache = (*fakeRecipeCache)(nil)

// fakeRecipeCache はメモリ上のキャッシュ。
type fakeRecipeCache struct {
	entries map[domain.MenuID]service.CachedRecipeLinks
	// findErr が非nilなら Find が必ずこのエラーを返す（読み出し障害の再現）。
	findErr error
	// saveErr が非nilなら Save が必ずこのエラーを返す（書き込み障害の再現）。
	saveErr error

	findCalls int
	saveCalls int
	// lastSaved は最後に Save に渡された内容。
	lastSaved service.CachedRecipeLinks
}

func newFakeRecipeCache() *fakeRecipeCache {
	return &fakeRecipeCache{entries: map[domain.MenuID]service.CachedRecipeLinks{}}
}

func (c *fakeRecipeCache) Find(_ context.Context, id domain.MenuID) (service.CachedRecipeLinks, error) {
	c.findCalls++
	if c.findErr != nil {
		return service.CachedRecipeLinks{}, c.findErr
	}
	entry, ok := c.entries[id]
	if !ok {
		return service.CachedRecipeLinks{}, service.ErrRecipeCacheMiss
	}
	return entry, nil
}

func (c *fakeRecipeCache) Save(_ context.Context, id domain.MenuID, links []domain.RecipeLink, fetchedAt time.Time) error {
	c.saveCalls++
	if c.saveErr != nil {
		return c.saveErr
	}
	entry := service.CachedRecipeLinks{Links: links, FetchedAt: fetchedAt}
	c.entries[id] = entry
	c.lastSaved = entry
	return nil
}

// put は既にキャッシュがある状態を作る。
func (c *fakeRecipeCache) put(id domain.MenuID, fetchedAt time.Time, links ...domain.RecipeLink) {
	c.entries[id] = service.CachedRecipeLinks{Links: links, FetchedAt: fetchedAt}
}
