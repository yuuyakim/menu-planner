import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useLocation, useNavigate } from 'react-router'

import type { User } from '../../api/types'
import { ErrorMessage } from '../../components/ErrorMessage'
import { clearSessionState } from '../../hooks/useSessionState'
import { googleLoginPath, login, signUp } from './api'
import { validateCredentials } from './validate'
import { meQueryKey } from './useCurrentUser'

type Mode = 'login' | 'signup'

const copy = {
  login: {
    heading: 'ログイン',
    submit: 'ログイン',
    switch: '新規登録はこちら',
    // 戻ってきた人と初めての人では、かける言葉が違う。
    greeting: 'おかえり！待ってたよ',
  },
  signup: {
    heading: '新規登録',
    submit: '登録する',
    switch: 'ログインはこちら',
    greeting: 'はじめまして！よろしくね',
  },
} as const

// redirectedErrorText は backend から `/login?error=...` で戻されたときの文言。
//
// Googleログインのようにブラウザが直接APIへ遷移する経路では、失敗しても
// fetch のエラーとして扱えない。backend が生の JSON を見せる代わりに
// ここへ戻してくるので（Issue #81）、種別に応じた案内に翻訳する。
function redirectedErrorText(kind: string | null): string | undefined {
  if (!kind) return undefined
  switch (kind) {
    case 'rate-limited':
      return 'アクセスが集中しています。しばらく待ってからもう一度お試しください。'
    case 'google-auth-failed':
      return 'Googleでのログインに失敗しました。お手数ですが、もう一度お試しください。'
    case 'token-invalid':
    case 'user-not-found':
      return 'ログインの有効期限が切れました。もう一度ログインしてください。'
    default:
      // 想定外の種別でも黙らない。何か起きたことだけは必ず伝える。
      return 'ログインの処理中に問題が発生しました。もう一度お試しください。'
  }
}

