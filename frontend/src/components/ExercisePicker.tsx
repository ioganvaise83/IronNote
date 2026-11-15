// Bottom-sheet exercise picker used by the active workout.
//
// Reuses the catalogue query with a local search; no separate endpoint, so the
// list the user sees here is the same list they maintain in the catalogue.

import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { exercisesApi } from '../lib/api'
import { queryKeys } from '../lib/queryKeys'
import { EmptyState, ErrorBanner, Input, Skeleton } from './ui'

export function ExercisePicker({
  open,
  onClose,
  onPick,
  busy = false,
}: {
  open: boolean
  onClose: () => void
  onPick: (exerciseId: number) => void
  busy?: boolean
}) {
  const [search, setSearch] = useState('')
  const exercisesQuery = useQuery({
    queryKey: queryKeys.exercises({ includeArchived: false }),
    queryFn: () => exercisesApi.list({ includeArchived: false }),
    enabled: open,
  })

  const filtered = useMemo(() => {
    const term = search.trim().toLowerCase()
    const items = exercisesQuery.data ?? []
    if (!term) return items
    return items.filter((exercise) => exercise.name.toLowerCase().includes(term))
  }, [exercisesQuery.data, search])

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-end justify-center bg-black/70 lg:items-center"
      role="dialog"
      aria-modal="true"
      aria-label="Выбор упражнения"
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
    >
      <div className="flex max-h-[85dvh] w-full max-w-lg flex-col rounded-t-2xl border border-line bg-surface shadow-sheet lg:rounded-card">
        <div className="flex items-center justify-between gap-2 border-b border-line px-4 py-3">
          <h2 className="text-lg font-semibold">Добавить упражнение</h2>
          <button type="button" onClick={onClose} aria-label="Закрыть" className="grid h-9 w-9 place-items-center rounded-lg text-ink-muted hover:bg-surface-hover">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="h-5 w-5">
              <path d="M6 6l12 12M18 6L6 18" strokeLinecap="round" />
            </svg>
          </button>
        </div>

        <div className="border-b border-line p-3">
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Поиск упражнения"
            aria-label="Поиск упражнения"
            autoFocus
          />
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto p-2 safe-bottom">
          {exercisesQuery.isPending && (
            <div className="space-y-2 p-2">
              <Skeleton className="h-12 w-full" />
              <Skeleton className="h-12 w-full" />
            </div>
          )}
          {exercisesQuery.isError && (
            <div className="p-2">
              <ErrorBanner message="Не удалось загрузить упражнения" onRetry={() => exercisesQuery.refetch()} />
            </div>
          )}
          {exercisesQuery.data && filtered.length === 0 && (
            <EmptyState
              title={search ? 'Ничего не найдено' : 'Справочник пуст'}
              description={search ? 'Попробуйте другое название.' : 'Добавьте упражнения в разделе «Упражнения».'}
            />
          )}
          <ul>
            {filtered.map((exercise) => (
              <li key={exercise.id}>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => onPick(exercise.id)}
                  className="flex w-full items-center justify-between gap-3 rounded-xl px-3 py-3 text-left hover:bg-surface-hover disabled:opacity-50"
                >
                  <span className="min-w-0">
                    <span className="block truncate font-medium">{exercise.name}</span>
                    <span className="block truncate text-xs text-ink-faint">
                      {exercise.muscleGroups.map((g) => g.muscleGroupName).join(' · ') || 'Без группы мышц'}
                    </span>
                  </span>
                  <span className="text-accent">+</span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </div>
  )
}
