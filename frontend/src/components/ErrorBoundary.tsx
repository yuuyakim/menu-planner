import { Component, type ErrorInfo, type ReactNode } from 'react'

interface Props {
  children: ReactNode
  /** エラー時に表示する内容。省略時は既定のフォールバックを出す。 */
  fallback?: ReactNode
}

interface State {
  hasError: boolean
}

// ErrorBoundary は配下の描画時エラーを受け止め、真っ白な画面の代わりに
// 復帰導線付きのフォールバックを表示する。
//
// 描画中に投げられたエラーは Hooks では捕捉できないため、
// error boundary はクラスコンポーネントでしか書けない（Reactの制約）。
export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false }

  static getDerivedStateFromError(): State {
    return { hasError: true }
  }

  componentDidCatch(error: unknown, info: ErrorInfo): void {
    // 原因を追えるようコンソールに残す。本番は監視へ送る差し替え先にできる。
    console.error('画面の描画でエラーが発生しました', error, info)
  }

  render(): ReactNode {
    if (this.state.hasError) {
      return this.props.fallback ?? <DefaultFallback />
    }
    return this.props.children
  }
}

// DefaultFallback は既定のフォールバック。エラーだと伝え、再読み込みで復帰させる。
// role="alert" にして、支援技術にも異常を知らせる。
function DefaultFallback() {
  return (
    <section role="alert">
      <h1 className="text-2xl font-bold">問題が発生しました</h1>
      <p className="mt-2 text-slate-600">
        画面の表示中に予期しないエラーが発生しました。お手数ですが再読み込みしてください。
      </p>
      <button
        type="button"
        onClick={() => {
          window.location.reload()
        }}
        className="mt-4 inline-block rounded bg-emerald-700 px-4 py-2 text-white hover:bg-emerald-800"
      >
        再読み込み
      </button>
    </section>
  )
}
