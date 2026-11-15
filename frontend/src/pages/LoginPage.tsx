// Login screen: the only unauthenticated page.
//
// No "hint" text about which credentials might work, no distinction between a bad
// username and a bad password, and the server's generic message is shown as-is —
// the UI must not undo the API's anti-enumeration design.

import { useState, type FormEvent } from 'react'
import { Button, ErrorBanner, Field, Input } from '../components/ui'
import { ApiError } from '../lib/api'
import { useAuth } from '../lib/auth'

export function LoginPage() {
  const { login } = useAuth()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(event: FormEvent) {
    event.preventDefault()
    setError(null)
    setBusy(true)
    try {
      await login(username.trim(), password)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Не удалось войти')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-dvh flex-col justify-center bg-canvas px-5 py-10">
      <div className="mx-auto w-full max-w-sm">
        <div className="mb-8 text-center">
          <span className="mx-auto mb-3 grid h-14 w-14 place-items-center rounded-2xl bg-accent text-white">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" className="h-7 w-7">
              <path d="M4 9v6M7 6v12M17 6v12M20 9v6M7 12h10" strokeLinecap="round" />
            </svg>
          </span>
          <h1 className="text-2xl font-semibold tracking-tight">IronNote</h1>
          <p className="mt-1 text-sm text-ink-muted">Дневник силовых тренировок</p>
        </div>

        {error && (
          <div className="mb-4">
            <ErrorBanner message={error} />
          </div>
        )}

        <form onSubmit={submit} className="space-y-3">
          <Field label="Логин">
            <Input
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              autoComplete="username"
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              required
              disabled={busy}
            />
          </Field>
          <Field label="Пароль">
            <Input
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete="current-password"
              required
              disabled={busy}
            />
          </Field>
          <Button type="submit" variant="primary" block loading={busy} disabled={!username || !password}>
            Войти
          </Button>
        </form>

        <p className="mt-6 text-center text-xs text-ink-faint">
          Доступ только для участников. Учётные записи создаёт администратор.
        </p>
      </div>
    </div>
  )
}