// LoginPage はログインと新規登録を1画面で扱う。
// 入力項目が同じなので画面を分けず、切り替えで済ませる。
export function LoginPage() {
  const [mode, setMode] = useState<Mode>('login')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  // 送信前の検証で見つけた問題。サーバ由来のエラーとは別に持つ。
  const [invalid, setInvalid] = useState<string | undefined>(undefined)

  const emailId = useId()
  const passwordId = useId()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const location = useLocation()

  // 守られた画面から送られてきた場合は、そこへ戻す（RequireAuth が入れる）。
  const from = (location.state as { from?: string } | null)?.from ?? '/'

  // backend が画面遷移のエラーで戻してきた理由（Issue #81）。
  const redirected = redirectedErrorText(
    new URLSearchParams(location.search).get('error'),
  )

  // 入力の誤りを優先する。利用者が今まさに直せるのはそちらのため。
  const notice = invalid ?? redirected

  const submit = useMutation({
    mutationFn: (m: Mode) =>
      m === 'login' ? login({ email, password }) : signUp({ email, password }),
    onSuccess: (user: User) => {
      // 前のユーザーのキャッシュを先に捨てる。ログアウトを挟まずに
      // （セッション切れなどで）ログインし直した場合、これが無いと
      // 前のユーザーのお気に入りや履歴が表示されてしまう（Issue #78）。
      //
      // ここは resetQueries ではなく clear でよい。直後に新しいユーザーを
      // setQueryData で入れ、さらに画面遷移して他のクエリは張り直されるため、
      // 「取り直されず読み込み中のまま固まる」問題が起きない。
      // 余計な取り直しも走らせずに済む（ログアウト側は事情が違うので resetQueries）。
      queryClient.clear()
      // 週間献立は sessionStorage に持つため別途捨てる（Issue #83）。
      clearSessionState()
      // 取得済みのユーザーをキャッシュに入れ、遷移先で問い合わせ直さない。
      queryClient.setQueryData(meQueryKey, user)
      void navigate(from, { replace: true })
    },
  })

  const text = copy[mode]

  function onSubmit(e: React.FormEvent) {
    e.preventDefault()

    const problem = validateCredentials(email, password)
    setInvalid(problem)
    if (problem) return

    // 前回のサーバエラーが残ったままにならないようにする。
    submit.reset()
    submit.mutate(mode)
  }

  return (
    <section className="mx-auto max-w-sm space-y-6">
      {/* ここは何かが失敗した画面ではないので、こんたてんが迎える。
          絵は装飾（alt=""）で、挨拶は本文として持つ。 */}
      <div className="flex items-center gap-3">
        <img
          src="/mascot/face-happy.png"
          alt=""
          className="size-16 shrink-0"
        />
        <p className="relative rounded-2xl border border-kon-leaf-soft bg-white px-4 py-2 text-sm font-medium text-kon-ink">
          {text.greeting}
          <span
            aria-hidden="true"
            className="absolute -left-[7px] bottom-3 size-3 rotate-45 border-b border-l border-kon-leaf-soft bg-white"
          />
        </p>
      </div>

      <h1 className="text-2xl font-bold text-kon-ink">{text.heading}</h1>

      <form className="space-y-4" onSubmit={onSubmit} noValidate>
        <div>
          <label htmlFor={emailId} className="mb-1 block text-sm font-medium text-kon-ink">
            メールアドレス
          </label>
          <input
            id={emailId}
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            autoComplete="email"
            className="w-full rounded-xl border border-kon-leaf-soft bg-white px-3 py-2 text-kon-ink outline-none focus:border-kon-leaf"
          />
        </div>

        <div>
          <label htmlFor={passwordId} className="mb-1 block text-sm font-medium text-kon-ink">
            パスワード
          </label>
          <input
            id={passwordId}
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            // 新規登録では「新しいパスワード」として扱わせ、
            // 保存済みのものを提案させない。
            autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
            className="w-full rounded-xl border border-kon-leaf-soft bg-white px-3 py-2 text-kon-ink outline-none focus:border-kon-leaf"
          />
        </div>

        {/* 入力の誤りとサーバのエラーは同じ場所に出す。出所が違っても、
            利用者が知りたいのは「次に何をすればよいか」だけ。 */}
        {notice ? (
          <p
            role="alert"
            className="rounded-2xl border border-kon-peach bg-kon-peach/25 px-4 py-3 text-kon-ink"
          >
            {notice}
          </p>
        ) : (
          submit.error && <ErrorMessage error={submit.error} />
        )}

        <button
          type="submit"
          disabled={submit.isPending}
          className="w-full rounded-full bg-kon-leaf px-6 py-2.5 font-medium text-white transition-colors hover:brightness-95 disabled:cursor-not-allowed disabled:bg-kon-leaf-soft disabled:text-kon-ink/70"
        >
          {submit.isPending ? '送信中…' : text.submit}
        </button>
      </form>

      <div className="border-t border-kon-leaf-soft pt-4">
        {/*
          Google認証は fetch ではなく通常の遷移で始める。
          OAuth はブラウザ自身がリダイレクトを辿る必要があり、
          fetch では同意画面に行けず Cookie も設定されない。
        */}
        <a
          href={googleLoginPath}
          className="block rounded-full border border-kon-leaf-soft bg-white px-6 py-2.5 text-center font-medium text-kon-ink transition-colors hover:border-kon-leaf hover:bg-kon-cream"
        >
          Googleでログイン
        </a>
      </div>

      <button
        type="button"
        onClick={() => {
          setMode(mode === 'login' ? 'signup' : 'login')
          // 切り替え前のエラーは、切り替え後の操作とは無関係。
          setInvalid(undefined)
          submit.reset()
        }}
        className="text-sm text-kon-ink underline decoration-kon-leaf decoration-2 underline-offset-2"
      >
        {text.switch}
      </button>
    </section>
  )
}
