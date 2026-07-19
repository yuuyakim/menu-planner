package auth

import (
	"errors"
	"fmt"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

// ErrTokenInvalid はトークンが無効（期限切れ・署名不正・改竄・alg不正・種別違い）
// であることを表す。失敗の内訳は攻撃者に手掛かりを与えるため、外向きには1つに丸める。
var ErrTokenInvalid = errors.New("トークンが無効です")

const (
	// defaultAccessTTL はアクセストークンの寿命（spec.md 5章・15分）。
	// 短くして漏洩時の被害を抑え、更新はリフレッシュトークンで行う。
	defaultAccessTTL = 15 * time.Minute

	// defaultRefreshTTL はリフレッシュトークンの寿命（spec.md 5章・30日）。
	// これでアクセストークンを再発行する。長寿命なので Cookie は
	// /auth 配下にだけ送るよう絞る（handler 側）。
	defaultRefreshTTL = 30 * 24 * time.Hour

	// signingAlg は署名アルゴリズム。対称鍵の HS256 に固定する。
	signingAlg = "HS256"

	// tokenIssuer は発行者。将来トークンの用途が増えたときに識別できるよう入れる。
	tokenIssuer = "menu-planner"

	// トークン種別。アクセスとリフレッシュを取り違えないよう typ クレームで区別する。
	// これが無いと、長寿命のリフレッシュトークンを短寿命のはずのアクセストークンとして
	// そのまま使えてしまい、15分という寿命の意味が無くなる。
	typeAccess  = "access"
	typeRefresh = "refresh"
)

// Claims はトークンから取り出す情報。
type Claims struct {
	// UserID はトークンの主体（JWT の sub）。
	UserID string
}

// tokenClaims は署名するペイロード。用途を typ で区別する。
type tokenClaims struct {
	Type string `json:"typ"`
	jwtlib.RegisteredClaims
}

// JWT はアクセス／リフレッシュトークンの発行と検証を行う。
type JWT struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time
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

// WithRefreshTTL はリフレッシュトークンの寿命を変える。
func WithRefreshTTL(d time.Duration) JWTOption {
	return func(j *JWT) { j.refreshTTL = d }
}

// NewJWT はトークンの発行・検証器を組み立てる。
// 空の鍵は事故のもとなので生成時に弾く。
func NewJWT(secret []byte, opts ...JWTOption) (*JWT, error) {
	if len(secret) == 0 {
		return nil, errors.New("JWTの秘密鍵が空です")
	}
	j := &JWT{
		secret:     secret,
		accessTTL:  defaultAccessTTL,
		refreshTTL: defaultRefreshTTL,
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(j)
	}
	return j, nil
}

// AccessTTL はアクセストークンの寿命を返す。Cookie の MaxAge を揃えるのに使う。
func (j *JWT) AccessTTL() time.Duration { return j.accessTTL }

// RefreshTTL はリフレッシュトークンの寿命を返す。
func (j *JWT) RefreshTTL() time.Duration { return j.refreshTTL }

// Issue は userID を主体とするアクセストークンを発行する。
func (j *JWT) Issue(userID string) (string, error) {
	return j.issue(userID, typeAccess, j.accessTTL)
}

// IssueRefresh は userID を主体とするリフレッシュトークンを発行する。
func (j *JWT) IssueRefresh(userID string) (string, error) {
	return j.issue(userID, typeRefresh, j.refreshTTL)
}

func (j *JWT) issue(userID, typ string, ttl time.Duration) (string, error) {
	now := j.now()
	claims := tokenClaims{
		Type: typ,
		RegisteredClaims: jwtlib.RegisteredClaims{
			Subject:   userID,
			Issuer:    tokenIssuer,
			IssuedAt:  jwtlib.NewNumericDate(now),
			ExpiresAt: jwtlib.NewNumericDate(now.Add(ttl)),
		},
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	signed, err := token.SignedString(j.secret)
	if err != nil {
		return "", fmt.Errorf("トークンの署名に失敗しました: %w", err)
	}
	return signed, nil
}

// Verify はアクセストークンを検証し、主体を取り出す。無効なら ErrTokenInvalid。
func (j *JWT) Verify(token string) (Claims, error) {
	return j.verify(token, typeAccess)
}

// VerifyRefresh はリフレッシュトークンを検証し、主体を取り出す。無効なら ErrTokenInvalid。
func (j *JWT) VerifyRefresh(token string) (Claims, error) {
	return j.verify(token, typeRefresh)
}

// verify はトークンを検証し、種別が expectedType であることまで確かめる。
//
// 署名方式は HS256 に限定する。これにより alg=none（署名なし）や、公開鍵を
// 秘密鍵と取り違えさせる非対称方式へのすり替えを拒否する。種別が食い違う
// トークン（リフレッシュをアクセスとして使う等）も拒否する。
func (j *JWT) verify(token, expectedType string) (Claims, error) {
	var claims tokenClaims
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
	if !parsed.Valid || claims.Type != expectedType {
		return Claims{}, ErrTokenInvalid
	}

	return Claims{UserID: claims.Subject}, nil
}
