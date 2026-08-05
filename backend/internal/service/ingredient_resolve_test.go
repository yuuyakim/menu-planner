package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// countingResolver は「何回呼ばれたか」を数えるスタブ。
// ①②で解けたときに Gateway が呼ばれないことを検証するために使う。
type countingResolver struct {
	calls   int
	mapping map[string]string
	err     error
	// lastWords は最後に問い合わされた語。①②で解けた語まで
	// 渡していないことを検証するために記録する。
	lastWords []string
}

func (r *countingResolver) Resolve(
	ctx context.Context, words []string, _ []string,
) ([]service.GatewayResolution, error) {
	r.calls++
	r.lastWords = words
	// 実際の gateway も、リクエストが Anthropic に飛んだ後でクライアントが
	// 切断されれば context canceled で返る。そのふるまいをここで模す。
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.err != nil {
		return nil, r.err
	}
	out := make([]service.GatewayResolution, 0, len(words))
	for _, w := range words {
		out = append(out, service.GatewayResolution{Word: w, Name: r.mapping[w]})
	}
	return out, nil
}

// fakeResolutionRepo は解決キャッシュの最小のインメモリ実装。
type fakeResolutionRepo struct {
	data  map[string]*domain.IngredientID
	saved []string
}

func (r *fakeResolutionRepo) FindByWords(
	_ context.Context, words []string,
) (map[string]*domain.IngredientID, error) {
	out := map[string]*domain.IngredientID{}
	for _, w := range words {
		if v, ok := r.data[w]; ok {
			out[w] = v
		}
	}
	return out, nil
}

func (r *fakeResolutionRepo) Save(
	_ context.Context, word string, id *domain.IngredientID,
) error {
	if r.data == nil {
		r.data = map[string]*domain.IngredientID{}
	}
	r.data[word] = id
	r.saved = append(r.saved, word)
	return nil
}

// testCatalog はテスト用の食材マスタ。
func testCatalog(t *testing.T) []domain.Ingredient {
	t.Helper()
	mk := func(name, kana string, c domain.IngredientCategory) domain.Ingredient {
		return domain.Ingredient{
			ID: domain.NewIngredientID(), Name: name, NameKana: kana, Category: c,
		}
	}
	return []domain.Ingredient{
		mk("玉ねぎ", "たまねぎ", domain.CategoryVegetable),
		mk("豚肉", "ぶたにく", domain.CategoryMeat),
		mk("卵", "たまご", domain.CategoryDairyEgg),
	}
}

// fakeRecorder は LLM 呼び出しの実績を数えるスタブ。
type fakeRecorder struct {
	calls    int
	subjects []service.ResolveSubject
	err      error
}

func (r *fakeRecorder) Record(ctx context.Context, s service.ResolveSubject) error {
	// 実際の Increment も pgx 経由の DB 呼び出しなので、キャンセル済みの
	// context を渡されれば書き込めずに context canceled を返す。ここで
	// 同じふるまいを模すことで、context.WithoutCancel を外すと
	// 「呼ばれたのに数えられない」状態を再現できるようにする。
	if err := ctx.Err(); err != nil {
		return err
	}
	r.calls++
	r.subjects = append(r.subjects, s)
	return r.err
}

// allowAll は LLM を許可する既定のポリシー。上限そのものを見ないテストで使う。
func allowAll() service.ResolvePolicy {
	return service.ResolvePolicy{
		AllowLLM: true,
		Subject:  service.ResolveSubject{Scope: service.ScopeIP, Subject: "hash-test"},
	}
}

