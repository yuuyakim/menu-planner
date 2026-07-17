package gateway_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/gateway"
)

// testBackoff はテスト用の短いバックオフ。実際の待ちは 200ms 起点だが、
// テストで待つ意味は無いので縮める。
// 計測のノイズ（接続確立やスケジューリング）に埋もれない程度の長さは要る。
const testBackoff = 40 * time.Millisecond

// recorder は模擬サーバが受けたリクエストの時刻を記録する。
type recorder struct {
	mu    sync.Mutex
	times []time.Time
}

func (r *recorder) record() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.times = append(r.times, time.Now())
	return len(r.times)
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.times)
}

// gaps は各リクエストの間隔を返す。
func (r *recorder) gaps() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()

	gaps := make([]time.Duration, 0, len(r.times))
	for i := 1; i < len(r.times); i++ {
		gaps = append(gaps, r.times[i].Sub(r.times[i-1]))
	}
	return gaps
}

// flakyServer は handler に試行回数を渡して応答を切り替えるサーバを起動する。
func flakyServer(t *testing.T, rec *recorder, handler func(attempt int, w http.ResponseWriter)) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handler(rec.record(), w)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func writeOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"web": map[string]any{"results": sampleResults(3)},
	})
}

// newResilientBrave はリトライ間隔だけ縮めた Brave を返す。
func newResilientBrave(t *testing.T, srv *httptest.Server) *gateway.Brave {
	t.Helper()

	g, err := gateway.NewBrave(testAPIKey,
		gateway.WithEndpoint(srv.URL),
		gateway.WithBackoff(testBackoff),
	)
	require.NoError(t, err)
	return g
}

