package handler_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// エラーの写像漏れを構造的に防ぐ。
//
// **同じ見落としを2度やっている。** domain にエラーを定義したのに problem.go の
// 写像を書き忘れると、そのエラーが handler まで届いた瞬間に 500 になる。
// フェーズ12（保存ID）とフェーズ13（食材ID）で同型の漏れが出た。
// どちらも「その経路を通るテストを書いて初めて気づく」性質で、
// 定義しただけの段階では誰も気づけない。
//
// そこで、定義と写像を突き合わせる。実行時に取れないので構文解析で見る
// （Go はパッケージ変数を名前から引けない）。

// intentionallyUnmapped は HTTP 応答にしないエラー。
//
// **ここに足すときは理由を書くこと。** 理由が書けないなら、それは
// 写像すべきエラーである可能性が高い。
var intentionallyUnmapped = map[string]string{
	// 認証の内訳は隠す。存在しないメールもパスワード違いも
	// service が ErrInvalidCredentials に丸めてから返す。
	"auth.ErrPasswordMismatch":      "service が ErrInvalidCredentials に丸める",
	"service.ErrCredentialNotFound": "同上。ユーザーの存在を推測させないため",

	// service の内部事情。外に出る前に変換される。
	"service.ErrNoCandidates":    "menu.go が domain のエラーに変換して返す",
	"service.ErrRecipeCacheMiss": "キャッシュ不在は障害ではなく通常の制御フロー",

	// JWT のクレーム由来。壊れていればセッション不正なので
	// service が ErrUserNotFound（401）に変換する。
	"domain.ErrInvalidUserID": "service が ErrUserNotFound に変換する",

	// DBから読んだ値の解釈に失敗した場合にだけ起きる。
	// リクエスト由来ではないため、500 になるのが正しい（データ不整合）。
	"domain.ErrInvalidIngredient":         "DBの値の解釈失敗。リクエスト由来ではない",
	"domain.ErrInvalidIngredientCategory": "同上",
	"domain.ErrInvalidRecipeLink":         "外部APIの応答の解釈失敗。リクエスト由来ではない",
	"domain.ErrInvalidSearchMode":         "DBの値の解釈失敗。リクエスト由来ではない",

	// 現状どのハンドラも本文から食材IDを解釈しない。
	// 解釈する経路が増えたら写像すること（写像済みでもこのテストは通る）。
	"domain.ErrInvalidIngredientID": "リクエスト由来の経路が無い",
}

// mappedErrors は problem.go の写像表に載っているエラーを "pkg.Name" で返す。
func mappedErrors(t *testing.T) map[string]bool {
	t.Helper()

	f, err := parser.ParseFile(token.NewFileSet(), "problem.go", nil, 0)
	require.NoError(t, err, "problem.go を解析できること")

	mapped := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, e := range cl.Elts {
			// 各要素は {err, status, typ, title} の並び。先頭がエラー。
			entry, ok := e.(*ast.CompositeLit)
			if !ok || len(entry.Elts) == 0 {
				continue
			}
			sel, ok := entry.Elts[0].(*ast.SelectorExpr)
			if !ok {
				continue
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				continue
			}
			mapped[pkg.Name+"."+sel.Sel.Name] = true
		}
		return true
	})
	return mapped
}

// definedErrors は指定パッケージが公開しているエラー変数を "pkg.Name" で返す。
func definedErrors(t *testing.T, pkgs ...string) map[string]bool {
	t.Helper()

	defined := map[string]bool{}
	for _, pkg := range pkgs {
		dir := "../" + pkg
		// parser.ParseDir は Go 1.25 で非推奨。代替の go/packages は
		// 依存が増えるうえ型検査まで走るので、ここでは自分で走査する
		// （見たいのは同一ディレクトリの .go だけで、ビルドタグも絡まない）。
		entries, err := os.ReadDir(dir)
		require.NoError(t, err, "%s を読めること", pkg)

		found := 0
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, 0)
			require.NoError(t, err, "%s を解析できること", name)
			found++

			for _, d := range file.Decls {
				gd, ok := d.(*ast.GenDecl)
				if !ok || gd.Tok != token.VAR {
					continue
				}
				for _, spec := range gd.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, n := range vs.Names {
						// 命名規約（Err で始まる公開変数）で拾う。
						if n.IsExported() && strings.HasPrefix(n.Name, "Err") {
							defined[pkg+"."+n.Name] = true
						}
					}
				}
			}
		}
		require.NotZero(t, found, "%s に解析対象の .go が無い", pkg)
	}
	return defined
}

func TestProblemMapping_全てのエラーが写像されるか意図的に除外されている(t *testing.T) {
	t.Parallel()

	mapped := mappedErrors(t)
	defined := definedErrors(t, "domain", "service", "auth")

	// 解析が壊れていないことを先に確かめる。0件なら以降の検査は無意味になる。
	require.NotEmpty(t, mapped, "写像表を読み取れていない")
	require.NotEmpty(t, defined, "エラー定義を読み取れていない")

	for name := range defined {
		if mapped[name] {
			continue
		}
		reason, excluded := intentionallyUnmapped[name]
		assert.True(t, excluded,
			"%s が problem.go に写像されていない。"+
				"handler まで届くと 500 になる。写像を足すか、"+
				"HTTP 応答にしない理由を intentionallyUnmapped に書くこと", name)
		if excluded {
			assert.NotEmpty(t, reason, "%s の除外理由が空", name)
		}
	}
}

func TestProblemMapping_除外リストに存在しないエラーが残っていない(t *testing.T) {
	t.Parallel()

	// エラーを消したり改名したりしたときに、除外リストだけ残るのを防ぐ。
	// 残っていると「意図的に除外している」ように見えて実態と食い違う。
	defined := definedErrors(t, "domain", "service", "auth")

	for name := range intentionallyUnmapped {
		assert.True(t, defined[name],
			"%s は除外リストにあるが、もう定義されていない。リストから消すこと", name)
	}
}
