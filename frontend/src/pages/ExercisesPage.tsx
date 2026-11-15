// Screen 3 — Справочник упражнений: search, muscle-group filter, cards that
// expand into detail, and create/edit entry points.

import { useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { PageHeader } from '../components/AppShell'
import { Badge, Button, Card, EmptyState, ErrorBanner, Input, Skeleton } from '../components/ui'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { ApiError, exercisesApi, muscleGroupsApi } from '../lib/api'
import type { Exercise } from '../lib/types'
import { plural } from '../lib/format'
import { queryKeys } from '../lib/queryKeys'

export function ExercisesPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [groupId, setGroupId] = useState<number | undefined>(undefined)
  const [showArchived, setShowArchived] = useState(false)
  const [expanded, setExpanded] = useState<number | null>(null)
  const [toDelete, setToDelete] = useState<Exercise | null>(null)
  const [error, setError] = useState<string | null>(null)

  const groupsQuery = useQuery({ queryKey: queryKeys.muscleGroups, queryFn: muscleGroupsApi.list })
  const params = {
    ...(search.trim() ? { search: search.trim() } : {}),
    ...(groupId !== undefined ? { muscleGroupId: groupId } : {}),
    ...(showArchived ? { includeArchived: true } : {}),
  }
  const exercisesQuery = useQuery({
    queryKey: queryKeys.exercises(params),
    queryFn: () => exercisesApi.list(params),
  })

  const remove = useMutation({
    mutationFn: (id: number) => exercisesApi.remove(id),
    onSuccess: () => {
      setToDelete(null)
      void queryClient.invalidateQueries({ queryKey: ['exercises'] })
    },
    onError: (err) => {
      setToDelete(null)
      setError(err instanceof ApiError ? err.message : 'Не удалось удалить упражнение')
    },
  })

  const archive = useMutation({
    mutationFn: ({ id, archived }: { id: number; archived: boolean }) => exercisesApi.archive(id, archived),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['exercises'] }),
    onError: (err) => setError(err instanceof ApiError ? err.message : 'Не удалось изменить архив'),
  })

  const items = exercisesQuery.data ?? []
  const groups = groupsQuery.data ?? []

  const grouped = useMemo(() => {
    if (!showArchived) return items
    return [...items].sort((a, b) => Number(a.isArchived) - Number(b.isArchived))
  }, [items, showArchived])

  return (
    <div>
      <PageHeader
        title="Упражнения"
        subtitle={`${items.length} ${plural(items.length, 'упражнение', 'упражнения', 'упражнений')}`}
        actions={
          <Button variant="primary" onClick={() => navigate('/exercises/new')}>
            Создать
          </Button>
        }
      />

      <div className="mb-3">
        <Input
          type="search"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder="Поиск по названию"
          aria-label="Поиск по названию"
        />
      </div>

      <div className="-mx-1 mb-3 flex gap-2 overflow-x-auto px-1 pb-1 no-scrollbar" aria-label="Фильтр по группе мышц">
        <button type="button" onClick={() => setGroupId(undefined)} className={`chip ${groupId === undefined ? 'chip-active' : ''}`}>
          Все
        </button>
        {groups.map((group) => (
          <button
            key={group.id}
            type="button"
            onClick={() => setGroupId(group.id === groupId ? undefined : group.id)}
            className={`chip ${groupId === group.id ? 'chip-active' : ''}`}
          >
            {group.name}
          </button>
        ))}
      </div>

      <label className="mb-3 flex items-center gap-2 text-sm text-ink-muted">
        <input
          type="checkbox"
          checked={showArchived}
          onChange={(event) => setShowArchived(event.target.checked)}
          className="h-4 w-4 rounded border-line bg-canvas accent-accent"
        />
        Показывать архив
      </label>

      {error && (
        <div className="mb-3">
          <ErrorBanner message={error} onRetry={() => setError(null)} />
        </div>
      )}

      {exercisesQuery.isPending && (
        <div className="space-y-2">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </div>
      )}

      {exercisesQuery.isError && (
        <ErrorBanner message="Не удалось загрузить упражнения" onRetry={() => exercisesQuery.refetch()} />
      )}

      {exercisesQuery.data && grouped.length === 0 && (
        <Card>
          <EmptyState
            title={search || groupId ? 'Ничего не найдено' : 'Справочник пуст'}
            description={
              search || groupId
                ? 'Измените запрос или сбросьте фильтр по группе мышц.'
                : 'Добавьте упражнения, которые вы действительно выполняете — их можно будет выбирать в тренировке.'
            }
            action={
              !search && !groupId ? (
                <Button variant="primary" onClick={() => navigate('/exercises/new')}>
                  Создать упражнение
                </Button>
              ) : undefined
            }
          />
        </Card>
      )}

      <ul className="space-y-2">
        {grouped.map((exercise) => (
          <li key={exercise.id}>
            <Card className={exercise.isArchived ? 'opacity-70' : ''}>
              <button
                type="button"
                onClick={() => setExpanded(expanded === exercise.id ? null : exercise.id)}
                aria-expanded={expanded === exercise.id}
                className="flex w-full items-start justify-between gap-3 p-4 text-left"
              >
                <span className="min-w-0">
                  <span className="flex items-center gap-2">
                    <span className="truncate font-medium">{exercise.name}</span>
                    {exercise.isArchived && <Badge>архив</Badge>}
                  </span>
                  <span className="mt-1 flex flex-wrap gap-1">
                    {exercise.muscleGroups.length === 0 ? (
                      <span className="text-xs text-ink-faint">Без группы мышц</span>
                    ) : (
                      exercise.muscleGroups.map((group) => (
                        <span
                          key={group.muscleGroupId}
                          className={`rounded px-1.5 py-0.5 text-[11px] ${
                            group.isPrimary ? 'bg-accent-soft text-accent' : 'bg-surface-raised text-ink-muted'
                          }`}
                        >
                          {group.muscleGroupName}
                        </span>
                      ))
                    )}
                  </span>
                </span>
                <svg
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  className={`mt-1 h-4 w-4 shrink-0 text-ink-faint transition ${expanded === exercise.id ? 'rotate-180' : ''}`}
                >
                  <path d="M6 9l6 6 6-6" strokeLinecap="round" />
                </svg>
              </button>

              {expanded === exercise.id && (
                <div className="border-t border-line px-4 py-3 text-sm">
                  {exercise.description ? (
                    <p className="whitespace-pre-line text-ink-muted">{exercise.description}</p>
                  ) : (
                    <p className="text-ink-faint">Описание не заполнено.</p>
                  )}
                  {exercise.equipment && (
                    <p className="mt-2 text-ink-muted">
                      <span className="text-ink-faint">Снаряжение: </span>
                      {exercise.equipment}
                    </p>
                  )}
                  <div className="mt-3 flex flex-wrap gap-2">
                    <Link to={`/exercises/${exercise.id}`}>
                      <Button variant="secondary">Изменить</Button>
                    </Link>
                    <Button
                      variant="ghost"
                      onClick={() => archive.mutate({ id: exercise.id, archived: !exercise.isArchived })}
                      loading={archive.isPending}
                    >
                      {exercise.isArchived ? 'Вернуть из архива' : 'В архив'}
                    </Button>
                    <Button variant="ghost" onClick={() => setToDelete(exercise)} className="text-danger">
                      Удалить
                    </Button>
                  </div>
                  <p className="mt-2 text-xs text-ink-faint">
                    Упражнение с историей удалить нельзя — используйте архив, чтобы сохранить прошлые тренировки.
                  </p>
                </div>
              )}
            </Card>
          </li>
        ))}
      </ul>

      <ConfirmDialog
        open={toDelete !== null}
        title="Удалить упражнение?"
        description="Если упражнение уже используется в тренировках, удаление будет отклонено — тогда выберите архив."
        busy={remove.isPending}
        onConfirm={() => toDelete && remove.mutate(toDelete.id)}
        onCancel={() => setToDelete(null)}
      />
    </div>
  )
}
