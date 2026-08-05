package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// GatewayResolution は1語ぶんの対応づけ結果。
type GatewayResolution struct {
	// Word は問い合わせた語（正規化済み）。
	Word string
	// Name は対応づいた食材名。**空文字は「該当なし」を表す。**
	Name string
}

// IngredientResolveGateway は未解決語を食材名に対応づける外部サービス。
//
// RecipeSearchGateway（spec.md 3.3）と同じく、実装差し替えのみで
// プロバイダを切り替えられる状態にする。
//
// **食材IDではなく食材名をやり取りする**（設計 3.5）。UUID は36文字あり
// 166件で約3000トークンに達するうえ、1文字の取り違えが解決失敗になる。
// 名前で受けてアプリ側でIDに引き直せば、マスタに無い名前が返っても
// 「該当なし」に落ちるだけで済む。
type IngredientResolveGateway interface {
	// Resolve は words を catalog のいずれかの食材名に対応づける。
	// 戻り値は words と同じ件数・同じ順序であることを期待する。
	Resolve(ctx context.Context, words []string, catalog []string) ([]GatewayResolution, error)
}

// ErrEmptyResolveText は解決すべき語が1つも無いことを表す（400）。
var ErrEmptyResolveText = errors.New("食材のテキストが空です")

// ResolutionRepository は入力語から食材への解決キャッシュ。
//
// 値が nil は「マスタに無いと確定済み」、キーが無いことは
// 「まだ問い合わせていない」を表す。
type ResolutionRepository interface {
	FindByWords(ctx context.Context, words []string) (map[string]*domain.IngredientID, error)
	Save(ctx context.Context, word string, id *domain.IngredientID) error
}

// ResolvedWord は1語ぶんの解決結果。
type ResolvedWord struct {
	// Word は利用者が書いた**元の語**（正規化前）。
	// 画面で「マツタケ は登録がありません」と出すとき、書いた通りの
	// 文字列の方が伝わるため。
	Word string
	// Ingredient は対応づいた食材。
	Ingredient domain.Ingredient
}

// DegradedReason は LLM への問い合わせをスキップした理由（設計 8章）。
//
// bool ひとつでは「LLM が落ちた」と「上限に達した」を区別できず、
// 画面が出す文言を選べない。値と文言は1対1ではない
// （counter_unavailable は llm_error と同じ文言を出す）。
type DegradedReason string

const (
	// ReasonLLMError は LLM への問い合わせが失敗したことを表す。
	ReasonLLMError DegradedReason = "llm_error"
	// ReasonCounterUnavailable は日次カウンタが読めずフェイルクローズしたことを表す（設計 9.1）。
	ReasonCounterUnavailable DegradedReason = "counter_unavailable"
	// ReasonAnonDailyLimit は非ログインの日次上限に達したことを表す。
	ReasonAnonDailyLimit DegradedReason = "anon_daily_limit"
	// ReasonUserDailyLimit はログインユーザーの日次上限に達したことを表す。
	ReasonUserDailyLimit DegradedReason = "user_daily_limit"
	// ReasonServiceDailyLimit はサービス全体の日次上限に達したことを表す。
	ReasonServiceDailyLimit DegradedReason = "service_daily_limit"
)

// ResolveResult はテキスト1件ぶんの解決結果。
type ResolveResult struct {
	// Resolved は食材に対応づいた語。
	Resolved []ResolvedWord
	// Unresolved はマスタに無かった語（元の語）。検索には使われない。
	Unresolved []string
	// Degraded は LLM への問い合わせをスキップしたことを表す（設計 3.6）。
	// 立っていても①②で解けた分は Resolved に入っている。
	Degraded bool
	// Reason は Degraded が立った理由。Degraded が false なら空。
	Reason DegradedReason
}

// ResolveRecorder は LLM を呼んだ実績を数える。
//
// 判定（ResolveQuota.Check）は handler が行い、記録だけを service が持つ。
// LLM を呼んだかどうかは③まで来て初めて分かるため（設計 5章）。
type ResolveRecorder interface {
	Record(ctx context.Context, s ResolveSubject) error
}

// ResolvePolicy は1リクエストぶんの LLM 利用の可否（設計 5章）。
type ResolvePolicy struct {
	// AllowLLM が false なら③をスキップする。
	AllowLLM bool
	// DenyReason は AllowLLM が false のときの理由。結果の Reason にそのまま載る。
	DenyReason DegradedReason
	// Subject は実績を数えるときのキー。
	Subject ResolveSubject
}

// IngredientResolveService は手持ちの食材テキストを食材IDに解決する。
//
// IngredientService に相乗りさせていないのは、あちらが「食材マスタそのもの」と
// 「食材からの献立検索」を担っており、こちらは外部I/Oを2つ（キャッシュ・LLM）
// 新たに抱えるため。同居させると既存機能のテストに LLM スタブが必要になる。
type IngredientResolveService struct {
	ingredients IngredientRepository
	cache       ResolutionRepository
	gateway     IngredientResolveGateway
	recorder    ResolveRecorder
}

