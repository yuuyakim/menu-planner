package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
)

// googleStub は Google のトークン／ユーザー情報エンドポイントを模す。
func googleStub(t *testing.T, tokenStatus int, userInfoJSON string, userInfoStatus int) *auth.GoogleOAuth {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		if tokenStatus != http.StatusOK {
			w.WriteHeader(tokenStatus)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"stub-access","token_type":"Bearer","expires_in":3600}`))
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(userInfoStatus)
		_, _ = w.Write([]byte(userInfoJSON))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return auth.NewGoogleOAuth("cid", "secret", "http://localhost/cb",
		auth.WithGoogleEndpoints(srv.URL+"/auth", srv.URL+"/token", srv.URL+"/userinfo"))
}

func TestGoogleExchange_Success(t *testing.T) {
	t.Parallel()

	g := googleStub(t, http.StatusOK,
		`{"id":"google-sub-1","email":"taro@gmail.com","verified_email":true,"name":"太郎"}`,
		http.StatusOK)

	id, err := g.Exchange(context.Background(), "auth-code", auth.GenerateVerifier())
	require.NoError(t, err)
	require.Equal(t, "google-sub-1", id.Sub)
	require.Equal(t, "taro@gmail.com", id.Email)
	require.True(t, id.EmailVerified)
	require.Equal(t, "太郎", id.Name)
}

func TestGoogleExchange_TokenEndpointFails(t *testing.T) {
	t.Parallel()

	// verifier 不一致やコード無効はトークンエンドポイントが 4xx を返す。
	g := googleStub(t, http.StatusBadRequest, "", http.StatusOK)

	_, err := g.Exchange(context.Background(), "bad-code", auth.GenerateVerifier())
	require.ErrorIs(t, err, auth.ErrGoogleAuthFailed)
}

func TestGoogleExchange_UserInfoFails(t *testing.T) {
	t.Parallel()

	g := googleStub(t, http.StatusOK, "", http.StatusInternalServerError)

	_, err := g.Exchange(context.Background(), "auth-code", auth.GenerateVerifier())
	require.ErrorIs(t, err, auth.ErrGoogleAuthFailed)
}
