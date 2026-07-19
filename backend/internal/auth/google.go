package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// googleScopes は要求する権限。ログインに必要な最小限（本人確認とメール）。
var googleScopes = []string{"openid", "email", "profile"}

// GoogleOAuth は Google の OAuth2 認可フロー（Authorization Code + PKCE）を扱う。
type GoogleOAuth struct {
	cfg *oauth2.Config
}

// NewGoogleOAuth は Google OAuth の設定を組み立てる。
// client_secret は認可URLの生成には不要だが、コールバックでのトークン交換
// （5-I）で使うため、まとめてここで受け取る。
func NewGoogleOAuth(clientID, clientSecret, redirectURL string) *GoogleOAuth {
	return &GoogleOAuth{
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       googleScopes,
			Endpoint:     google.Endpoint,
		},
	}
}

// Configured は Google ログインが利用可能に設定されているかを返す。
// client_id が空なら未設定として扱う。
func (g *GoogleOAuth) Configured() bool {
	return g.cfg.ClientID != ""
}

// AuthCodeURL は Google の認可画面に送るURLを組み立てる。
// PKCE の code_challenge（verifier の S256）と state を含める。
func (g *GoogleOAuth) AuthCodeURL(state, verifier string) string {
	return g.cfg.AuthCodeURL(state,
		oauth2.S256ChallengeOption(verifier),
		// リフレッシュトークンを得るため（将来サーバ側で使う場合に備える）。
		oauth2.AccessTypeOffline,
	)
}

// GenerateVerifier は PKCE の code_verifier を生成する。毎回異なる乱数。
func GenerateVerifier() string {
	return oauth2.GenerateVerifier()
}

// GenerateState は CSRF 対策の state を生成する。推測できないよう乱数から作る。
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("stateの生成に失敗しました: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
