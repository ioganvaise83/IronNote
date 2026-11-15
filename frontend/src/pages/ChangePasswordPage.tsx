// Shown when the server says the password must be changed (first login after an
// admin created the account, or after an admin reset).

import { useState, type FormEvent } from 'react'
import { Button, Card, ErrorBanner, Field, Input } from '../components/ui'
import { ApiError } from '../lib/api'
import { useAuth } from '../lib/auth'

export function ChangePasswordPage({ required = false }: { required?: boolean }) {
  const { user, refresh, logout } = useAuth()
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [repeat, setRepeat] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [details, setDetails] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState(false)

  async function submit(event: FormEvent) {
    event.preventDefault()
    setError(null)
    setDetails({})
    if (next !== repeat) {
      setDetails({ repeat: 'Пароли не совпадают' })
      return
    }
    setBusy(true)
    try {
      const { authApi } = await import('../lib/api')
      await authApi.changePassword(current, next)
      await refresh()
      setDone(true)
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message)
        setDetails(err.details)
      } else {
        setError('Не удалось сменить пароль')
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="mx-auto max-w-md py-6">
      <h1 className="text-2xl font-semibold tracking-tight">Смена пароля</h1>
      <p className="mt-1 text-sm text-ink-muted">
        {required
          ? 'Администратор выдал временный пароль. Задайте свой, чтобы продолжить.'
          : `Вы вошли как ${user?.username}.`}
      </p>

      {error && (
        <div className="mt-4">
          <ErrorBanner message={error} details={details} />
        </div>
      )}

      {done && !required ? (
        <Card className="mt-4 p-4">
          <p className="text-sm text-success">Пароль изменён.</p>
        </Card>
      ) : (
        <Card className="mt-4 p-4">
          <form onSubmit={submit} className="space-y-3">
            <Field label="Текущий пароль" error={details.currentPassword}>
              <Input
                type="password"
                value={current}
                onChange={(event) => setCurrent(event.target.value)}
                autoComplete="current-password"
                required
                disabled={busy}
              />
            </Field>
            <Field
              label="Новый пароль"
              hint="Минимум 12 символов, буквы и хотя бы одна цифра или символ"
              error={details.newPassword}
            >
              <Input
                type="password"
                value={next}
                onChange={(event) => setNext(event.target.value)}
                autoComplete="new-password"
                minLength={12}
                required
                disabled={busy}
              />
            </Field>
            <Field label="Повторите новый пароль" error={details.repeat}>
              <Input
                type="password"
                value={repeat}
                onChange={(event) => setRepeat(event.target.value)}
                autoComplete="new-password"
                required
                disabled={busy}
              />
            </Field>
            <Button type="submit" variant="primary" block loading={busy}>
              Сохранить пароль
            </Button>
          </form>
        </Card>
      )}

      {required && (
        <Button variant="ghost" block className="mt-3" onClick={() => void logout()}>
          Выйти
        </Button>
      )}
    </div>
  )
}