func TestResolve_ExactMatchOnly(t *testing.T) {
	ctx := context.Background()
	items := testCatalog(t)
	gw := &countingResolver{}
	svc := service.NewIngredientResolveService(
		&fakeIngredientRepo{all: items}, &fakeResolutionRepo{}, gw, &fakeRecorder{})

	t.Run("全語が完全一致ならGatewayを呼ばない", func(t *testing.T) {
		got, err := svc.Resolve(ctx, "玉ねぎ、卵", allowAll())
		if err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if gw.calls != 0 {
			t.Errorf("Gateway が呼ばれています: %d回", gw.calls)
		}
		if len(got.Resolved) != 2 {
			t.Fatalf("2件解決するべきです: %+v", got.Resolved)
		}
		if len(got.Unresolved) != 0 {
			t.Errorf("未解決は0件であるべきです: %v", got.Unresolved)
		}
		if got.Degraded {
			t.Error("縮退していないのに Degraded が立っています")
		}
	})

	t.Run("カタカナ表記も完全一致で解ける", func(t *testing.T) {
		got, err := svc.Resolve(ctx, "タマネギ", allowAll())
		if err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if len(got.Resolved) != 1 || got.Resolved[0].Ingredient.Name != "玉ねぎ" {
			t.Errorf("カナ一致で解けていません: %+v", got.Resolved)
		}
	})

	t.Run("元の語をそのまま返す", func(t *testing.T) {
		got, _ := svc.Resolve(ctx, "タマネギ", allowAll())
		if got.Resolved[0].Word != "タマネギ" {
			t.Errorf("利用者が書いた語を返すべきです: %q", got.Resolved[0].Word)
		}
	})

	t.Run("重複した語は1件にまとめる", func(t *testing.T) {
		got, _ := svc.Resolve(ctx, "玉ねぎ、たまねぎ", allowAll())
		if len(got.Resolved) != 1 {
			t.Errorf("同じ食材は1件にまとめるべきです: %+v", got.Resolved)
		}
	})

	t.Run("マスタに無い語は未解決に落ちる", func(t *testing.T) {
		got, err := svc.Resolve(ctx, "玉ねぎ、マツタケ", allowAll())
		if err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if len(got.Resolved) != 1 {
			t.Errorf("解決できた分は返すべきです: %+v", got.Resolved)
		}
		if len(got.Unresolved) != 1 || got.Unresolved[0] != "マツタケ" {
			t.Errorf("元の語のまま未解決に落とすべきです: %v", got.Unresolved)
		}
	})

	t.Run("空テキストはエラー", func(t *testing.T) {
		_, err := svc.Resolve(ctx, "  、 ", allowAll())
		if !errors.Is(err, service.ErrEmptyResolveText) {
			t.Errorf("ErrEmptyResolveText を返すべきです: %v", err)
		}
	})
}

