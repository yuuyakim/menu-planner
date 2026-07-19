package auth_test

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
)

func newGoogle() *auth.GoogleOAuth {
	return auth.NewGoogleOAuth(
		"test-client-id",
		"test-client-secret",
		"http://localhost:8080/api/v1/auth/google/callback",
	)
}

func TestGoogleOAuth_AuthCodeURL(t *testing.T) {
	t.Parallel()

	g := newGoogle()
	verifier := auth.GenerateVerifier()
	raw := g.AuthCodeURL("state-abc", verifier)

	u, err := url.Parse(raw)
	require.NoError(t, err)
	// Google の認可エンドポイントに向く。
	require.Equal(t, "accounts.google.com", u.Host)

	q := u.Query()
	require.Equal(t, "test-client-id", q.Get("client_id"))
	require.Equal(t, "http://localhost:8080/api/v1/auth/google/callback", q.Get("redirect_uri"))
	require.Equal(t, "code", q.Get("response_type"))
	require.Equal(t, "state-abc", q.Get("state"))
	require.Contains(t, q.Get("scope"), "email")

	// PKCE: code_challenge は verifier の SHA-256 を base64url した値、method は S256。
	require.Equal(t, "S256", q.Get("code_challenge_method"))
	sum := sha256.Sum256([]byte(verifier))
	wantChallenge := base64.RawURLEncoding.EncodeToString(sum[:])
	require.Equal(t, wantChallenge, q.Get("code_challenge"))
}

func TestGenerateVerifier_Unique(t *testing.T) {
	t.Parallel()

	// PKCE の verifier は毎回異なる（推測されると PKCE の意味が無い）。
	v1 := auth.GenerateVerifier()
	v2 := auth.GenerateVerifier()
	require.NotEqual(t, v1, v2)
	require.NotEmpty(t, v1)
}

func TestGenerateState_Unique(t *testing.T) {
	t.Parallel()

	// state は CSRF 対策。毎回異なり、十分な長さを持つ。
	s1, err := auth.GenerateState()
	require.NoError(t, err)
	s2, err := auth.GenerateState()
	require.NoError(t, err)
	require.NotEqual(t, s1, s2)
	require.GreaterOrEqual(t, len(s1), 16)
}
