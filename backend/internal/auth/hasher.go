package auth

// Hasher はパッケージ関数をまとめ、service.PasswordHasher を満たすアダプタ。
//
// service 側のインターフェースを満たすためだけの薄い型。関数を直接注入する
// 手もあるが、複数メソッドを1つの依存としてまとめられるよう構造体にする。
// auth は service を import しない（構造的にインターフェースを満たす）。
type Hasher struct{}

// Hash は平文パスワードをハッシュ化する。
func (Hasher) Hash(plain string) (string, error) {
	return HashPassword(plain)
}

// Verify は平文がハッシュと一致するか検証する。
func (Hasher) Verify(hash, plain string) error {
	return VerifyPassword(hash, plain)
}
