import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { Route, Routes } from 'react-router'
import { describe, expect, it } from 'vitest'

import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { LoginPage } from './LoginPage'

const user = {
  id: '018f0000-0000-7000-8000-000000000001',
  email: 'user@example.com',
  displayName: 'ユーザー',
}

// ログイン成功後の遷移先を確かめるため、行き先のルートも用意する。
function renderLogin() {
  return renderWithProviders(
    <Routes>
      <Route path="/" element={<h1>献立を探す</h1>} />
      <Route path="/login" element={<LoginPage />} />
    </Routes>,
    { route: '/login' },
  )
}

// respondAuth は認証APIの応答を仕込み、送られた本文を記録する。
function respondAuth(path: 'login' | 'signup', status = 200) {
  const bodies: { email?: string; password?: string }[] = []
  server.use(
    http.post(`/api/v1/auth/${path}`, async ({ request }) => {
      bodies.push((await request.json()) as { email: string; password: string })
      return HttpResponse.json({ user }, { status })
    }),
  )
  return bodies
}

function respondProblem(path: 'login' | 'signup', status: number, title: string) {
  server.use(
    http.post(`/api/v1/auth/${path}`, () =>
      HttpResponse.json(
        { type: 'https://example.com/probs/x', title, status },
        { status, headers: { 'Content-Type': 'application/problem+json' } },
      ),
    ),
  )
}

async function fillIn(email: string, password: string) {
  const u = userEvent.setup()
  await u.clear(screen.getByLabelText('メールアドレス'))
  await u.type(screen.getByLabelText('メールアドレス'), email)
  await u.clear(screen.getByLabelText('パスワード'))
  await u.type(screen.getByLabelText('パスワード'), password)
  return u
}

describe('ログイン画面', () => {
  it('ログインフォームを表示する', () => {
    renderLogin()

    expect(screen.getByRole('heading', { level: 1, name: 'ログイン' })).toBeVisible()
    expect(screen.getByLabelText('メールアドレス')).toBeVisible()
    expect(screen.getByLabelText('パスワード')).toBeVisible()
    expect(screen.getByRole('button', { name: 'ログイン' })).toBeVisible()
  })

  it('入力を送信し、成功したら検索画面へ移る', async () => {
    const bodies = respondAuth('login')
    renderLogin()

    const u = await fillIn('user@example.com', 'password123')
    await u.click(screen.getByRole('button', { name: 'ログイン' }))

    expect(
      await screen.findByRole('heading', { name: '献立を探す' }),
    ).toBeVisible()
    expect(bodies[0]).toEqual({
      email: 'user@example.com',
      password: 'password123',
    })
  })

  it('401 はエラーを表示し、画面に留まる', async () => {
    respondProblem('login', 401, 'メールアドレスまたはパスワードが正しくありません')
    renderLogin()

    const u = await fillIn('user@example.com', 'wrongpassword')
    await u.click(screen.getByRole('button', { name: 'ログイン' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'メールアドレスまたはパスワードが正しくありません',
    )
    expect(screen.getByLabelText('メールアドレス')).toBeVisible()
  })

  it('Googleログインは通常のリンクで遷移する', () => {
    renderLogin()

    const link = screen.getByRole('link', { name: /Google/ })
    // OAuth はブラウザ自身がリダイレクトを辿る必要がある。
    // fetch で叩くとリダイレクト先に行けず、Cookie も設定されない。
    expect(link).toHaveAttribute('href', '/api/v1/auth/google')
  })
})

describe('新規登録', () => {
  async function switchToSignUp() {
    const u = userEvent.setup()
    await u.click(screen.getByRole('button', { name: '新規登録はこちら' }))
    return u
  }

  it('新規登録に切り替えられる', async () => {
    renderLogin()
    await switchToSignUp()

    expect(
      screen.getByRole('heading', { level: 1, name: '新規登録' }),
    ).toBeVisible()
    expect(screen.getByRole('button', { name: '登録する' })).toBeVisible()
  })

  it('メール形式が不正なら送信しない', async () => {
    const bodies = respondAuth('signup', 201)
    renderLogin()
    await switchToSignUp()

    const u = await fillIn('not-an-email', 'password123')
    await u.click(screen.getByRole('button', { name: '登録する' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'メールアドレスの形式',
    )
    // 手前で弾き、無駄な問い合わせをしない。
    expect(bodies).toHaveLength(0)
  })

  it('パスワードが8文字未満なら送信しない', async () => {
    const bodies = respondAuth('signup', 201)
    renderLogin()
    await switchToSignUp()

    const u = await fillIn('user@example.com', 'pass123')
    await u.click(screen.getByRole('button', { name: '登録する' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('8文字以上')
    expect(bodies).toHaveLength(0)
  })

  it('8文字ちょうどは受け付ける', async () => {
    const bodies = respondAuth('signup', 201)
    renderLogin()
    await switchToSignUp()

    const u = await fillIn('user@example.com', 'pass1234')
    await u.click(screen.getByRole('button', { name: '登録する' }))

    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0].password).toBe('pass1234')
  })

  it('登録に成功したら検索画面へ移る', async () => {
    respondAuth('signup', 201)
    renderLogin()
    await switchToSignUp()

    const u = await fillIn('user@example.com', 'password123')
    await u.click(screen.getByRole('button', { name: '登録する' }))

    expect(
      await screen.findByRole('heading', { name: '献立を探す' }),
    ).toBeVisible()
  })

  it('登録済みのメールは 409 のメッセージを出す', async () => {
    respondProblem('signup', 409, 'メールアドレスは既に登録されています')
    renderLogin()
    await switchToSignUp()

    const u = await fillIn('taken@example.com', 'password123')
    await u.click(screen.getByRole('button', { name: '登録する' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'メールアドレスは既に登録されています',
    )
  })

  it('入力し直せば検証メッセージは消える', async () => {
    respondAuth('signup', 201)
    renderLogin()
    await switchToSignUp()

    let u = await fillIn('not-an-email', 'password123')
    await u.click(screen.getByRole('button', { name: '登録する' }))
    expect(await screen.findByRole('alert')).toBeVisible()

    u = await fillIn('user@example.com', 'password123')
    await u.click(screen.getByRole('button', { name: '登録する' }))

    await waitFor(() =>
      expect(screen.queryByRole('alert')).not.toBeInTheDocument(),
    )
  })
})

describe('画面遷移でエラーになって戻されたとき（Issue #81）', () => {
  // backend がブラウザの画面遷移のエラーを /login?error=... に飛ばしてくる。
  // 生のJSONを見せない代わりに、ここで利用者向けの文言を出す。
  function renderLoginWithError(kind: string) {
    return renderWithProviders(
      <Routes>
        <Route path="/login" element={<LoginPage />} />
      </Routes>,
      { route: `/login?error=${kind}` },
    )
  }

  it('レート制限なら、時間をおくよう案内する', () => {
    renderLoginWithError('rate-limited')
    const alert = screen.getByRole('alert')
    expect(alert).toBeVisible()
    expect(alert.textContent).toMatch(/しばらく/)
  })

  it('Google認証の失敗なら、その旨を伝える', () => {
    renderLoginWithError('google-auth-failed')
    expect(screen.getByRole('alert').textContent).toMatch(/Google/)
  })

  it('未知の種別でも、何か問題が起きたことは伝える', () => {
    renderLoginWithError('something-unexpected')
    expect(screen.getByRole('alert')).toBeVisible()
  })

  it('error が無ければ何も出さない', () => {
    renderLogin()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})
