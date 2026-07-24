package repository_test

import (
	"testing"

	"github.com/yuuyakim/menu-planner/backend/internal/repository"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// MenuRepository が service.MenuRepository を満たすことをコンパイル時に保証する。
// service 側のインターフェースを変更したのに実装を直し忘れた場合、
// 実行時ではなくビルド時に気付ける。
var _ service.MenuRepository = (*repository.MenuRepository)(nil)

// SubscriptionRepository が service.SubscriptionStore を満たすことをコンパイル時に保証する。
var _ service.SubscriptionStore = (*repository.SubscriptionRepository)(nil)

// ShoppingListOverrideRepository が service.ShoppingListOverrideStore を満たすことをコンパイル時に保証する。
var _ service.ShoppingListOverrideStore = (*repository.ShoppingListOverrideRepository)(nil)

func TestMenuRepository_serviceのインターフェースを満たす(t *testing.T) {
	t.Parallel()
	// 上の var 宣言がコンパイルできれば満たしている。
}
