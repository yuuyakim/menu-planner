import { Link, NavLink, Route, Routes, useLocation } from 'react-router'

import { ErrorBoundary } from '../components/ErrorBoundary'
import { Footer } from '../components/Footer'
import { NotFoundPage } from '../components/NotFoundPage'
import { LoginPage } from '../features/auth/LoginPage'
import { RequireAuth } from '../features/auth/RequireAuth'
import { AuthMenu } from '../features/auth/AuthMenu'
import { AccountPage } from '../features/billing/AccountPage'
import { CheckoutCompletePage } from '../features/billing/CheckoutCompletePage'
import { CheckoutPage } from '../features/billing/CheckoutPage'
import { FavoritePage } from '../features/favorite/FavoritePage'
import { HistoryPage } from '../features/history/HistoryPage'
import { HomePage } from '../features/home/HomePage'
import { PrivacyPage } from '../features/legal/PrivacyPage'
import { TermsPage } from '../features/legal/TermsPage'
import { TokushohoPage } from '../features/legal/TokushohoPage'
import { MenuDetailPage } from '../features/menu/MenuDetailPage'
import { SavedWeeklyPage } from '../features/menu/SavedWeeklyPage'
import { SearchByIngredientsPage } from '../features/menu/SearchByIngredientsPage'
import { SearchPage } from '../features/menu/SearchPage'
import { ShoppingListPage } from '../features/menu/ShoppingListPage'
import { WeeklyPage } from '../features/menu/WeeklyPage'
import { PricingPage } from '../features/pricing/PricingPage'

// navItems はヘッダに並べるリンク。増減はここだけで済ませる。
const navItems = [
  { to: '/search', label: '献立を探す' },
  { to: '/from-fridge', label: '冷蔵庫から探す' },
  { to: '/weekly', label: '1週間の献立' },
  { to: '/saved-weekly', label: '保存した週間献立' },
  { to: '/histories', label: '履歴' },
  { to: '/favorites', label: 'お気に入り' },
] as const

// linkClass は現在地のリンクだけ色を変える。
// NavLink が渡す isActive を使い、現在地の判定を自前で持たない。
function linkClass({ isActive }: { isActive: boolean }): string {
  // whitespace-nowrap が要る。狭い画面で flex がリンクを縮めると、
  // 「1週間の献立」のような和文がひらがな単位で折り返されて読めなくなる。
  const base = 'whitespace-nowrap rounded-full px-3 py-1 text-sm transition-colors'
  return isActive
    ? `${base} bg-kon-leaf/20 font-medium text-kon-ink`
    : `${base} text-kon-ink/70 hover:bg-kon-cream hover:text-kon-ink`
}

// App は全画面共通の骨組み。ヘッダはルートが変わっても残す。
//
// ルータ自体はここでは持たない。本番は BrowserRouter（main.tsx）、
// テストは MemoryRouter と包む側を変えられるようにするため。
export function App() {
  // ページ描画中にエラーが起きても、境界を現在地でキーづけしておくと
  // 別画面へ移った時点で境界が作り直され、自動で通常表示に戻る。
  const location = useLocation()

  return (
    <div className="min-h-screen bg-kon-cream/40 text-kon-ink">
      <header className="border-b border-kon-leaf-soft bg-white">
        {/* 狭い画面では折り返す。1行に押し込むと各リンクが縮んで和文が割れる。 */}
        <nav className="mx-auto flex max-w-3xl flex-wrap items-center gap-x-2 gap-y-1 px-4 py-3">
          {/* ホームへの導線。どの画面からでも起点に戻れるようにする。 */}
          <Link
            to="/"
            className="mr-2 flex items-center gap-2 whitespace-nowrap font-bold text-kon-ink"
          >
            {/* こんたてんは頭と体がひと続きなので、顔だけを切り出すと断面が出る。
                丸く抜いた枠に収めて上端を見せることで、断面を隠して顔を出す。 */}
            <span className="size-9 shrink-0 overflow-hidden rounded-full bg-kon-cream">
              <img
                src="/mascot/hero.png"
                alt=""
                className="size-full scale-125 object-cover object-top"
              />
            </span>
            献立くん
          </Link>
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={linkClass}
            >
              {item.label}
            </NavLink>
          ))}
          <AuthMenu />
        </nav>
      </header>

      <main className="mx-auto max-w-3xl px-4 py-8">
        {/* ページ本文だけを境界で包む。ヘッダは残し、他画面へ移れるようにする。 */}
        <ErrorBoundary key={location.pathname}>
          <Routes>
            <Route path="/" element={<HomePage />} />
            <Route path="/search" element={<SearchPage />} />
            <Route path="/weekly" element={<WeeklyPage />} />
            {/* 冷蔵庫から探すのは検索と同じ扱いで、未認証でも使える（spec.md 2.9）。 */}
            <Route path="/from-fridge" element={<SearchByIngredientsPage />} />
            {/* 買い物リストは週間献立から作る。未認証でも使える（spec.md 2.7）。 */}
            <Route path="/shopping-list" element={<ShoppingListPage />} />
            <Route path="/menus/:id" element={<MenuDetailPage />} />
            {/* 履歴とお気に入りは本人のものだけを扱うため認証必須。
                検索と週間献立は未認証でも使える（spec.md 1.3）。 */}
            <Route
              path="/histories"
              element={
                <RequireAuth>
                  <HistoryPage />
                </RequireAuth>
              }
            />
            {/* 保存はユーザーに紐づくため認証必須（spec.md 2.8）。 */}
            <Route
              path="/saved-weekly"
              element={
                <RequireAuth>
                  <SavedWeeklyPage />
                </RequireAuth>
              }
            />
            <Route
              path="/favorites"
              element={
                <RequireAuth>
                  <FavoritePage />
                </RequireAuth>
              }
            />
            {/* 加入は本人に紐づくため認証必須。 */}
            <Route
              path="/checkout"
              element={
                <RequireAuth>
                  <CheckoutPage />
                </RequireAuth>
              }
            />
            <Route
              path="/checkout/complete"
              element={
                <RequireAuth>
                  <CheckoutCompletePage />
                </RequireAuth>
              }
            />
            {/* プランの管理は本人のものだけを扱うため認証必須。 */}
            <Route
              path="/account"
              element={
                <RequireAuth>
                  <AccountPage />
                </RequireAuth>
              }
            />
            {/* 料金の提示は未ログインにも見せる。加入を検討する前に見る画面で、
                ログインを要求すると意味を成さない。 */}
            <Route path="/pricing" element={<PricingPage />} />
            <Route path="/login" element={<LoginPage />} />
            {/* 法務3ページは/loginと同じく未認証でも見える必要があるため、
                RequireAuth で包まない（表示義務のあるページのため）。 */}
            <Route path="/legal/tokushoho" element={<TokushohoPage />} />
            <Route path="/legal/terms" element={<TermsPage />} />
            <Route path="/legal/privacy" element={<PrivacyPage />} />
            {/* どれにも一致しないパスは404画面に落とす。 */}
            <Route path="*" element={<NotFoundPage />} />
          </Routes>
        </ErrorBoundary>
      </main>

      <Footer />
    </div>
  )
}
