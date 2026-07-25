import { Link } from 'react-router'

// footerLinks は法務3ページへの導線。表示義務のあるページなので、
// どの画面からでも辿れるよう全ページ共通のフッターに置く。
const footerLinks = [
  { to: '/legal/tokushoho', label: '特定商取引法に基づく表記' },
  { to: '/legal/terms', label: '利用規約' },
  { to: '/legal/privacy', label: 'プライバシーポリシー' },
] as const

// Footer は全ページ下部に出る共通フッター。
// ヘッダのnavと違い、常設の主張は要らないため控えめな配色にとどめる。
export function Footer() {
  return (
    <footer className="border-t border-kon-leaf-soft bg-white">
      <nav className="mx-auto flex max-w-3xl flex-wrap justify-center gap-x-6 gap-y-2 px-4 py-6 text-sm text-kon-ink/70">
        {footerLinks.map((item) => (
          <Link
            key={item.to}
            to={item.to}
            className="whitespace-nowrap transition-colors hover:text-kon-ink"
          >
            {item.label}
          </Link>
        ))}
      </nav>
    </footer>
  )
}
