import { NavLink, Route, Routes } from 'react-router'

import { NotFoundPage } from '../components/NotFoundPage'
import { LoginPage } from '../features/auth/LoginPage'
import { FavoritePage } from '../features/favorite/FavoritePage'
import { HistoryPage } from '../features/history/HistoryPage'
import { MenuDetailPage } from '../features/menu/MenuDetailPage'
import { SearchPage } from '../features/menu/SearchPage'
import { WeeklyPage } from '../features/menu/WeeklyPage'

// navItems はヘッダに並べるリンク。増減はここだけで済ませる。
const navItems = [
  { to: '/', label: '献立を探す' },
  { to: '/weekly', label: '1週間の献立' },
  { to: '/histories', label: '履歴' },
  { to: '/favorites', label: 'お気に入り' },
] as const

// linkClass は現在地のリンクだけ色を変える。
// NavLink が渡す isActive を使い、現在地の判定を自前で持たない。
function linkClass({ isActive }: { isActive: boolean }): string {
  return isActive
    ? 'text-emerald-700 font-medium'
    : 'text-slate-600 hover:text-slate-900'
}

// App は全画面共通の骨組み。ヘッダはルートが変わっても残す。
//
// ルータ自体はここでは持たない。本番は BrowserRouter（main.tsx）、
// テストは MemoryRouter と包む側を変えられるようにするため。
export function App() {
  return (
    <div className="min-h-screen bg-slate-50 text-slate-900">
      <header className="border-b border-slate-200 bg-white">
        <nav className="mx-auto flex max-w-3xl gap-6 px-4 py-4">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              // '/' は前方一致だと全ページに一致してしまうため完全一致にする。
              end={item.to === '/'}
              className={linkClass}
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
      </header>

      <main className="mx-auto max-w-3xl px-4 py-8">
        <Routes>
          <Route path="/" element={<SearchPage />} />
          <Route path="/weekly" element={<WeeklyPage />} />
          <Route path="/menus/:id" element={<MenuDetailPage />} />
          <Route path="/histories" element={<HistoryPage />} />
          <Route path="/favorites" element={<FavoritePage />} />
          <Route path="/login" element={<LoginPage />} />
          {/* どれにも一致しないパスは404画面に落とす。 */}
          <Route path="*" element={<NotFoundPage />} />
        </Routes>
      </main>
    </div>
  )
}
