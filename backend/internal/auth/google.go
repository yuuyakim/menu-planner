package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// ErrGoogleAuthFailed は Google 認証に失敗したことを表す。
// state 不一致・コード交換の失敗・メール未確認など、内訳を明かさず 401 に丸める。
var ErrGoogleAuthFailed = errors.New("Google認証に失敗しました")

// googleScopes は要求する権限。ログインに必要な最小限（本人確認とメール）。
var googleScopes = []string{"openid", "email", "profile"}

// defaultUserInfoURL は Google のユーザー情報エンドポイント。
const defaultUserInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"

// GoogleIdentity は Google から得た本人情報。
type GoogleIdentity struct {
	// Sub は Google アカウントの一意な識別子（auth_identities.provider_uid）。
	Sub string
	// Email はメールアドレス。
	Email string
	// EmailVerified は Google がメールを確認済みか。未確認のメールでの
	// アカウント紐付けは乗っ取りに使えるため、確認済みのみ受け入れる。
	EmailVerified bool
	// Name は表示名。
	Name string
}

// GoogleOAuth は Google の OAuth2 認可フロー（Authorization Code + PKCE）を扱う。
type GoogleOAuth struct {
	cfg         *oauth2.Config
	userInfoURL string
}

// GoogleOption は GoogleOAuth の任意設定。
type GoogleOption func(*GoogleOAuth)

// WithGoogleEndpoints は認可／トークン／ユーザー情報のURLを差し替える。
// テストで httptest サーバに向けるために使う。
func WithGoogleEndpoints(authURL, tokenURL, userInfoURL string) GoogleOption {
	return func(g *GoogleOAuth) {
		g.cfg.Endpoint = oauth2.Endpoint{AuthURL: authURL, TokenURL: tokenURL}
		g.userInfoURL = userInfoURL
	}
}

// NewGoogleOAuth は Google OAuth の設定を組み立てる。
// client_secret は認可URLの生成には不要だが、コールバックでのトークン交換
// で使うため、まとめてここで受け取る。
func NewGoogleOAuth(clientID, clientSecret, redirectURL string, opts ...GoogleOption) *GoogleOAuth {
	g := &GoogleOAuth{
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       googleScopes,
			Endpoint:     google.Endpoint,
		},
		userInfoURL: defaultUserInfoURL,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
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

// Exchange は認可コードを本人情報に交換する。
// PKCE の verifier を渡し、Google 側で code_challenge と突合させる。
// verifier が合わない・コードが無効などの失敗は ErrGoogleAuthFailed に丸める。
func (g *GoogleOAuth) Exchange(ctx context.Context, code, verifier string) (GoogleIdentity, error) {
	token, err := g.cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		// verifier 不一致・コード無効・期限切れなどはここで失敗する。
		return GoogleIdentity{}, fmt.Errorf("%w: コード交換に失敗しました: %w", ErrGoogleAuthFailed, err)
	}

	// アクセストークンでユーザー情報を取る。cfg.Client がトークンを付与する。
	client := g.cfg.Client(ctx, token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.userInfoURL, nil)
	if err != nil {
		return GoogleIdentity{}, fmt.Errorf("ユーザー情報の要求を作れませんでした: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return GoogleIdentity{}, fmt.Errorf("%w: ユーザー情報の取得に失敗しました: %w", ErrGoogleAuthFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return GoogleIdentity{}, fmt.Errorf("%w: ユーザー情報が %d でした: %s", ErrGoogleAuthFailed, resp.StatusCode, body)
	}

	var info struct {
		ID            string `json:"id"`
		Email         string `json:"email"`
		VerifiedEmail bool   `json:"verified_email"`
		Name          string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return GoogleIdentity{}, fmt.Errorf("%w: ユーザー情報を解釈できませんでした: %w", ErrGoogleAuthFailed, err)
	}

	return GoogleIdentity{
		Sub:           info.ID,
		Email:         info.Email,
		EmailVerified: info.VerifiedEmail,
		Name:          info.Name,
	}, nil
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