// NewIngredientResolveService は IngredientResolveService を生成する。
func NewIngredientResolveService(
	ingredients IngredientRepository,
	cache ResolutionRepository,
	gw IngredientResolveGateway,
	rec ResolveRecorder,
) *IngredientResolveService {
	return &IngredientResolveService{
		ingredients: ingredients, cache: cache, gateway: gw, recorder: rec,
	}
}

// resolveEntry は解決の途中経過。元の語と正規化語の対応を保つ。
type resolveEntry struct {
	original   string
	normalized string
}

// Resolve はテキストを食材に対応づける（設計 3.3）。
//
//	① 食材マスタとの完全一致
//	② 解決キャッシュ
//	③ LLM
//
// **全語が①で解ければ LLM を一度も呼ばない。**
func (s *IngredientResolveService) Resolve(
	ctx context.Context, text string, policy ResolvePolicy,
) (ResolveResult, error) {
	entries := buildResolveEntries(text)
	if len(entries) == 0 {
		return ResolveResult{}, fmt.Errorf("%w: 1語以上を指定してください", ErrEmptyResolveText)
	}

	items, err := s.ingredients.FindAll(ctx)
	if err != nil {
		return ResolveResult{}, fmt.Errorf("食材マスタの取得に失敗しました: %w", err)
	}
	byName := buildNameIndex(items)

	result := ResolveResult{Resolved: []ResolvedWord{}, Unresolved: []string{}}
	seen := map[domain.IngredientID]bool{}

	pending := make([]resolveEntry, 0, len(entries))
	for _, e := range entries {
		if ing, ok := byName[e.normalized]; ok {
			// ① 完全一致。同じ食材に落ちる語が複数あっても1件にまとめる。
			if !seen[ing.ID] {
				seen[ing.ID] = true
				result.Resolved = append(result.Resolved, ResolvedWord{Word: e.original, Ingredient: ing})
			}
			continue
		}
		pending = append(pending, e)
	}

	if len(pending) == 0 {
		// 全語が①で解けた。LLM を呼ばずに返す。
		return result, nil
	}

	// ② 解決キャッシュ。①で解けなかった語だけを引く。
	pending = s.applyCache(ctx, pending, byName, seen, &result)
	if len(pending) == 0 {
		// キャッシュで全部片付いた。LLM を呼ばずに返す。
		return result, nil
	}

	if !policy.AllowLLM {
		// 上限に達している、またはカウンタが読めない。③をスキップする。
		// ①②で解けた分は返す。よくある食材はここまでで通るため、
		// 機能が丸ごと死んだようには見えない（設計 5章）。
		result.Degraded = true
		result.Reason = policy.DenyReason
		for _, e := range pending {
			result.Unresolved = append(result.Unresolved, e.original)
		}
		return result, nil
	}

	s.resolveByGateway(ctx, pending, byName, items, seen, &result, policy.Subject)
	return result, nil
}

// applyCache はキャッシュで解決できる語を result に足し、残りを返す。
//
// キャッシュの引きに失敗しても機能は止めない。③で聞き直すだけで、
// 余分なコストがかかるものの結果は同じになる。
func (s *IngredientResolveService) applyCache(
	ctx context.Context,
	pending []resolveEntry,
	byName map[string]domain.Ingredient,
	seen map[domain.IngredientID]bool,
	result *ResolveResult,
) []resolveEntry {
	words := make([]string, 0, len(pending))
	for _, e := range pending {
		words = append(words, e.normalized)
	}

	cached, err := s.cache.FindByWords(ctx, words)
	if err != nil {
		slog.WarnContext(ctx, "解決キャッシュの取得に失敗しました", "error", err)
		return pending
	}

	rest := make([]resolveEntry, 0, len(pending))
	for _, e := range pending {
		id, ok := cached[e.normalized]
		if !ok {
			// まだ問い合わせていない語。③へ回す。
			rest = append(rest, e)
			continue
		}
		if id == nil {
			// 「マスタに無い」と確定済み。聞き直さない。
			result.Unresolved = append(result.Unresolved, e.original)
			continue
		}
		ing, found := findByID(byName, *id)
		if !found {
			// 食材が消えた直後などに起こりうる。③へ回して聞き直す。
			rest = append(rest, e)
			continue
		}
		if !seen[ing.ID] {
			seen[ing.ID] = true
			result.Resolved = append(result.Resolved, ResolvedWord{Word: e.original, Ingredient: ing})
		}
	}
	return rest
}

// findByID は名前索引の中から食材IDで1件引く。
// 索引は名前引き用なので、IDでの逆引きはここで線形に探す（166件）。
func findByID(byName map[string]domain.Ingredient, id domain.IngredientID) (domain.Ingredient, bool) {
	for _, ing := range byName {
		if ing.ID == id {
			return ing, true
		}
	}
	return domain.Ingredient{}, false
}

