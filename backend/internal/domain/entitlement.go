package domain

// 週間献立の保存上限。プランによらず一律（spec.md 2.8）。
//
// 上限値は仕様であってデータなので、DBではなくコードに置く。
// DBに置くと変更のたびにマイグレーションが要り、テストもDBの状態に依存する。
//
// サブスク撤廃前は free=10 / premium=50 に分かれていた。撤廃にあたり、
// 加入者が損をしない側（50）に寄せて1つにまとめた。
const savedWeeklyMenuLimit = 50

// Entitlement は「今この利用者が何をどれだけ使えるか」を表す。
//
// サブスク撤廃により、機能の可否はプランに依存しなくなった。それでも型と
// メソッドは残す。判定点をここ1箇所に保っておけば、将来サブスクを再開する
// ときにこのファイルを戻すだけで済み、RequirePremium や service の
// 権限チェック（呼び出し位置）を探し直さずに済むため。
type Entitlement struct {
	plan Plan
}

// NewEntitlement はプランから Entitlement を組み立てる。
func NewEntitlement(p Plan) Entitlement {
	return Entitlement{plan: p}
}

// Plan は契約プランを返す。
// premium 以外は全て free として扱う（ゼロ値・DBの想定外の値を含む）。
//
// 機能の可否には使われなくなったが、/auth/me が返すため残す。
func (e Entitlement) Plan() Plan {
	if e.plan == PlanPremium {
		return PlanPremium
	}
	return PlanFree
}

// SavedWeeklyMenuLimit は保存できる週間献立の件数を返す。
func (e Entitlement) SavedWeeklyMenuLimit() int {
	return savedWeeklyMenuLimit
}

// CanPersistShoppingList は買い物リストの差分を保存できるかを返す。
// サブスク撤廃により誰でも保存できる。
func (e Entitlement) CanPersistShoppingList() bool {
	return true
}

// CanUseWeeklyPlanning は週間献立の計画一式（提案・保存・週間の買い物リスト）を
// 使えるかを返す。サブスク撤廃により誰でも使える。
func (e Entitlement) CanUseWeeklyPlanning() bool {
	return true
}
