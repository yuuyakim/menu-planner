package auth_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
)

func TestJWT_IssueAndVerifyRefresh(t *testing.T) {
	t.Parallel()

	j := newJWT(t)
	token, err := j.IssueRefresh("user-123")
	require.NoError(t, err)

	claims, err := j.VerifyRefresh(token)
	require.NoError(t, err)
	require.Equal(t, "user-123", claims.UserID)
}

func TestJWT_RefreshExpiresIn30Days(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	issuer := newJWT(t, auth.WithNow(func() time.Time { return base }))
	token, err := issuer.IssueRefresh("user-123")
	require.NoError(t, err)

	// 30日未満は有効、30日ちょうどで失効（exp は排他的）。
	justBefore := newJWT(t, auth.WithNow(func() time.Time { return base.Add(30*24*time.Hour - time.Second) }))
	_, err = justBefore.VerifyRefresh(token)
	require.NoError(t, err, "30日未満は有効であるべき")

	atLimit := newJWT(t, auth.WithNow(func() time.Time { return base.Add(30 * 24 * time.Hour) }))
	_, err = atLimit.VerifyRefresh(token)
	require.ErrorIs(t, err, auth.ErrTokenInvalid, "30日に達したら失効すべき")
}

func TestJWT_RefreshRejectedAsAccess(t *testing.T) {
	t.Parallel()

	// リフレッシュトークンをアクセストークンとして使えてはならない。
	// これができると 15分の寿命を回避して 30日使い回せてしまう。
	j := newJWT(t)
	refresh, err := j.IssueRefresh("user-123")
	require.NoError(t, err)

	_, err = j.Verify(refresh)
	require.ErrorIs(t, err, auth.ErrTokenInvalid, "リフレッシュをアクセスとして使うのは拒否すべき")
}

func TestJWT_AccessRejectedAsRefresh(t *testing.T) {
	t.Parallel()

	// 逆向きも拒否。アクセストークンでリフレッシュはできない。
	j := newJWT(t)
	access, err := j.Issue("user-123")
	require.NoError(t, err)

	_, err = j.VerifyRefresh(access)
	require.ErrorIs(t, err, auth.ErrTokenInvalid)
}