// resolveByGateway は未解決語を LLM に問い合わせ、結果を result に足す。
//
// **エラーは返さない。** LLM の失敗で機能全体を落とさず、①②で解けた分を
// 返すため（設計 3.6）。失敗したことは result.Degraded で伝える。
func (s *IngredientResolveService) resolveByGateway(
	ctx context.Context,
	pending []resolveEntry,
	byName map[string]domain.Ingredient,
	items []domain.Ingredient,
	seen map[domain.IngredientID]bool,
	result *ResolveResult,
	subject ResolveSubject,
) {
	words := make([]string, 0, len(pending))
	for _, e := range pending {
		words = append(words, e.normalized)
	}

	answers, err := s.gateway.Resolve(ctx, words, catalogNames(items))

	// **呼んだ時点で数える。** 失敗してもトークンは消費されている（設計 4章）。
	if rerr := s.recorder.Record(ctx, subject); rerr != nil {
		// 呼び出しはもう済んでいる。ここで機能を止めても払った金は戻らない。
		// 数え漏れは許容する（設計 9.2）。
		slog.WarnContext(ctx, "読み取りカウンタの加算に失敗しました", "error", rerr)
	}

	if err != nil {
		// 縮退。未解決語はすべて Unresolved に落とす。
		// **キャッシュには保存しない。** 失敗した問い合わせを
		// 「マスタに無い」として焼き付けると、復旧後も誤りが残る。
		result.Degraded = true
		result.Reason = ReasonLLMError
		for _, e := range pending {
			result.Unresolved = append(result.Unresolved, e.original)
		}
		return
	}

	byWord := make(map[string]string, len(answers))
	for _, a := range answers {
		byWord[a.Word] = a.Name
	}

	for _, e := range pending {
		name := domain.NormalizeIngredientWord(byWord[e.normalized])
		ing, ok := byName[name]
		if !ok {
			// 空文字（該当なし）と、マスタに無い名前（ハルシネーション）が
			// ここに来る。どちらも「マスタに無い」として同じ扱いにする。
			result.Unresolved = append(result.Unresolved, e.original)
			s.saveResolution(ctx, e.normalized, nil)
			continue
		}
		id := ing.ID
		s.saveResolution(ctx, e.normalized, &id)
		if seen[id] {
			continue
		}
		seen[id] = true
		result.Resolved = append(result.Resolved, ResolvedWord{Word: e.original, Ingredient: ing})
	}
}

// catalogNames は LLM に渡す食材名の一覧を作る。
// **name のみ。** カナは①の完全一致で既に効いており、③に届く語は
// カナ一致で拾えなかったものに限られるため、渡してもトークンが倍になるだけ（設計 4.2）。
func catalogNames(items []domain.Ingredient) []string {
	names := make([]string, 0, len(items))
	for _, i := range items {
		names = append(names, i.Name)
	}
	return names
}

// saveResolution はキャッシュへの保存を試みる。
// **失敗しても解決自体は成功として扱う。** キャッシュはコスト削減のための
// 仕組みであり、書けなかったことを利用者に見せる意味がない。
func (s *IngredientResolveService) saveResolution(
	ctx context.Context, word string, id *domain.IngredientID,
) {
	if err := s.cache.Save(ctx, word, id); err != nil {
		slog.WarnContext(ctx, "解決キャッシュの保存に失敗しました", "word", word, "error", err)
	}
}

// buildResolveEntries はテキストを分割し、正規化して空語を捨てる。
// 正規化後に同じになる語（「玉ねぎ」と「玉ねぎ 」）は1件にまとめる。
func buildResolveEntries(text string) []resolveEntry {
	words := domain.SplitIngredientWords(text)
	entries := make([]resolveEntry, 0, len(words))
	seen := map[string]bool{}
	for _, w := range words {
		n := domain.NormalizeIngredientWord(w)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		entries = append(entries, resolveEntry{original: w, normalized: n})
	}
	return entries
}

// buildNameIndex は正規化した name / name_kana の両方から食材を引ける索引を作る。
//
// **domain.NormalizeIngredientWord を通す。** 入力側と同じ関数を通さないと、
// ①で引けなかった語が②でも引けないという穴が開く（設計 3.4）。
//
// 166件で固定的なため、毎リクエスト組み直しても実測できるコストにならない。
func buildNameIndex(items []domain.Ingredient) map[string]domain.Ingredient {
	idx := make(map[string]domain.Ingredient, len(items)*2)
	for _, i := range items {
		idx[domain.NormalizeIngredientWord(i.Name)] = i
		// name が優先。同じキーになったら name 側を残す。
		if k := domain.NormalizeIngredientWord(i.NameKana); k != "" {
			if _, exists := idx[k]; !exists {
				idx[k] = i
			}
		}
	}
	return idx
}
