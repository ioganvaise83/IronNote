// Screen 6 — Настройки / Справочники. Muscle-group management plus app info.
// Kept intentionally small: this is a personal tool, not a preferences maze.

import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { PageHeader } from '../components/AppShell'
import { Button, Card, ErrorBanner, Field, Input, Skeleton } from '../components/ui'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { ApiError, exercisesApi, muscleGroupsApi } from '../lib/api'
import type { MuscleGroup } from '../lib/types'
import { plural } from '../lib/format'
import { queryKeys } from '../lib/queryKeys'

export function SettingsPage() {
  const queryClient = useQueryClient()
  const [error, setError] = useState<string | null>(null)
  const [details, setDetails] = useState<Record<string, string>>({})
  const [editing, setEditing] = useState<MuscleGroup | 'new' | null>(null)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [toDelete, setToDelete] = useState<MuscleGroup | null>(null)

  const groupsQuery = useQuery({ queryKey: queryKeys.muscleGroups, queryFn: muscleGroupsApi.list })
  const exercisesQuery = useQuery({
    queryKey: queryKeys.exercises({ includeArchived: false }),
    queryFn: () => exercisesApi.list({}),
  })

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: queryKeys.muscleGroups })
    void queryClient.invalidateQueries({ queryKey: ['exercises'] })
  }

  const save = useMutation({
    mutationFn: (payload: { id?: number; name: string; description: string }) =>
      payload.id === undefined
        ? muscleGroupsApi.create({ name: payload.name, description: payload.description })
        : muscleGroupsApi.update(payload.id, { name: payload.name, description: payload.description }),
    onSuccess: () => {
      invalidate()
      setEditing(null)
      setName('')
      setDescription('')
      setDetails({})
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        setError(err.message)
        setDetails(err.details)
      } else {
        setError('Не удалось сохранить группу мышц')
      }
    },
  })

  const remove = useMutation({
    mutationFn: (id: number) => muscleGroupsApi.remove(id),
    onSuccess: () => {
      invalidate()
      setToDelete(null)
    },
    onError: (err) => {
      setToDelete(null)
      setError(err instanceof ApiError ? err.message : 'Не удалось удалить группу')
    },
  })

  function openNew() {
    setEditing('new')
    setName('')
    setDescription('')
    setDetails({})
  }

  function openEdit(group: MuscleGroup) {
    setEditing(group)
    setName(group.name)
    setDescription(group.description)
    setDetails({})
  }

  function submit() {
    setError(null)
    if (!name.trim()) {
      setDetails({ name: 'Введите название' })
      return
    }
    save.mutate({
      ...(editing && editing !== 'new' ? { id: editing.id } : {}),
      name: name.trim(),
      description: description.trim(),
    })
  }

  return (
    <div>
      <PageHeader title="Настройки" subtitle="Справочники и данные" />

      {error && (
        <div className="mb-3">
          <ErrorBanner message={error} onRetry={() => setError(null)} />
        </div>
      )}

      <section className="mb-6">
        <div className="mb-2 flex items-center justify-between">
          <h2 className="section-title">Группы мышц</h2>
          <button
            type="button"
            onClick={openNew}
            className="-mr-2 inline-flex min-h-tap items-center rounded-lg px-2 text-sm font-medium text-accent"
          >
            Добавить
          </button>
        </div>

        {groupsQuery.isPending && <Skeleton className="h-40 w-full" />}
        {groupsQuery.isError && (
          <ErrorBanner message="Не удалось загрузить группы мышц" onRetry={() => groupsQuery.refetch()} />
        )}

        {groupsQuery.data && (
          <Card className="divide-y divide-line/60">
            {groupsQuery.data.map((group) => (
              <div key={group.id} className="flex items-center justify-between gap-2 px-4 py-3">
                <div className="min-w-0">
                  <p className="truncate font-medium">{group.name}</p>
                  {group.description && <p className="truncate text-xs text-ink-faint">{group.description}</p>}
                </div>
                <div className="flex shrink-0 gap-1">
                  <button
                    type="button"
                    onClick={() => openEdit(group)}
                    className="grid h-11 w-11 place-items-center rounded-lg text-ink-faint hover:bg-surface-hover hover:text-ink"
                    aria-label={`Изменить ${group.name}`}
                  >
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-4 w-4">
                      <path d="M4 20h4l10-10a2.8 2.8 0 0 0-4-4L4 16v4z" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  </button>
                  <button
                    type="button"
                    onClick={() => setToDelete(group)}
                    className="grid h-11 w-11 place-items-center rounded-lg text-ink-faint hover:bg-danger/10 hover:text-danger"
                    aria-label={`Удалить ${group.name}`}
                  >
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-4 w-4">
                      <path d="M6 7h12M10 7V5h4v2M8 7l1 12h6l1-12" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  </button>
                </div>
              </div>
            ))}
          </Card>
        )}

        {editing && (
          <Card className="mt-3 space-y-3 p-4">
            <h3 className="font-medium">{editing === 'new' ? 'Новая группа' : 'Изменить группу'}</h3>
            <Field label="Название" error={details.name}>
              <Input value={name} onChange={(event) => setName(event.target.value)} maxLength={120} autoFocus />
            </Field>
            <Field label="Описание" hint="Необязательно">
              <Input value={description} onChange={(event) => setDescription(event.target.value)} />
            </Field>
            <div className="flex gap-2">
              <Button variant="ghost" block onClick={() => setEditing(null)}>
                Отмена
              </Button>
              <Button variant="primary" block onClick={submit} loading={save.isPending}>
                Сохранить
              </Button>
            </div>
          </Card>
        )}
      </section>

      <section className="mb-6">
        <h2 className="section-title mb-2">Упражнения</h2>
        <Card className="p-4">
          <p className="text-sm text-ink-muted">
            В справочнике {exercisesQuery.data?.length ?? 0}{' '}
            {plural(exercisesQuery.data?.length ?? 0, 'упражнение', 'упражнения', 'упражнений')}.
          </p>
          <div className="mt-3 flex flex-wrap gap-2">
            <Link to="/exercises">
              <Button variant="secondary">Открыть справочник</Button>
            </Link>
            <Link to="/exercises/new">
              <Button variant="ghost">Добавить упражнение</Button>
            </Link>
          </div>
        </Card>
      </section>

      <section className="mb-6">
        <h2 className="section-title mb-2">О приложении</h2>
        <Card className="space-y-2 p-4 text-sm text-ink-muted">
          <p>
            <span className="text-ink">IronNote</span> — личный дневник силовых тренировок. Он не составляет
            программы, не даёт советов и не анализирует вас: только записывает фактические подходы и считает
            статистику по ним.
          </p>
          <p>
            Все данные хранятся в PostgreSQL на вашем собственном сервере. Приложение рассчитано на одного
            пользователя и не требует регистрации.
          </p>
        </Card>
      </section>

      <ConfirmDialog
        open={toDelete !== null}
        title={`Удалить группу «${toDelete?.name ?? ''}»?`}
        description="Группу, которая используется в упражнениях, удалить нельзя — сначала уберите её из упражнений."
        busy={remove.isPending}
        onConfirm={() => toDelete && remove.mutate(toDelete.id)}
        onCancel={() => setToDelete(null)}
      />
    </div>
  )
}
