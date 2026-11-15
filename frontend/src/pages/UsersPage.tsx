// Admin-only user management.
//
// Safety rails visible in the UI: you cannot demote, disable or delete the last
// active administrator, and you cannot delete or disable yourself. The server
// enforces the same rules; this is clarity, not security.

import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { PageHeader } from '../components/AppShell'
import { Badge, Button, Card, ErrorBanner, Field, Input, Skeleton } from '../components/ui'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { ApiError, usersApi } from '../lib/api'
import type { CurrentUser, UserRole } from '../lib/types'
import { formatDateTime } from '../lib/format'
import { useAuth } from '../lib/auth'

export function UsersPage() {
  const queryClient = useQueryClient()
  const { user: me } = useAuth()
  const [error, setError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [resetTarget, setResetTarget] = useState<CurrentUser | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<CurrentUser | null>(null)

  const usersQuery = useQuery({ queryKey: ['users'], queryFn: usersApi.list })
  const invalidate = () => void queryClient.invalidateQueries({ queryKey: ['users'] })

  const update = useMutation({
    mutationFn: (vars: { id: number; body: { role?: UserRole; isActive?: boolean } }) =>
      usersApi.update(vars.id, vars.body),
    onSuccess: invalidate,
    onError: (err) => setError(err instanceof ApiError ? err.message : 'Не удалось изменить пользователя'),
  })

  const remove = useMutation({
    mutationFn: (id: number) => usersApi.remove(id),
    onSuccess: () => {
      setDeleteTarget(null)
      invalidate()
    },
    onError: (err) => {
      setDeleteTarget(null)
      setError(err instanceof ApiError ? err.message : 'Не удалось удалить пользователя')
    },
  })

  const users = usersQuery.data ?? []
  const activeAdmins = users.filter((u) => u.role === 'admin' && u.isActive).length

  return (
    <div>
      <PageHeader
        title="Пользователи"
        back={{ to: '/settings' }}
        subtitle={`${users.length} · администраторов: ${activeAdmins}`}
        actions={
          <Button variant="primary" onClick={() => setCreating(true)}>
            Добавить
          </Button>
        }
      />

      {error && (
        <div className="mb-3">
          <ErrorBanner message={error} onRetry={() => setError(null)} />
        </div>
      )}

      {creating && (
        <CreateUserCard
          onClose={() => setCreating(false)}
          onCreated={() => {
            setCreating(false)
            invalidate()
          }}
          onError={setError}
        />
      )}

      {usersQuery.isPending && (
        <div className="space-y-2">
          <Skeleton className="h-20 w-full" />
          <Skeleton className="h-20 w-full" />
        </div>
      )}

      {usersQuery.isError && (
        <ErrorBanner message="Не удалось загрузить список" onRetry={() => usersQuery.refetch()} />
      )}

      <ul className="space-y-2">
        {users.map((user) => {
          const isMe = user.id === me?.id
          const lastAdmin = user.role === 'admin' && user.isActive && activeAdmins <= 1
          return (
            <li key={user.id}>
              <Card className={`p-4 ${user.isActive ? '' : 'opacity-70'}`}>
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="flex flex-wrap items-center gap-2">
                      <span className="truncate font-medium">{user.username}</span>
                      {user.role === 'admin' && <Badge tone="accent">админ</Badge>}
                      {!user.isActive && <Badge tone="warning">отключён</Badge>}
                      {user.mustChangePassword && <Badge>сменит пароль</Badge>}
                      {isMe && <Badge tone="success">это вы</Badge>}
                    </p>
                    <p className="mt-0.5 text-sm text-ink-muted">
                      {user.displayName || 'Без имени'}
                    </p>
                    <p className="mt-1 text-xs text-ink-faint">
                      {user.lastLoginAt ? `Вход: ${formatDateTime(user.lastLoginAt)}` : 'Ещё не входил'}
                    </p>
                  </div>
                </div>

                <div className="mt-3 flex flex-wrap gap-2">
                  <Button
                    variant="ghost"
                    disabled={isMe || lastAdmin || update.isPending}
                    onClick={() =>
                      update.mutate({
                        id: user.id,
                        body: { role: user.role === 'admin' ? 'user' : 'admin' },
                      })
                    }
                  >
                    {user.role === 'admin' ? 'Сделать пользователем' : 'Сделать админом'}
                  </Button>
                  <Button
                    variant="ghost"
                    disabled={isMe || lastAdmin || update.isPending}
                    onClick={() => update.mutate({ id: user.id, body: { isActive: !user.isActive } })}
                  >
                    {user.isActive ? 'Отключить' : 'Включить'}
                  </Button>
                  <Button variant="ghost" onClick={() => setResetTarget(user)}>
                    Сбросить пароль
                  </Button>
                  <Button
                    variant="ghost"
                    className="text-danger"
                    disabled={isMe || lastAdmin}
                    onClick={() => setDeleteTarget(user)}
                  >
                    Удалить
                  </Button>
                </div>
                {lastAdmin && (
                  <p className="mt-2 text-xs text-warning">
                    Это последний активный администратор — его нельзя понизить, отключить или удалить.
                  </p>
                )}
              </Card>
            </li>
          )
        })}
      </ul>

      <ResetPasswordDialog target={resetTarget} onClose={() => setResetTarget(null)} onError={setError} />

      <ConfirmDialog
        open={deleteTarget !== null}
        title={`Удалить пользователя «${deleteTarget?.username ?? ''}»?`}
        description="Вместе с ним будут удалены все его тренировки, шаблоны и история. Это действие необратимо. Чтобы сохранить данные, используйте «Отключить»."
        busy={remove.isPending}
        onConfirm={() => deleteTarget && remove.mutate(deleteTarget.id)}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  )
}

function CreateUserCard({
  onClose,
  onCreated,
  onError,
}: {
  onClose: () => void
  onCreated: () => void
  onError: (message: string) => void
}) {
  const [username, setUsername] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState<UserRole>('user')
  const [details, setDetails] = useState<Record<string, string>>({})

  const create = useMutation({
    mutationFn: () => usersApi.create({ username: username.trim(), displayName, password, role }),
    onSuccess: onCreated,
    onError: (err) => {
      if (err instanceof ApiError) {
        setDetails(err.details)
        onError(err.message)
      } else {
        onError('Не удалось создать пользователя')
      }
    },
  })

  return (
    <Card className="mb-3 space-y-3 p-4">
      <h2 className="font-medium">Новый пользователь</h2>
      <Field label="Логин" hint="Латинские буквы, цифры, точка, дефис" error={details.username}>
        <Input
          value={username}
          onChange={(event) => setUsername(event.target.value)}
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          maxLength={32}
        />
      </Field>
      <Field label="Имя" hint="Как обращаться к человеку">
        <Input value={displayName} onChange={(event) => setDisplayName(event.target.value)} />
      </Field>
      <Field
        label="Временный пароль"
        hint="Минимум 12 символов. Пользователь сменит его при первом входе."
        error={details.newPassword ?? details.password}
      >
        <Input
          type="text"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          autoComplete="off"
          spellCheck={false}
        />
      </Field>
      <label className="block">
        <span className="field-label">Роль</span>
        <select value={role} onChange={(event) => setRole(event.target.value as UserRole)} className="field">
          <option value="user">Пользователь — только свои тренировки</option>
          <option value="admin">Администратор — справочники и пользователи</option>
        </select>
      </label>
      <div className="flex gap-2">
        <Button variant="ghost" block onClick={onClose}>
          Отмена
        </Button>
        <Button
          variant="primary"
          block
          loading={create.isPending}
          disabled={!username.trim() || password.length < 12}
          onClick={() => create.mutate()}
        >
          Создать
        </Button>
      </div>
    </Card>
  )
}

function ResetPasswordDialog({
  target,
  onClose,
  onError,
}: {
  target: CurrentUser | null
  onClose: () => void
  onError: (message: string) => void
}) {
  const [password, setPassword] = useState('')

  const reset = useMutation({
    mutationFn: () => usersApi.resetPassword(target!.id, password),
    onSuccess: () => {
      setPassword('')
      onClose()
    },
    onError: (err) => onError(err instanceof ApiError ? err.message : 'Не удалось сбросить пароль'),
  })

  if (!target) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-end justify-center bg-black/70 p-4 sm:items-center"
      role="dialog"
      aria-modal="true"
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
    >
      <div className="w-full max-w-sm rounded-card border border-line bg-surface p-5">
        <h2 className="text-lg font-semibold">Сброс пароля</h2>
        <p className="mt-1 text-sm text-ink-muted">
          {target.username} получит временный пароль и должен будет сменить его при входе. Все его
          активные сеансы будут завершены.
        </p>
        <div className="mt-3">
          <Field label="Новый временный пароль" hint="Минимум 12 символов">
            <Input
              type="text"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete="off"
              spellCheck={false}
            />
          </Field>
        </div>
        <div className="mt-4 flex gap-2">
          <Button variant="secondary" block onClick={onClose}>
            Отмена
          </Button>
          <Button
            variant="primary"
            block
            loading={reset.isPending}
            disabled={password.length < 12}
            onClick={() => reset.mutate()}
          >
            Сбросить
          </Button>
        </div>
      </div>
    </div>
  )
}