func TestResolve_Gateway(t *testing.T) {
	ctx := context.Background()
	items := testCatalog(t)

	newSvc := func(gw *countingResolver, cache *fakeResolutionRepo) *service.IngredientResolveService {
		return service.NewIngredientResolveService(&fakeIngredientRepo{all: items}, cache, gw, &fakeRecorder{})
	}

	t.Run("未解決語だけがGatewayに渡る", func(t *testing.T) {
		gw := &countingResolver{mapping: map[string]string{"豚こま": "豚肉"}}
		got, err := newSvc(gw, &fakeResolutionRepo{}).Resolve(ctx, "玉ねぎ、豚こま", allowAll())
		if err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if gw.calls != 1 {
			t.Errorf("Gateway は1回だけ呼ばれるべきです: %d回", gw.calls)
		}
		if len(gw.lastWords) != 1 || gw.lastWords[0] != "豚こま" {
			t.Errorf("完全一致で解けた語まで渡してはいけません: %v", gw.lastWords)
		}
		if len(got.Resolved) != 2 {
			t.Errorf("2件解決するべきです: %+v", got.Resolved)
		}
	})

	t.Run("解決結果はキャッシュに保存される", func(t *testing.T) {
		gw := &countingResolver{mapping: map[string]string{"豚こま": "豚肉"}}
		cache := &fakeResolutionRepo{}
		if _, err := newSvc(gw, cache).Resolve(ctx, "豚こま", allowAll()); err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if len(cache.saved) != 1 || cache.saved[0] != "豚こま" {
			t.Errorf("正規化済みの語で保存するべきです: %v", cache.saved)
		}
	})

	t.Run("未解決(該当なし)もキャッシュに保存される", func(t *testing.T) {
		gw := &countingResolver{mapping: map[string]string{}}
		cache := &fakeResolutionRepo{}
		got, _ := newSvc(gw, cache).Resolve(ctx, "マツタケ", allowAll())
		if len(got.Unresolved) != 1 || got.Unresolved[0] != "マツタケ" {
			t.Errorf("未解決に落ちるべきです: %+v", got)
		}
		if len(cache.saved) != 1 {
			t.Error("該当なしもキャッシュしないと毎回LLMを通ってしまいます")
		}
	})

	t.Run("マスタに無い名前が返っても未解決に落ちる", func(t *testing.T) {
		gw := &countingResolver{mapping: map[string]string{"まつたけ": "存在しない食材"}}
		got, _ := newSvc(gw, &fakeResolutionRepo{}).Resolve(ctx, "マツタケ", allowAll())
		if len(got.Unresolved) != 1 {
			t.Errorf("マスタに無い名前は未解決に落とすべきです: %+v", got)
		}
		if got.Degraded {
			t.Error("ハルシネーションは障害ではないので Degraded は立てない")
		}
	})

	t.Run("Gatewayのエラーは部分成功になる", func(t *testing.T) {
		gw := &countingResolver{err: errors.New("timeout")}
		got, err := newSvc(gw, &fakeResolutionRepo{}).Resolve(ctx, "玉ねぎ、豚こま", allowAll())
		if err != nil {
			t.Fatalf("部分成功にするべきです: %v", err)
		}
		if !got.Degraded {
			t.Error("Degraded が立つべきです")
		}
		if len(got.Resolved) != 1 || got.Resolved[0].Ingredient.Name != "玉ねぎ" {
			t.Errorf("完全一致で解けた分は返すべきです: %+v", got.Resolved)
		}
		if len(got.Unresolved) != 1 || got.Unresolved[0] != "豚こま" {
			t.Errorf("解けなかった語は未解決に入れるべきです: %+v", got.Unresolved)
		}
	})

	t.Run("Gatewayが落ちてもキャッシュには保存しない", func(t *testing.T) {
		gw := &countingResolver{err: errors.New("timeout")}
		cache := &fakeResolutionRepo{}
		if _, err := newSvc(gw, cache).Resolve(ctx, "豚こま", allowAll()); err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if len(cache.saved) != 0 {
			t.Error("失敗した問い合わせを未解決として焼き付けてはいけません")
		}
	})
}

func TestResolve_Cache(t *testing.T) {
	ctx := context.Background()
	items := testCatalog(t)
	porkID := items[1].ID // 豚肉

	t.Run("キャッシュにあればGatewayを呼ばない", func(t *testing.T) {
		gw := &countingResolver{mapping: map[string]string{"豚こま": "豚肉"}}
		cache := &fakeResolutionRepo{data: map[string]*domain.IngredientID{"豚こま": &porkID}}
		svc := service.NewIngredientResolveService(&fakeIngredientRepo{all: items}, cache, gw, &fakeRecorder{})

		got, err := svc.Resolve(ctx, "豚こま", allowAll())
		if err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if gw.calls != 0 {
			t.Errorf("Gateway が呼ばれています: %d回", gw.calls)
		}
		if len(got.Resolved) != 1 || got.Resolved[0].Ingredient.Name != "豚肉" {
			t.Errorf("キャッシュから解決するべきです: %+v", got.Resolved)
		}
	})

	t.Run("未解決と確定済みならGatewayを呼ばない", func(t *testing.T) {
		gw := &countingResolver{mapping: map[string]string{}}
		cache := &fakeResolutionRepo{data: map[string]*domain.IngredientID{"まつたけ": nil}}
		svc := service.NewIngredientResolveService(&fakeIngredientRepo{all: items}, cache, gw, &fakeRecorder{})

		got, _ := svc.Resolve(ctx, "マツタケ", allowAll())
		if gw.calls != 0 {
			t.Errorf("該当なしと確定済みなら聞き直すべきではありません: %d回", gw.calls)
		}
		if len(got.Unresolved) != 1 {
			t.Errorf("未解決に入るべきです: %+v", got)
		}
	})

	t.Run("キャッシュに無い語だけがGatewayに渡る", func(t *testing.T) {
		gw := &countingResolver{mapping: map[string]string{"牛こま": "豚肉"}}
		cache := &fakeResolutionRepo{data: map[string]*domain.IngredientID{"豚こま": &porkID}}
		svc := service.NewIngredientResolveService(&fakeIngredientRepo{all: items}, cache, gw, &fakeRecorder{})

		if _, err := svc.Resolve(ctx, "豚こま、牛こま", allowAll()); err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if gw.calls != 1 {
			t.Errorf("Gateway は1回だけ呼ばれるべきです: %d回", gw.calls)
		}
		if len(gw.lastWords) != 1 || gw.lastWords[0] != "牛こま" {
			t.Errorf("キャッシュで解けた語まで渡してはいけません: %v", gw.lastWords)
		}
	})
}

