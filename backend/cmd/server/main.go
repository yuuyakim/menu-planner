// Package main は献立提案APIサーバのエントリポイント。
// 依存の組み立て（DI）とサーバのライフサイクル管理のみを行う。
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/db"
	"github.com/yuuyakim/menu-planner/backend/internal/gateway"
	"github.com/yuuyakim/menu-planner/backend/internal/handler"
	"github.com/yuuyakim/menu-planner/backend/internal/random"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// dbConnectTimeout は起動時のDB接続に費やしてよい上限。
// db.NewPool はコールドスタート（Neonのスリープ復帰など）を見込んで指数バックオフで
// リトライするため、その分の猶予を持たせる。ホスト名が解決できない場合などは
// リトライが尽きる前にこの上限が先に効いて起動を打ち切る。
const dbConnectTimeout = 30 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("サーバの起動に失敗しました", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// DBに繋がらないまま起動すると、献立APIが全て500を返すサーバが
	// ヘルスチェックだけ通る状態になる。起動時に失敗させて気付けるようにする。
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL が設定されていません")
	}

	dbCtx, dbCancel := context.WithTimeout(context.Background(), dbConnectTimeout)
	defer dbCancel()

	pool, err := db.NewPool(dbCtx, db.Config{DSN: dsn})
	if err != nil {
		return fmt.Errorf("DBへの接続に失敗しました: %w", err)
	}
	defer pool.Close()
	slog.Info("DBに接続しました")

	// 検索APIの設定不備は起動時に落とす。プロバイダ未設定のまま起動できると、
	// 利用者にダミーのレシピを配り続けることになり誰も気付けない。
	recipeGateway, err := gateway.New(gateway.Config{
		Provider: os.Getenv("SEARCH_API_PROVIDER"),
		APIKey:   os.Getenv("SEARCH_API_KEY"),
	})
	if err != nil {
		return fmt.Errorf("レシピ検索の設定に失敗しました: %w", err)
	}
	slog.Info("レシピ検索を設定しました", "provider", os.Getenv("SEARCH_API_PROVIDER"))

	// JWT の秘密鍵が無いと誰でも通るサーバになりかねない。起動時に落とす。
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return errors.New("JWT_SECRET が設定されていません")
	}
	tokens, err := auth.NewJWT([]byte(jwtSecret))
	if err != nil {
		return fmt.Errorf("JWTの初期化に失敗しました: %w", err)
	}

	// Stripe の設定が無いと課金APIが機能しないサーバになる。起動時に落とす。
	stripeSecret := os.Getenv("STRIPE_SECRET_KEY")
	stripeWebhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	stripePriceID := os.Getenv("STRIPE_PRICE_ID")
	if stripeSecret == "" || stripeWebhookSecret == "" || stripePriceID == "" {
		return errors.New("STRIPE_SECRET_KEY / STRIPE_WEBHOOK_SECRET / STRIPE_PRICE_ID が設定されていません")
	}

	// Google SSO は任意。未設定でも起動し、/auth/google だけ 503 になる。
	googleOAuth := auth.NewGoogleOAuth(
		os.Getenv("GOOGLE_CLIENT_ID"),
		os.Getenv("GOOGLE_CLIENT_SECRET"),
		os.Getenv("GOOGLE_REDIRECT_URL"),
	)

	// FRONTEND_ORIGIN は CORS の許可オリジンであると同時に、Google ログイン完了後に
	// 利用者を戻す先でもある。本番で未設定のまま起動すると「認証は成功するのに
	// localhost へ飛ばされる」という気付きにくい壊れ方をするため、
	// 既定値で起動したことを警告に残す。
	frontendOrigin := os.Getenv("FRONTEND_ORIGIN")
	if frontendOrigin == "" {
		frontendOrigin = "http://localhost:5173"
		slog.Warn("FRONTEND_ORIGIN が未設定のため既定値を使います。"+
			"本番ではGoogleログイン後の戻り先とCORSの許可オリジンが不正になります",
			"fallback", frontendOrigin)
	}

	userRepo := repository.NewUserRepository(pool)
	authSvc := service.NewAuthService(userRepo, auth.Hasher{})

	// entitlementSvc は authHandler（/auth/me がプランを返す）と
	// savedWeeklySvc（保存上限のチェック）の両方が使うため、両者より前に定義する。
	subscriptionRepo := repository.NewSubscriptionRepository(pool)
	entitlementSvc := service.NewEntitlementService(subscriptionRepo, time.Now)

	authHandler := handler.NewAuthHandler(authSvc, tokens, googleOAuth, frontendOrigin, entitlementSvc)

	historyRepo := repository.NewHistoryRepository(pool)
	historySvc := service.NewHistoryService(historyRepo)
	historyHandler := handler.NewHistoryHandler(historySvc, tokens)

	favoriteRepo := repository.NewFavoriteRepository(pool)
	favoriteSvc := service.NewFavoriteService(favoriteRepo)
	favoriteHandler := handler.NewFavoriteHandler(favoriteSvc, tokens)

	savedWeeklyRepo := repository.NewSavedWeeklyMenuRepository(pool)
	savedWeeklySvc := service.NewSavedWeeklyMenuService(savedWeeklyRepo, entitlementSvc)
	// 保存/一覧/削除は premium 限定のため entitlementSvc も渡す。
	savedWeeklyHandler := handler.NewSavedWeeklyMenuHandler(savedWeeklySvc, tokens, entitlementSvc)

	ingredientRepo := repository.NewIngredientRepository(pool)

	menuRepo := repository.NewMenuRepository(pool)
	recipeCache := repository.NewRecipeLinkCache(pool)
	menuSvc := service.NewMenuService(menuRepo, random.NewCrypto(), recipeGateway, recipeCache)
	// 献立検索は履歴（RecentMenuIDs / Record）とトークン（OptionalAuth）を使う。
	// 週間献立（suggest-weekly / reroll-day）は premium 限定のため entitlementSvc も渡す。
	menuHandler := handler.NewMenuHandler(menuSvc, historySvc, tokens, entitlementSvc)

	// 買い物リストと食材は同じ service が担う（どちらも献立×食材を扱うため）。
	shoppingSvc := service.NewShoppingListService(menuRepo, ingredientRepo)
	shoppingHandler := handler.NewShoppingListHandler(shoppingSvc)
	ingredientSvc := service.NewIngredientService(ingredientRepo, menuRepo)
	ingredientHandler := handler.NewIngredientHandler(shoppingSvc, ingredientSvc)

	// 手持ちの食材のテキスト入力。①完全一致 →②解決キャッシュ →③LLM の順に解く。
	resolutionRepo := repository.NewResolutionRepository(pool)
	resolveGateway, err := gateway.NewResolver(gateway.ResolverConfig{
		Provider: os.Getenv("INGREDIENT_RESOLVER_PROVIDER"),
		APIKey:   os.Getenv("INGREDIENT_RESOLVER_API_KEY"),
	})
	if err != nil {
		slog.Error("食材解決の設定に失敗しました", "error", err)
		os.Exit(1)
	}
	slog.Info("食材解決を設定しました", "provider", os.Getenv("INGREDIENT_RESOLVER_PROVIDER"))

	// 読み取り（LLM）の日次上限（設計 3章）。分単位のバースト制御は searchLimit が
	// 担い、その上に日次の層を重ねる。0 で無制限にできるのは、Vite プロキシ配下の
	// 開発・E2E で全リクエストが単一IPに集約されるため。
	resolveUsageRepo := repository.NewResolveUsageRepository(pool)
	resolveQuota := service.NewResolveQuota(resolveUsageRepo, service.ResolveQuotaLimits{
		Anon:  envInt("RESOLVE_DAILY_LIMIT_ANON", 10),
		User:  envInt("RESOLVE_DAILY_LIMIT_USER", 30),
		Total: envInt("RESOLVE_DAILY_LIMIT_TOTAL", 300),
	}, time.Now)

	// 未設定で起動させない。既定値で動かすと生IPと同じ強度の値が全環境で共有され、
	// ハッシュ化の意味が無くなる（設計 6.2）。
	resolveIPHashSecret := os.Getenv("RESOLVE_IP_HASH_SECRET")
	if resolveIPHashSecret == "" {
		slog.Error("RESOLVE_IP_HASH_SECRET が未設定です")
		os.Exit(1)
	}

	resolveSvc := service.NewIngredientResolveService(
		ingredientRepo, resolutionRepo, resolveGateway, resolveQuota)
	resolveHandler := handler.NewIngredientResolveHandler(
		resolveSvc, resolveQuota, resolveIPHashSecret, tokens)

	// 保存済み週の買い物リストは、既存の導出（shoppingSvc）に差分（overrideRepo）を
	// 重ねる。所有者検証は savedWeeklyRepo、premium 判定は entitlementSvc に委ねる。
	overrideRepo := repository.NewShoppingListOverrideRepository(pool)
	savedShoppingListSvc := service.NewSavedShoppingListService(
		shoppingSvc, savedWeeklyRepo, overrideRepo, entitlementSvc)
	// GET/PUT の買い物リストも premium 限定のため entitlementSvc も渡す。
	savedShoppingListHandler := handler.NewSavedShoppingListHandler(savedShoppingListSvc, tokens, entitlementSvc)

	// トライアル期間はspec.md/課金の仕様で決まる固定値（初回加入者のみ適用）。
	const trialDays = 5
	paymentGateway := repository.NewStripePaymentGateway(
		stripeSecret, stripeWebhookSecret, stripePriceID, int64(trialDays))
	billingSvc := service.NewBillingService(
		entitlementSvc, subscriptionRepo, paymentGateway,
		frontendOrigin+"/checkout/complete?session_id={CHECKOUT_SESSION_ID}",
		frontendOrigin+"/checkout",
		frontendOrigin+"/account",
		trialDays, time.Now)
	billingHandler := handler.NewBillingHandler(billingSvc, tokens)

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// 本番はプロキシ（Cloudflare Pages の Function）の背後に立つ。共有シークレットが
	// 一致したリクエストの X-Forwarded-For だけを信頼して実クライアントIPを取り出す。
	// 未設定なら接続元をそのまま使う（＝転送ヘッダを信用しない安全側）。
	e.IPExtractor = handler.TrustedProxyIPExtractor(os.Getenv("TRUSTED_PROXY_SECRET"))

	// 全てのエラーレスポンスを RFC 7807 形式に統一する
	e.HTTPErrorHandler = handler.ErrorHandler()

	e.Use(middleware.Recover())
	// Webhook（未認証で叩ける公開エンドポイント）が io.ReadAll で本文を無制限に
	// 読み込むためのDoS対策。Stripeの実ペイロードは数十KB程度、他エンドポイントの
	// 本文もごく小さいため 1M で十分safe側。
	e.Use(middleware.BodyLimit("1M"))
	e.Use(middleware.RequestID())
	// RequestID の後に置く。request_id を全ログに伝播させ、1リクエスト1行の
	// アクセスログを出す。本文・機微なヘッダ・クエリは記録しない。
	e.Use(handler.RequestLogging(slog.Default()))
	e.Use(handler.CORS(frontendOrigin))

	// レート制限は IP 単位（spec.md 11章）。認証は総当たりを防ぐため 10req/min と
	// 厳しめ、検索は通常利用で詰まらないよう 60req/min。他系統は spec で未指定のため課さない。
	//
	// 上限は環境変数で上書きできる。Vite プロキシ配下（開発・E2E）では全リクエストが
	// プロキシの単一IPに集約され、正当な利用でも即座に上限へ達してしまう。そこでは
	// 0 を渡して制限を切る（RateLimiter が 0 以下を無制限として扱う）。
	authLimit := handler.RateLimiter(envInt("AUTH_RATE_LIMIT_PER_MIN", 10), time.Minute)
	searchLimit := handler.RateLimiter(envInt("SEARCH_RATE_LIMIT_PER_MIN", 60), time.Minute)

	health := handler.NewHealthHandler()
	e.GET("/health", health.Health)
	menuHandler.RegisterRoutes(e, searchLimit)
	// 食材・買い物リストは検索系と同じ扱い（未認証で使え、検索の上限を適用）。
	ingredientHandler.RegisterRoutes(e, searchLimit)
	resolveHandler.RegisterRoutes(e, searchLimit)
	shoppingHandler.RegisterRoutes(e, searchLimit)
	authHandler.RegisterRoutes(e, authLimit)
	historyHandler.RegisterRoutes(e)
	favoriteHandler.RegisterRoutes(e)
	savedWeeklyHandler.RegisterRoutes(e)
	savedShoppingListHandler.RegisterRoutes(e)
	billingHandler.RegisterRoutes(e)

	addr := ":" + env("PORT", "8080")

	go func() {
		slog.Info("サーバを起動しました", "addr", addr)
		if err := e.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("サーバが異常終了しました", "error", err)
			os.Exit(1)
		}
	}()

	// SIGTERM/SIGINT を受けたら処理中のリクエストを捌き切ってから終了する。
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	slog.Info("シャットダウンを開始します")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return e.Shutdown(ctx)
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envInt は環境変数を整数として読む。未設定・空・数値でない場合は fallback。
// レート制限の上限のように「設定されていれば上書き、無ければ既定」を表す。
func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		slog.Warn("環境変数を整数として解釈できませんでした。既定値を使います",
			"key", key, "value", v, "fallback", fallback)
		return fallback
	}
	return n
}
