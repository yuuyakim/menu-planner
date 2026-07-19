package auth

import (
	"errors"
	"fmt"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

// ErrTokenInvalid はトークンが無効（期限切れ・署名不正・改竄・alg不正）であることを表す。
// 失敗の内訳は攻撃者に手掛かりを与えるため、外向きには1つに丸める。
var ErrTokenInvalid = errors.New("トークンが無効です")

const (
	// defaultAccessTTL はアクセストークンの寿命（spec.md 5章・15分）。
	// 短くして漏洩時の被害を抑え、更新はリフレッシュトークンで行う（5-F）。
	defaultAccessTTL = 15 * time.Minute

	// signingAlg は署名アルゴリズム。対称鍵の HS256 に固定する。
	signingAlg = "HS256"

	// tokenIssuer は発行者。将来トークンの用途が増えたときに識別できるよう入れる。
	tokenIssuer = "menu-planner"
)

// Claims はアクセストークンから取り出す情報。
type Claims struct {
	// UserID はトークンの主体（JWT の sub）。
	UserID string
}

// JWT はアクセストークンの発行と検証を行う。
type JWT struct {
	secret    []byte
	accessTTL time.Duration
	now       func() time.Time
}

// JWTOption は JWT の任意設定。
type JWTOption func(*JWT)

// WithNow は現在時刻の取得方法を変える。テストで期限を固定するために使う。
func WithNow(now func() time.Time) JWTOption {
	return func(j *JWT) { j.now = now }
}

// WithAccessTTL はアクセストークンの寿命を変える。
func WithAccessTTL(d time.Duration) JWTOption {
	return func(j *JWT) { j.accessTTL = d }
}

// NewJWT はトークンの発行・検証器を組み立てる。
// 空の鍵は事故のもとなので生成時に弾く。
func NewJWT(secret []byte, opts ...JWTOption) (*JWT, error) {
	if len(secret) == 0 {
		return nil, errors.New("JWTの秘密鍵が空です")
	}
	j := &JWT{
		secret:    secret,
		accessTTL: defaultAccessTTL,
		now:       time.Now,
	}
	for _, opt := range opts {
		opt(j)
	}
	return j, nil
}

// Issue は userID を主体とするアクセストークンを発行する。
func (j *JWT) Issue(userID string) (string, error) {
	now := j.now()
	claims := jwtlib.RegisteredClaims{
		Subject:   userID,
		Issuer:    tokenIssuer,
		IssuedAt:  jwtlib.NewNumericDate(now),
		ExpiresAt: jwtlib.NewNumericDate(now.Add(j.accessTTL)),
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	signed, err := token.SignedString(j.secret)
	if err != nil {
		return "", fmt.Errorf("トークンの署名に失敗しました: %w", err)
	}
	return signed, nil
}

// Verify はトークンを検証し、主体を取り出す。無効なら ErrTokenInvalid を返す。
//
// 署名方式は HS256 に限定する。これにより alg=none（署名なし）や、公開鍵を
// 秘密鍵と取り違えさせる非対称方式へのすり替えを拒否する。
func (j *JWT) Verify(token string) (Claims, error) {
	var claims jwtlib.RegisteredClaims
	parsed, err := jwtlib.ParseWithClaims(token, &claims,
		func(*jwtlib.Token) (any, error) { return j.secret, nil },
		jwtlib.WithValidMethods([]string{signingAlg}),
		jwtlib.WithIssuer(tokenIssuer),
		// 期限判定に使う「今」を差し替え可能にする。テストと本番で同じ経路を通す。
		jwtlib.WithTimeFunc(j.now),
	)
	if err != nil {
		// 失敗の内訳（期限切れ・署名不正など）はログや調査のために包むが、
		// 呼び出し側は ErrTokenInvalid だけを見て一律に扱う。
		return Claims{}, fmt.Errorf("%w: %w", ErrTokenInvalid, err)
	}
	if !parsed.Valid {
		return Claims{}, ErrTokenInvalid
	}

	return Claims{UserID: claims.Subject}, nil
}