func TestResolve_GatewayErrorSetsReason(t *testing.T) {
	ctx := context.Background()
	items := testCatalog(t)
	gw := &countingResolver{err: errors.New("LLMが落ちました")}
	svc := service.NewIngredientResolveService(
		&fakeIngredientRepo{all: items}, &fakeResolutionRepo{}, gw, &fakeRecorder{})

	got, err := svc.Resolve(ctx, "マツタケ", allowAll())
	if err != nil {
		t.Fatalf("Resolve が失敗しました: %v", err)
	}
	if !got.Degraded {
		t.Fatal("degraded が立っていません")
	}
	if got.Reason != service.ReasonLLMError {
		t.Errorf("理由が llm_error ではありません: %q", got.Reason)
	}
}

func TestResolve_DeniedPolicySkipsGateway(t *testing.T) {
	ctx := context.Background()
	items := testCatalog(t)
	gw := &countingResolver{}
	rec := &fakeRecorder{}
	svc := service.NewIngredientResolveService(
		&fakeIngredientRepo{all: items}, &fakeResolutionRepo{}, gw, rec)

	got, err := svc.Resolve(ctx, "玉ねぎ、マツタケ", service.ResolvePolicy{
		AllowLLM:   false,
		DenyReason: service.ReasonAnonDailyLimit,
		Subject:    service.ResolveSubject{Scope: service.ScopeIP, Subject: "hash-a"},
	})
	if err != nil {
		t.Fatalf("Resolve が失敗しました: %v", err)
	}

	if gw.calls != 0 {
		t.Errorf("上限に達しているのに Gateway が呼ばれています: %d回", gw.calls)
	}
	if rec.calls != 0 {
		t.Errorf("呼んでいないのに実績が数えられています: %d回", rec.calls)
	}
	// ①で解けた分は返す。機能全体を落とさない。
	if len(got.Resolved) != 1 || got.Resolved[0].Word != "玉ねぎ" {
		t.Errorf("完全一致の結果が返っていません: %+v", got.Resolved)
	}
	if !got.Degraded || got.Reason != service.ReasonAnonDailyLimit {
		t.Errorf("理由が渡っていません: degraded=%v reason=%q", got.Degraded, got.Reason)
	}
	if len(got.Unresolved) != 1 || got.Unresolved[0] != "マツタケ" {
		t.Errorf("未解決語が返っていません: %+v", got.Unresolved)
	}
}

func TestResolve_RecordsWhenGatewayCalled(t *testing.T) {
	ctx := context.Background()
	items := testCatalog(t)
	gw := &countingResolver{mapping: map[string]string{"まつたけ": ""}}
	rec := &fakeRecorder{}
	svc := service.NewIngredientResolveService(
		&fakeIngredientRepo{all: items}, &fakeResolutionRepo{}, gw, rec)

	subject := service.ResolveSubject{Scope: service.ScopeUser, Subject: "user-1"}
	if _, err := svc.Resolve(ctx, "マツタケ", service.ResolvePolicy{
		AllowLLM: true, Subject: subject,
	}); err != nil {
		t.Fatalf("Resolve が失敗しました: %v", err)
	}

	if rec.calls != 1 {
		t.Fatalf("1回数えるはずです: %d回", rec.calls)
	}
	if rec.subjects[0] != subject {
		t.Errorf("キーが違います: %+v", rec.subjects[0])
	}
}

