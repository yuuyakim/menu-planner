package handler

import (
	"errors"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// ErrRateLimited はレート制限を超えたリクエストを表す。
// problem.go の対応表で 429 に変換される。
var ErrRateLimited = errors.New("レート制限を超えました")

// RateLimiter は IP ごとに window 内のリクエスト数を limit 回までに制限する。
// 超過したリクエストは ErrRateLimited（429）で弾く。
//
// 固定ウィンドウ方式にする。「10req/min」のような要件をそのまま表せて、
// トークンバケットのバースト設計を持ち込まずに済むため。
// 認証（10req/min）と検索（60req/min）で別々の上限を持たせられるよう、
// 環境変数を読まず limit / window を値として受け取る。
//
// limit が 0 以下なら無制限（素通し）にする。Vite プロキシ配下のように
// 全リクエストが1つのIPに集約されてしまう開発・E2E環境で、
// この値を使って制限を切れるようにするため。
func RateLimiter(limit int, window time.Duration) echo.MiddlewareFunc {
	if limit <= 0 {
		return func(next echo.HandlerFunc) echo.HandlerFunc {
			return next
		}
	}
	rl := &rateLimiter{
		limit:   limit,
		window:  window,
		buckets: make(map[string]*rateBucket),
		now:     time.Now,
	}
	// 端数の window でも最低1秒は待たせる。0 だと Retry-After が無意味になる。
	retryAfter := int(math.Ceil(window.Seconds()))
	if retryAfter < 1 {
		retryAfter = 1
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// RealIP は X-Forwarded-For / X-Real-IP を見てから RemoteAddr に落ちる。
			// リバースプロキシ越しでも実クライアント単位で数えられる。
			if !rl.allow(c.RealIP()) {
				c.Response().Header().Set("Retry-After", strconv.Itoa(retryAfter))
				return ErrRateLimited
			}
			return next(c)
		}
	}
}

// rateBucket は1つのIPの現ウィンドウでのリクエスト数を持つ。
type rateBucket struct {
	count       int
	windowStart time.Time
}

// rateLimiter は固定ウィンドウのカウンタを IP ごとに保持する。
type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]*rateBucket
	now     func() time.Time
}

// allow は ip からのリクエストを1つ数え、上限内なら true を返す。
func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()
	b := rl.buckets[ip]
	// 初回、またはウィンドウを過ぎていれば新しいウィンドウを開始する。
	if b == nil || now.Sub(b.windowStart) >= rl.window {
		rl.buckets[ip] = &rateBucket{count: 1, windowStart: now}
		return true
	}
	if b.count >= rl.limit {
		return false
	}
	b.count++
	return true
}
