// Package migrations はマイグレーションSQLをバイナリに埋め込む。
// 本番のコンテナ(distroless)にSQLファイルを配置しなくてよく、
// テスト(testcontainers)からも同じ定義を再利用できる。
package migrations

import "embed"

// FS はマイグレーションSQLを保持する。
//
//go:embed *.sql
var FS embed.FS
