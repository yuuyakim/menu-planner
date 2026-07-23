package domain

import "errors"

// ErrInvalidRole は文字列が既知の役割に一致しないことを表す。
var ErrInvalidRole = errors.New("不正な役割です")

// Role は献立が一食の中で担う役割を表す（spec.md 2.10）。
//
// difficulty が「手間」を表すのに対し、こちらは「単品で夕食が成立するか」を分ける。
// 晩ごはんを決めようとして副菜や汁物が単品で提案されると必ず引き直しになるため、
// 検索の絞り込みでこれを避けられるようにする。
type Role string

// 定義済みの役割。DBの role カラムに格納される値と一致する。
const (
	// RoleMain は主菜。丼・麺・鍋・定食のように単品で夕食が成立するものを含む。
	RoleMain Role = "main"
	// RoleSide は副菜。サラダや小鉢のように、それだけでは夕食にならないもの。
	RoleSide Role = "side"
	// RoleSoup は汁物。副菜と分けているのは「主菜＋副菜＋汁物」の組を
	// 後から作れるようにするため（2値だとスープが2品並びうる）。
	RoleSoup Role = "soup"
)

// AllRoles は全役割を返す。既定である main を先頭に置く（spec.md 2.10）。
func AllRoles() []Role {
	return []Role{RoleMain, RoleSide, RoleSoup}
}

// ParseRole は文字列を Role に変換する。
//
// APIの絞り込みで使う "all" は受け付けない。あれは「絞り込まない」ことを
// 明示するための値で、献立が持つ役割ではない（spec.md 5.1）。
// 献立側で解釈できてしまうと DB の CHECK 制約と食い違う。
func ParseRole(s string) (Role, error) {
	r := Role(s)
	if !r.Valid() {
		return "", ErrInvalidRole
	}
	return r, nil
}

// Valid は定義済みの役割かどうかを返す。
func (r Role) Valid() bool {
	switch r {
	case RoleMain, RoleSide, RoleSoup:
		return true
	default:
		return false
	}
}

// String は DB およびAPIで用いる文字列表現を返す。
func (r Role) String() string {
	return string(r)
}
