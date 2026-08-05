package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// ErrMissingResolverAPIKey はAPIキーが設定されていないことを表す。
var ErrMissingResolverAPIKey = errors.New("食材解決のAPIキーが未設定です")

// claudeResolverTimeout は1回の問い合わせの上限（設計 4.3）。
// 利用者は「読み取る」を押して待っているため、長く待たせるより
// 縮退して手動チェックに促す方がよい。
const claudeResolverTimeout = 3 * time.Second

// claudeResolverModel は使用するモデル。
//
// この対応づけはフロンティアモデルの知性を必要としないため、
// 最も安い層を選ぶ。プロバイダの最終決定は eval の数字で行う（設計 3.7）。
const claudeResolverModel = "claude-haiku-4-5"

// claudeMaxTokens は応答の上限。20語 × 1件あたり十数トークンで足りる。
const claudeMaxTokens = 1024

// ClaudeResolver は Claude で未解決語を食材名に対応づける。
type ClaudeResolver struct {
	client anthropic.Client
}

// NewClaudeResolver は ClaudeResolver を生成する。
func NewClaudeResolver(apiKey string) (*ClaudeResolver, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, ErrMissingResolverAPIKey
	}
	return &ClaudeResolver{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
	}, nil
}

// resolverAnswer は LLM に返させるJSONの1件。
type resolverAnswer struct {
	Word string `json:"word"`
	Name string `json:"name"`
}

// Resolve は未解決語を食材名に対応づける。
//
// **食材IDではなく名前をやり取りする**（設計 3.5）。UUID を渡すと
// 166件で約3000トークンに達し、1文字の取り違えが解決失敗になる。
// 名前で受ければ、マスタに無い名前が返っても呼び出し側で「該当なし」に落ちる。
func (r *ClaudeResolver) Resolve(
	ctx context.Context, words []string, catalog []string,
) ([]service.GatewayResolution, error) {
	if len(words) == 0 {
		return []service.GatewayResolution{}, nil
	}

	prompt := buildResolverPrompt(words, catalog)

	// **リトライは1回だけ**（設計 4.3）。利用者は「読み取る」を押して待っており、
	// 何度も粘るより縮退して手動チェックに促す方がよい。
	// JSON の解釈失敗はリトライしない（同じ応答が返る見込みが高く、待たせるだけ）。
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		text, err := r.callOnce(ctx, prompt)
		if err != nil {
			lastErr = err
			continue
		}
		answers, err := parseResolverAnswers(text)
		if err != nil {
			return nil, err
		}
		out := make([]service.GatewayResolution, 0, len(answers))
		for _, a := range answers {
			out = append(out, service.GatewayResolution{Word: a.Word, Name: a.Name})
		}
		return out, nil
	}
	return nil, lastErr
}

// callOnce は1回だけ問い合わせ、本文のテキストを連結して返す。
func (r *ClaudeResolver) callOnce(ctx context.Context, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, claudeResolverTimeout)
	defer cancel()

	resp, err := r.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     claudeResolverModel,
		MaxTokens: claudeMaxTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("食材解決の問い合わせに失敗しました: %w", err)
	}

	var text strings.Builder
	for _, block := range resp.Content {
		if b, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(b.Text)
		}
	}
	return text.String(), nil
}

// buildResolverPrompt は問い合わせ文を組み立てる。
//
// catalog は name のみ（設計 4.2）。カナは呼び出し側の完全一致で
// 既に効いており、ここに届く語はカナ一致で拾えなかったものに限られる。
func buildResolverPrompt(words, catalog []string) string {
	var b strings.Builder
	b.WriteString("あなたは料理アプリの食材名の正規化を担当します。\n")
	b.WriteString("利用者が書いた語を、下の食材リストのいずれか1つに対応づけてください。\n\n")
	b.WriteString("規則:\n")
	b.WriteString("- 対応する食材がリストに無ければ name を空文字にしてください。\n")
	b.WriteString("- リストに無い食材名を作らないでください。\n")
	b.WriteString("- 解釈が複数あり得る場合は、家庭料理で最も一般的なものを1つ選んでください。\n")
	b.WriteString("- JSON配列だけを出力してください。説明文は不要です。\n\n")
	b.WriteString("食材リスト:\n")
	b.WriteString(strings.Join(catalog, "\n"))
	b.WriteString("\n\n対応づける語:\n")
	b.WriteString(strings.Join(words, "\n"))
	b.WriteString("\n\n出力形式:\n")
	b.WriteString(`[{"word":"入力語","name":"食材名または空文字"}]`)
	return b.String()
}

// parseResolverAnswers は応答からJSON配列を取り出す。
//
// 前後に説明文が付く場合に備え、最初の '[' から最後の ']' までを切り出す。
// 壊れていればエラーにする。呼び出し側は縮退して手動チェックに促すため、
// 誤った解決を通すより落とす方がよい。
func parseResolverAnswers(s string) ([]resolverAnswer, error) {
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start < 0 || end < start {
		return nil, errors.New("食材解決の応答をJSONとして解釈できませんでした")
	}

	var answers []resolverAnswer
	if err := json.Unmarshal([]byte(s[start:end+1]), &answers); err != nil {
		return nil, fmt.Errorf("食材解決の応答をJSONとして解釈できませんでした: %w", err)
	}
	return answers, nil
}
