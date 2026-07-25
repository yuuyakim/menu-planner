import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

// LegalPage は法務文書の md をそのまま描画する。文言の正は docs/legal/*.md。
//
// @tailwindcss/typography（prose）は導入していないため、見出し・段落・表・
// リストへ Tailwind のクラスを直接当てる。表は remark-gfm で描画されるが、
// 狭い画面で崩れないよう横スクロールできるコンテナで包む。
export function LegalPage({ markdown }: { markdown: string }) {
  return (
    <article className="max-w-none space-y-4 text-kon-ink">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          h1: ({ children }) => (
            <h1 className="text-2xl font-bold text-kon-ink">{children}</h1>
          ),
          h2: ({ children }) => (
            <h2 className="mt-6 text-xl font-bold text-kon-ink">
              {children}
            </h2>
          ),
          h3: ({ children }) => (
            <h3 className="mt-4 text-lg font-bold text-kon-ink">
              {children}
            </h3>
          ),
          p: ({ children }) => (
            <p className="text-sm leading-relaxed text-kon-ink">{children}</p>
          ),
          ul: ({ children }) => (
            <ul className="list-disc space-y-1 pl-6 text-sm text-kon-ink">
              {children}
            </ul>
          ),
          ol: ({ children }) => (
            <ol className="list-decimal space-y-1 pl-6 text-sm text-kon-ink">
              {children}
            </ol>
          ),
          a: ({ children, href }) => (
            <a
              href={href}
              className="font-medium text-kon-ink underline decoration-kon-leaf decoration-2 underline-offset-2"
            >
              {children}
            </a>
          ),
          // 表は横に広がりやすいので、狭い画面ではコンテナ側でスクロールさせる。
          table: ({ children }) => (
            <div className="overflow-x-auto rounded-lg border border-kon-leaf-soft">
              <table className="w-full min-w-full border-collapse text-left text-sm">
                {children}
              </table>
            </div>
          ),
          thead: ({ children }) => (
            <thead className="bg-kon-cream">{children}</thead>
          ),
          th: ({ children }) => (
            <th className="border-b border-kon-leaf-soft px-3 py-2 font-bold text-kon-ink">
              {children}
            </th>
          ),
          td: ({ children }) => (
            <td className="border-b border-kon-leaf-soft px-3 py-2 text-kon-ink">
              {children}
            </td>
          ),
        }}
      >
        {markdown}
      </ReactMarkdown>
    </article>
  )
}
