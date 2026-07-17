package domain

// WeekLength は週間献立の日数（spec.md 2.2）。
const WeekLength = 7

// DayMenu は週間献立の1日分。
//
// 曜日ではなく起点からの通し番号で表す。週間献立は作成した日を起点とする
// 当日起点のため（spec.md 13.3）、何曜日かは呼び出し側で決まる。
// サーバが曜日を持たないので、タイムゾーンの解釈がずれても中身は変わらない。
type DayMenu struct {
	// Day は起点から数えた日。1 が起点当日で、7 まで。
	Day int
	// Menu はその日の献立。
	Menu Menu
}
