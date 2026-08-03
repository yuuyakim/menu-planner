package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// deepSeekEndpoint は OpenAI 互換のチャット補完エンドポイント。
const deepSeekEndpoint = "https://api.deepseek.com/chat/completions"

// deepSeekModel は使用するモデル。最も安い層を選ぶ（設計 3.7）。
const deepSeekModel = "deepseek-chat"

// DeepSeekResolver は DeepSeek で未解決語を食材名に対応づける。
//
// **公式SDKを足さず net/http で実装する。** eval で比較して採用しなかった
// 場合に、依存ごと削除できるようにするため。
type DeepSeekResolver struct {
	apiKey string
	client *http.Client
}

// NewDeepSeekResolver は DeepSeekResolver を生成する。
func NewDeepSeekResolver(apiKey string) (*DeepSeekResolver, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, ErrMissingResolverAPIKey
	}
	return &DeepSeekResolver{
		apiKey: apiKey,
		client: &http.Client{Timeout: claudeResolverTimeout},
	}, nil
}

// deepSeekRequest はチャット補完のリクエスト。
type deepSeekRequest struct {
	Model     string            `json:"model"`
	Messages  []deepSeekMessage `json:"messages"`
	MaxTokens int               `json:"max_tokens"`
}

type deepSeekMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// deepSeekResponse は必要な部分だけを取り出す。
type deepSeekResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Resolve は未解決語を食材名に対応づける。
// プロンプトと応答の解釈は Claude 実装と共用する（比較を公平にするため）。
func (r *DeepSeekResolver) Resolve(
	ctx context.Context, words []string, catalog []string,
) ([]service.GatewayResolution, error) {
	if len(words) == 0 {
		return []service.GatewayResolution{}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, claudeResolverTimeout)
	defer cancel()

	body, err := json.Marshal(deepSeekRequest{
		Model:     deepSeekModel,
		MaxTokens: claudeMaxTokens,
		Messages: []deepSeekMessage{
			{Role: "user", Content: buildResolverPrompt(words, catalog)},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("食材解決のリクエストを組み立てられませんでした: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deepSeekEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("食材解決のリクエストを組み立てられませんでした: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("食材解決の問い合わせに失敗しました: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("食材解決の問い合わせが失敗しました: status=%d", resp.StatusCode)
	}

	var parsed deepSeekResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("食材解決の応答を解釈できませんでした: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, errors.New("食材解決の応答が空でした")
	}

	answers, err := parseResolverAnswers(parsed.Choices[0].Message.Content)
	if err != nil {
		return nil, err
	}

	out := make([]service.GatewayResolution, 0, len(answers))
	for _, a := range answers {
		out = append(out, service.GatewayResolution{Word: a.Word, Name: a.Name})
	}
	return out, nil
}
