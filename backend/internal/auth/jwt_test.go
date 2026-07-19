package auth_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
)

const testSecret = "test-secret-please-ignore-1234567890"

func newJWT(t *testing.T, opts ...auth.JWTOption) *auth.JWT {
	t.Helper()
	j, err := auth.NewJWT([]byte(testSecret), opts...)
	require.NoError(t, err)
	return j
}

func TestJWT_IssueAndVerify(t *testing.T) {
	t.Parallel()

	j := newJWT(t)
	token, err := j.Issue("user-123")
	require.NoError(t, err)
	require.NotEmpty(t, token)

	claims, err := j.Verify(token)
	require.NoError(t, err)
	require.Equal(t, "user-123", claims.UserID)
}

func TestJWT_RejectsExpired(t *testing.T) {
	t.Parallel()

	// 発行時刻を過去に固定し、15分の寿命を過ぎさせる。
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	issuer := newJWT(t, auth.WithNow(func() time.Time { return base }))
	token, err := issuer.Issue("user-123")
	require.NoError(t, err)

	// exp は排他的（RFC 7519: 現在時刻が exp より前であること）。
	// 14分59秒では有効、15分ちょうどで失効（境界値）。
	justBefore := newJWT(t, auth.WithNow(func() time.Time { return base.Add(15*time.Minute - time.Second) }))
	_, err = justBefore.Verify(token)
	require.NoError(t, err, "15分未満は有効であるべき")

	atLimit := newJWT(t, auth.WithNow(func() time.Time { return base.Add(15 * time.Minute) }))
	_, err = atLimit.Verify(token)
	require.ErrorIs(t, err, auth.ErrTokenInvalid, "15分に達したら失効すべき")
}

func TestJWT_RejectsDifferentSecret(t *testing.T) {
	t.Parallel()

	issuer := newJWT(t)
	token, err := issuer.Issue("user-123")
	require.NoError(t, err)

	// 別の鍵で検証すると署名が合わない。
	other, err := auth.NewJWT([]byte("a-completely-different-secret-value"))
	require.NoError(t, err)
	_, err = other.Verify(token)
	require.ErrorIs(t, err, auth.ErrTokenInvalid)
}

func TestJWT_RejectsAlgNone(t *testing.T) {
	t.Parallel()

	// alg=none の署名なしトークンを自作する。受理すると誰でも偽造できる。
	claims := jwtlib.MapClaims{
		"sub": "attacker",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	tok := jwtlib.NewWithClaims(jwtlib.SigningMethodNone, claims)
	unsigned, err := tok.SignedString(jwtlib.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	j := newJWT(t)
	_, err = j.Verify(unsigned)
	require.ErrorIs(t, err, auth.ErrTokenInvalid, "alg=none は拒否すべき")
}

func TestJWT_RejectsTamperedPayload(t *testing.T) {
	t.Parallel()

	j := newJWT(t)
	token, err := j.Issue("user-123")
	require.NoError(t, err)

	// ペイロード(2番目のセグメント)の sub を書き換え、署名はそのままにする。
	parts := strings.Split(token, ".")
	require.Len(t, parts, 3)

	var payload map[string]any
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &payload))
	payload["sub"] = "someone-else"
	edited, err := json.Marshal(payload)
	require.NoError(t, err)
	parts[1] = base64.RawURLEncoding.EncodeToString(edited)
	tampered := strings.Join(parts, ".")

	_, err = j.Verify(tampered)
	require.ErrorIs(t, err, auth.ErrTokenInvalid, "改竄されたペイロードは拒否すべき")
}

func TestNewJWT_RejectsEmptySecret(t *testing.T) {
	t.Parallel()

	// 空の鍵での発行は事故のもと。生成時に弾く。
	_, err := auth.NewJWT([]byte(""))
	require.Error(t, err)
}