func TestResolve_RecordsEvenWhenGatewayFails(t *testing.T) {
	ctx := context.Background()
	items := testCatalog(t)
	gw := &countingResolver{err: errors.New("LLMが落ちました")}
	rec := &fakeRecorder{}
	svc := service.NewIngredientResolveService(
		&fakeIngredientRepo{all: items}, &fakeResolutionRepo{}, gw, rec)

	if _, err := svc.Resolve(ctx, "マツタケ", allowAll()); err != nil {
		t.Fatalf("Resolve が失敗しました: %v", err)
	}
	// 失敗してもトークンは消費されている（設計 4章）。
	if rec.calls != 1 {
		t.Errorf("失敗した呼び出しも数えるはずです: %d回", rec.calls)
	}
}

func TestResolve_RecordsEvenWhenContextCancelled(t *testing.T) {
	items := testCatalog(t)
	gw := &countingResolver{}
	rec := &fakeRecorder{}
	svc := service.NewIngredientResolveService(
		&fakeIngredientRepo{all: items}, &fakeResolutionRepo{}, gw, rec)

	// クライアントが接続を切った後の Echo と同じ状態を作る。
	// gateway.Resolve が呼ばれた時点でAnthropicへのリクエストはもう飛んでおり、
	// トークンは消費済み。ここでキャンセルを理由に加算まで諦めると、
	// 課金だけ発生して枠が一切減らない抜け道になる。
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := svc.Resolve(ctx, "マツタケ", allowAll()); err != nil {
		t.Fatalf("Resolve が失敗しました: %v", err)
	}
	if gw.calls != 1 {
		t.Fatalf("Gateway は呼ばれるはずです: %d回", gw.calls)
	}
	if rec.calls != 1 {
		t.Errorf("接続が切れていても実績は数えるはずです: %d回", rec.calls)
	}
}

func TestResolve_ExactMatchDoesNotRecord(t *testing.T) {
	ctx := context.Background()
	items := testCatalog(t)
	rec := &fakeRecorder{}
	svc := service.NewIngredientResolveService(
		&fakeIngredientRepo{all: items}, &fakeResolutionRepo{}, &countingResolver{}, rec)

	if _, err := svc.Resolve(ctx, "玉ねぎ、卵", allowAll()); err != nil {
		t.Fatalf("Resolve が失敗しました: %v", err)
	}
	// 料金が発生しない解決で枠を消さない（設計 4章）。
	if rec.calls != 0 {
		t.Errorf("完全一致だけなら数えないはずです: %d回", rec.calls)
	}
}

func TestResolve_RecordFailureDoesNotBreakResult(t *testing.T) {
	ctx := context.Background()
	items := testCatalog(t)
	// mapping のキーは正規化後の語。NormalizeIngredientWord はカタカナを
	// ひらがなにするだけなので、「豚こま」はそのまま「豚こま」で引ける。
	gw := &countingResolver{mapping: map[string]string{"豚こま": "豚肉"}}
	rec := &fakeRecorder{err: errors.New("DBが落ちました")}
	svc := service.NewIngredientResolveService(
		&fakeIngredientRepo{all: items}, &fakeResolutionRepo{}, gw, rec)

	got, err := svc.Resolve(ctx, "豚こま", allowAll())
	if err != nil {
		t.Fatalf("加算の失敗で機能を止めてはいけません: %v", err)
	}
	// 呼び出しはもう済んでいる。数え漏れは許容する（設計 9.2）。
	if len(got.Resolved) != 1 {
		t.Errorf("解決結果が返っていません: %+v", got.Resolved)
	}
	if got.Degraded {
		t.Error("加算の失敗で縮退させてはいけません")
	}
}