func TestBrave_HTTP500でエラー(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	srv := flakyServer(t, rec, func(_ int, w http.ResponseWriter) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := newResilientBrave(t, srv).Search(context.Background(), "親子丼", 3)

	require.Error(t, err)
	assert.ErrorIs(t, err, gateway.ErrSearchFailed, "呼び出し側が502に変換できること")
}

func TestBrave_不正なJSONでエラー(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	srv := flakyServer(t, rec, func(_ int, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"web": {"results": [`))
	})

	_, err := newResilientBrave(t, srv).Search(context.Background(), "親子丼", 3)

	require.Error(t, err)
	assert.ErrorIs(t, err, gateway.ErrSearchFailed)
}

func TestBrave_5xxはリトライして最終的に成功する(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	srv := flakyServer(t, rec, func(attempt int, w http.ResponseWriter) {
		if attempt < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeOK(w)
	})

	got, err := newResilientBrave(t, srv).Search(context.Background(), "親子丼", 3)

	require.NoError(t, err)
	assert.Len(t, got, 3)
	assert.Equal(t, 3, rec.count(), "初回 + リトライ2回")
}

func TestBrave_リトライは最大2回(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	srv := flakyServer(t, rec, func(_ int, w http.ResponseWriter) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := newResilientBrave(t, srv).Search(context.Background(), "親子丼", 3)

	require.Error(t, err)
	assert.Equal(t, 3, rec.count(), "初回 + リトライ2回 で打ち切ること")
}

func TestBrave_リトライ後も失敗ならエラー(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	srv := flakyServer(t, rec, func(_ int, w http.ResponseWriter) {
		w.WriteHeader(http.StatusBadGateway)
	})

	_, err := newResilientBrave(t, srv).Search(context.Background(), "親子丼", 3)

	require.Error(t, err)
	assert.ErrorIs(t, err, gateway.ErrSearchFailed)
	assert.Contains(t, err.Error(), "502", "最後のステータスが分かること")
}

func TestBrave_指数バックオフで待ち時間が伸びる(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	srv := flakyServer(t, rec, func(_ int, w http.ResponseWriter) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := newResilientBrave(t, srv).Search(context.Background(), "親子丼", 3)
	require.Error(t, err)

	gaps := rec.gaps()
	require.Len(t, gaps, 2)

	// 下限だけを見る。time.After は指定時間**以上**待つことを保証するので、
	// この2つは実装が正しい限り必ず通り、待ちが伸びない実装（40ms, 40ms）なら
	// 2つ目が落ちる。
	//
	// 「2回目の間隔 > 1回目の間隔」という比較はしない。初回リクエストには
	// 接続確立の分が上乗せされ、待ち時間より大きくなることがあるため
	// （実測で 1回目 52ms > 2回目 40ms になった）。
	assert.GreaterOrEqual(t, gaps[0], testBackoff, "1回目の待ちは backoff 以上")
	assert.GreaterOrEqual(t, gaps[1], 2*testBackoff, "2回目の待ちは backoff の倍以上")
}

func TestBrave_4xxはリトライしない(t *testing.T) {
	t.Parallel()

	tests := map[string]int{
		"400 不正なリクエスト": http.StatusBadRequest,
		"401 キーが不正":    http.StatusUnauthorized,
		"403 権限なし":     http.StatusForbidden,
		"404 経路が違う":    http.StatusNotFound,
		"422 パラメータが不正": http.StatusUnprocessableEntity,
	}
	for name, status := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// 4xx は再試行しても同じ結果になる。無駄にAPIを消費しない。
			rec := &recorder{}
			srv := flakyServer(t, rec, func(_ int, w http.ResponseWriter) {
				w.WriteHeader(status)
			})

			_, err := newResilientBrave(t, srv).Search(context.Background(), "親子丼", 3)

			require.Error(t, err)
			assert.ErrorIs(t, err, gateway.ErrSearchFailed)
			assert.Equal(t, 1, rec.count(), "1回で打ち切ること")
		})
	}
}

func TestBrave_429はリトライする(t *testing.T) {
	t.Parallel()

	// 429 は 4xx だがレート制限であり、時間をおけば成功しうる。
	// 他の 4xx と違って再試行に意味がある。
	rec := &recorder{}
	srv := flakyServer(t, rec, func(attempt int, w http.ResponseWriter) {
		if attempt == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeOK(w)
	})

	got, err := newResilientBrave(t, srv).Search(context.Background(), "親子丼", 3)

	require.NoError(t, err)
	assert.Len(t, got, 3)
	assert.Equal(t, 2, rec.count())
}

func TestBrave_タイムアウトでエラー(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	srv := flakyServer(t, rec, func(_ int, w http.ResponseWriter) {
		time.Sleep(200 * time.Millisecond)
		writeOK(w)
	})

	g, err := gateway.NewBrave(testAPIKey,
		gateway.WithEndpoint(srv.URL),
		gateway.WithBackoff(testBackoff),
		gateway.WithTimeout(30*time.Millisecond),
	)
	require.NoError(t, err)

	_, err = g.Search(context.Background(), "親子丼", 3)

	require.Error(t, err)
	assert.ErrorIs(t, err, gateway.ErrSearchFailed)
}

func TestBrave_タイムアウトもリトライの対象(t *testing.T) {
	t.Parallel()

	// 一時的な遅延なら次の試行で成功しうる。
	rec := &recorder{}
	srv := flakyServer(t, rec, func(attempt int, w http.ResponseWriter) {
		if attempt == 1 {
			time.Sleep(200 * time.Millisecond)
		}
		writeOK(w)
	})

	g, err := gateway.NewBrave(testAPIKey,
		gateway.WithEndpoint(srv.URL),
		gateway.WithBackoff(testBackoff),
		gateway.WithTimeout(50*time.Millisecond),
	)
	require.NoError(t, err)

	got, err := g.Search(context.Background(), "親子丼", 3)

	require.NoError(t, err)
	assert.Len(t, got, 3)
	assert.Equal(t, 2, rec.count())
}

func TestBrave_既定のタイムアウトは3秒(t *testing.T) {
	t.Parallel()

	g, err := gateway.NewBrave(testAPIKey)

	require.NoError(t, err)
	assert.Equal(t, 3*time.Second, g.Timeout())
}

func TestBrave_呼び出し側のcontextが切れたら即座に諦める(t *testing.T) {
	t.Parallel()

	// 利用者が画面を離れた場合など。リトライを続けても無駄にAPIを消費するだけ。
	rec := &recorder{}
	srv := flakyServer(t, rec, func(_ int, w http.ResponseWriter) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := newResilientBrave(t, srv).Search(ctx, "親子丼", 3)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, rec.count(), "1回も呼ばないこと")
}

func TestBrave_リトライ中にcontextが切れたら打ち切る(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	srv := flakyServer(t, rec, func(_ int, w http.ResponseWriter) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	// 1回目の応答後、バックオフの最中に切れるようにする。
	ctx, cancel := context.WithTimeout(context.Background(), testBackoff/2)
	defer cancel()

	_, err := newResilientBrave(t, srv).Search(ctx, "親子丼", 3)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, 1, rec.count(), "バックオフ中に諦め、2回目を投げないこと")
}
