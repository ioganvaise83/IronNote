// Screen 1 — Тренировки.
//
// Primary job: get the user into a workout in one tap. If a session is already in
// flight it dominates the screen, because losing it is the worst failure mode.

import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { PageHeader } from '../components/AppShell'
import { Button, Card, EmptyState, ErrorBanner, Skeleton } from '../components/ui'
import { ApiError, sessionsApi, templatesApi } from '../lib/api'
import { formatDateTime, formatElapsed, formatVolume, plural } from '../lib/format'
import { queryKeys } from '../lib/queryKeys'

export function WorkoutsPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [startError, setStartError] = useState<string | null>(null)

  const activeQuery = useQuery({
    queryKey: queryKeys.activeSession,
    queryFn: sessionsApi.active,
  })
  const templatesQuery = useQuery({ queryKey: queryKeys.templates, queryFn: templatesApi.list })

  const startEmpty = useMutation({
    mutationFn: () => sessionsApi.start({ name: 'Тренировка' }),
    onSuccess: (session) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.activeSession })
      navigate(`/workouts/${session.id}`)
    },
    onError: (error) => setStartError(describeError(error)),
  })

  const startTemplate = useMutation({
    mutationFn: (id: number) => templatesApi.start(id),
    onSuccess: (session) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.activeSession })
      navigate(`/workouts/${session.id}`)
    },
    onError: (error) => setStartError(describeError(error)),
  })

  const active = activeQuery.data ?? null
  const busy = startEmpty.isPending || startTemplate.isPending

  return (
    <div>
      <PageHeader
        title="Тренировки"
        subtitle={formatDateTime(new Date().toISOString())}
        actions={
          <Button variant="primary" onClick={() => startEmpty.mutate()} loading={startEmpty.isPending}>
            Начать
          </Button>
        }
      />

      {startError && (
        <div className="mb-4">
          <ErrorBanner message={startError} onRetry={() => setStartError(null)} />
        </div>
      )}

      {activeQuery.isPending && (
        <Card className="mb-4 p-4">
          <Skeleton className="h-5 w-40" />
          <Skeleton className="mt-3 h-10 w-full" />
        </Card>
      )}

      {activeQuery.isError && (
        <div className="mb-4">
          <ErrorBanner message="Не удалось загрузить текущую тренировку" onRetry={() => activeQuery.refetch()} />
        </div>
      )}

      {active && (
        <Card className="mb-6 overflow-hidden border-accent/40">
          <div className="border-b border-line/60 bg-accent-soft/60 px-4 py-3">
            <p className="text-xs font-semibold uppercase tracking-wider text-accent">Продолжить</p>
          </div>
          <div className="p-4">
            <div className="flex items-baseline justify-between gap-3">
              <h2 className="min-w-0 truncate text-lg font-semibold">{active.name}</h2>
              <ElapsedBadge startedAt={active.startedAt} />
            </div>
            <p className="mt-1 text-sm text-ink-muted">
              {active.exerciseCount} {plural(active.exerciseCount, 'упражнение', 'упражнения', 'упражнений')}
              {' · '}
              {active.completedSetCount} {plural(active.completedSetCount, 'подход', 'подхода', 'подходов')}
              {' · '}
              {formatVolume(active.volumeKg)}
            </p>
            <Button
              variant="primary"
              block
              className="mt-4"
              onClick={() => navigate(`/workouts/${active.id}`)}
            >
              Продолжить тренировку
            </Button>
          </div>
        </Card>
      )}

      <section className="mb-2 flex items-center justify-between">
        <h2 className="section-title">Шаблоны</h2>
        <Link to="/templates/new" className="-mr-2 inline-flex min-h-tap items-center rounded-lg px-2 text-sm font-medium text-accent">
          Создать
        </Link>
      </section>

      {templatesQuery.isPending && (
        <div className="space-y-2">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </div>
      )}

      {templatesQuery.isError && (
        <ErrorBanner message="Не удалось загрузить шаблоны" onRetry={() => templatesQuery.refetch()} />
      )}

      {templatesQuery.data?.length === 0 && (
        <Card>
          <EmptyState
            title="Шаблонов пока нет"
            description="Шаблон — это заготовка тренировки: упражнения и планируемые подходы. Создайте его один раз и начинайте тренировку в одно нажатие."
            action={
              <Link to="/templates/new">
                <Button variant="primary">Создать шаблон</Button>
              </Link>
            }
          />
        </Card>
      )}

      <ul className="space-y-2">
        {templatesQuery.data?.map((template) => (
          <li key={template.id}>
            <Card className="p-4">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <Link
                    to={`/templates/${template.id}`}
                    className="block truncate text-base font-medium hover:text-accent"
                  >
                    {template.name}
                  </Link>
                  <p className="mt-0.5 text-sm text-ink-muted">
                    {template.exerciseCount}{' '}
                    {plural(template.exerciseCount, 'упражнение', 'упражнения', 'упражнений')}
                    {template.plannedSets > 0 && (
                      <>
                        {' · '}
                        {template.plannedSets} {plural(template.plannedSets, 'подход', 'подхода', 'подходов')}
                      </>
                    )}
                  </p>
                </div>
                <Button
                  variant="primary"
                  onClick={() => startTemplate.mutate(template.id)}
                  disabled={busy || !!active}
                  title={active ? 'Сначала завершите текущую тренировку' : undefined}
                >
                  Начать
                </Button>
              </div>
              {template.description && (
                <p className="mt-2 line-clamp-2 text-sm text-ink-faint">{template.description}</p>
              )}
            </Card>
          </li>
        ))}
      </ul>

      {!active && templatesQuery.data && templatesQuery.data.length > 0 && (
        <p className="mt-6 text-center text-sm text-ink-faint">
          Нужна тренировка без плана?{' '}
          <button
            type="button"
            onClick={() => startEmpty.mutate()}
            className="font-medium text-accent"
            disabled={busy}
          >
            Начать с нуля
          </button>
        </p>
      )}
    </div>
  )
}

/** Live elapsed time; a workout is judged partly by how long it takes. */
function ElapsedBadge({ startedAt }: { startedAt: string }) {
  const [now, setNow] = useState(() => new Date())
  useNowTick(setNow)
  return (
    <span className="shrink-0 rounded-lg bg-surface-raised px-2 py-1 text-sm tabular text-ink-muted">
      {formatElapsed(startedAt, now)}
    </span>
  )
}

function useNowTick(setNow: (d: Date) => void) {
  const [mounted, setMounted] = useState(false)
  if (!mounted) setMounted(true)
  // A single interval per badge; 15s is enough for a minute-resolution display
  // and avoids re-rendering the whole screen every second.
  useInterval(() => setNow(new Date()), mounted ? 15_000 : null)
}

import { useEffect } from 'react'

function useInterval(callback: () => void, delay: number | null) {
  useEffect(() => {
    if (delay === null) return
    const id = window.setInterval(callback, delay)
    return () => window.clearInterval(id)
  }, [callback, delay])
}

function describeError(error: unknown): string {
  if (error instanceof ApiError) return error.message
  return 'Не удалось начать тренировку'
}
