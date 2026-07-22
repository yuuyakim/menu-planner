package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

func TestIngredientService_All_カテゴリ順そのあとカナ順(t *testing.T) {
	t.Parallel()

	// repository はカナ順までしか保証しない。カテゴリ順は service が与える
	// （カテゴリの表示順は domain の知識で、SQL に書き写さない）。
	repo := &fakeIngredientRepo{all: []domain.Ingredient{
		ingredient("米", "こめ", domain.CategoryStaple),
		ingredient("じゃがいも", "じゃがいも", domain.CategoryVegetable),
		ingredient("鶏もも肉", "とりももにく", domain.CategoryMeat),
		ingredient("玉ねぎ", "たまねぎ", domain.CategoryVegetable),
		ingredient("鮭", "さけ", domain.CategorySeafood),
	}}
	svc := service.NewIngredientService(repo)

	got, err := svc.All(context.Background())
	require.NoError(t, err)

	names := make([]string, 0, len(got))
	for _, i := range got {
		names = append(names, i.Name)
	}
	assert.Equal(t, []string{
		"じゃがいも", "玉ねぎ", // 野菜（じゃがいも → たまねぎ）
		"鶏もも肉", // 肉
		"鮭",    // 魚介
		"米",    // 主食
	}, names)
}

func TestIngredientService_All_repositoryの並びに依存しない(t *testing.T) {
	t.Parallel()

	// repository が ORDER BY を変えても、表示順は service が決める。
	// カナ順を service 側でも指定しているのはこのため。
	repo := &fakeIngredientRepo{all: []domain.Ingredient{
		ingredient("玉ねぎ", "たまねぎ", domain.CategoryVegetable),
		ingredient("じゃがいも", "じゃがいも", domain.CategoryVegetable),
	}}
	svc := service.NewIngredientService(repo)

	got, err := svc.All(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "じゃがいも", got[0].Name)
	assert.Equal(t, "玉ねぎ", got[1].Name)
}

func TestIngredientService_All_0件でもnilを返さない(t *testing.T) {
	t.Parallel()

	repo := &fakeIngredientRepo{all: []domain.Ingredient{}}
	svc := service.NewIngredientService(repo)

	got, err := svc.All(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestIngredientService_All_取得の失敗はそのまま返す(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("DB障害")
	repo := &fakeIngredientRepo{err: sentinel}
	svc := service.NewIngredientService(repo)

	_, err := svc.All(context.Background())
	assert.ErrorIs(t, err, sentinel)
}
