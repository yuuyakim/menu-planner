package domain

// 週間献立の保存上限。プランごとの値（spec.md 2.8 / 2.11）。
//
// 上限値は仕様であってデータなので、DBではなくコードに置く。
// DBに置くと変更のたびにマイグレーションが要り、テストもDBの状態に依存する。
const (
	freeSavedWeeklyMenuLimit    = 10
	premiumSavedWeeklyMenuLimit = 50
)

// Entitlement は「今この利用者が何をどれだけ使えるか」を表す。
//
// **上限をフィールドではなくメソッドで導出するのがこの型の要点。**
// 仮に SavedWeeklyMenuLimit を int のフィールドで持たせると、取得し忘れた
// Entitlement{} のゼロ値が「上限0件」を意味し、既存利用者が1件も保存できなくなる。
// plan を非公開にしてメソッドで導出すれば、ゼロ値の plan は空文字となり
// free と同じ扱いに落ちる。安全側の既定を型で保証する。
type Entitlement struct {
	plan Plan
}

// NewEntitlement はプランから Entitlement を組み立てる。
func NewEntitlement(p Plan) Entitlement {
	return Entitlement{plan: p}
}

// Plan は契約プランを返す。
// premium 以外は全て free として扱う（ゼロ値・DBの想定外の値を含む）。
func (e Entitlement) Plan() Plan {
	if e.plan == PlanPremium {
		return PlanPremium
	}
	return PlanFree
}

// SavedWeeklyMenuLimit は保存できる週間献立の件数を返す。
func (e Entitlement) SavedWeeklyMenuLimit() int {
	if e.Plan() == PlanPremium {
		return premiumSavedWeeklyMenuLimit
	}
	return freeSavedWeeklyMenuLimit
}

// CanPersistShoppingList は買い物リストの差分を保存できるかを返す。
//
// premium だけが true。ゼロ値の Entitlement は Plan() が free に落ちるため
// false になり、取得し忘れても永続化の権限は漏れない（false 側が安全）。
func (e Entitlement) CanPersistShoppingList() bool {
	return e.Plan() == PlanPremium
}

// CanUseWeeklyPlanning は週間献立の計画一式（提案・保存・週間の買い物リスト）を
// 使えるかを返す。premium だけ true。ゼロ値は free に落ちるため false（安全側）。
func (e Entitlement) CanUseWeeklyPlanning() bool {
	return e.Plan() == PlanPremium
}
