import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useLocation, useNavigate } from 'react-router'

import type { User } from '../../api/types'
import { ErrorMessage } from '../../components/ErrorMessage'
import { googleLoginPath, login, signUp } from './api'
import { validateCredentials } from './validate'
import { meQueryKey } from './useCurrentUser'

type Mode = 'login' | 'signup'

const copy = {
  login: {
    heading: 'ログイン',
    submit: 'ログイン',
    switch: '新規登録はこちら',
  },
  signup: {
    heading: '新規登録',
    submit: '登録する',
    switch: 'ログインはこちら',
  },
} as const

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

  const submit = useMutation({
    mutationFn: (m: Mode) =>
      m === 'login' ? login({ email, password }) : signUp({ email, password }),
    onSuccess: (user: User) => {
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
      <h1 className="text-2xl font-bold">{text.heading}</h1>

      <form className="space-y-4" onSubmit={onSubmit} noValidate>
        <div>
          <label htmlFor={emailId} className="mb-1 block text-sm font-medium">
            メールアドレス
          </label>
          <input
            id={emailId}
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            autoComplete="email"
            className="w-full rounded-lg border border-slate-300 px-3 py-2"
          />
        </div>

        <div>
          <label htmlFor={passwordId} className="mb-1 block text-sm font-medium">
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
            className="w-full rounded-lg border border-slate-300 px-3 py-2"
          />
        </div>

        {/* 入力の誤りとサーバのエラーは同じ場所に出す。出所が違っても、
            利用者が知りたいのは「次に何をすればよいか」だけ。 */}
        {invalid ? (
          <p
            role="alert"
            className="rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 text-amber-900"
          >
            {invalid}
          </p>
        ) : (
          submit.error && <ErrorMessage error={submit.error} />
        )}

        <button
          type="submit"
          disabled={submit.isPending}
          className="w-full rounded-lg bg-emerald-600 px-6 py-2.5 font-medium text-white hover:bg-emerald-700 disabled:cursor-not-allowed disabled:bg-slate-300"
        >
          {submit.isPending ? '送信中…' : text.submit}
        </button>
      </form>

      <div className="border-t border-slate-200 pt-4">
        {/*
          Google認証は fetch ではなく通常の遷移で始める。
          OAuth はブラウザ自身がリダイレクトを辿る必要があり、
          fetch では同意画面に行けず Cookie も設定されない。
        */}
        <a
          href={googleLoginPath}
          className="block rounded-lg border border-slate-300 bg-white px-6 py-2.5 text-center font-medium text-slate-700 hover:bg-slate-50"
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
        className="text-sm text-emerald-700 underline"
      >
        {text.switch}
      </button>
    </section>
  )
}
