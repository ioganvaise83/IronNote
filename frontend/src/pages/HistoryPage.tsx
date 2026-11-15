// Screen 4 — История. Reverse chronological, filterable by date, based on real
// sessions only.

import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { PageHeader } from '../components/AppShell'
import { Button, Card, EmptyState, ErrorBanner, Input, Skeleton } from '../components/ui'
import { sessionsApi } from '../lib/api'
import type { SessionFilter } from '../lib/types'
import { formatDate, formatDuration, formatTime, formatVolume, plural } from '../lib/format'
import { queryKeys } from '../lib/queryKeys'

type Preset = 'all' | '7d' | '30d' | '90d' | 'custom'

const presets: { id: Preset; label: string }[] = [
  { id: 'all', label: 'Всё время' },
  { id: '7d', label: '7 дней' },
  { id: '30d', label: '30 дней' },
  { id: '90d', label: '90 дней' },
  { id: 'custom', label: 'Период' },
]

export function HistoryPage() {
  const [preset, setPreset] = useState<Preset>('all')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')

  const filter: SessionFilter = useMemo(() => {
    const base: SessionFilter = { status: 'completed' }
    const today = new Date()
    if (preset === '7d' || preset === '30d' || preset === '90d') {
      const days = preset === '7d' ? 7 : preset === '30d' ? 30 : 90
      const start = new Date(today)
      start.setDate(start.getDate() - days)
      return { ...base, from: start.toISOString(), to: endOfToday(today) }
    }
    if (preset === 'custom') {
      return {
        ...base,
        ...(from ? { from: `${from}T00:00:00` } : {}),
        ...(to ? { to: to } : {}),
      }
    }
    return base
  }, [preset, from, to])

  const sessionsQuery = useQuery({
    queryKey: queryKeys.sessions(filter),
    queryFn: () => sessionsApi.list(filter),
  })

  const items = sessionsQuery.data ?? []

  return (
    <div>
      <PageHeader title="История" subtitle="Завершённые тренировки" />

      <div className="-mx-1 mb-4 flex gap-2 overflow-x-auto px-1 pb-1 no-scrollbar" role="tablist" aria-label="Фильтр по дате">
        {presets.map((item) => (
          <button
            key={item.id}
            type="button"
            role="tab"
            aria-selected={preset === item.id}
            onClick={() => setPreset(item.id)}
            className={`chip ${preset === item.id ? 'chip-active' : ''}`}
          >
            {item.label}
          </button>
        ))}
      </div>

      {preset === 'custom' && (
        <Card className="mb-4 grid grid-cols-2 gap-2 p-3">
          <label className="block">
            <span className="field-label">С</span>
            <Input type="date" value={from} onChange={(event) => setFrom(event.target.value)} max={to || undefined} />
          </label>
          <label className="block">
            <span className="field-label">По</span>
            <Input type="date" value={to} onChange={(event) => setTo(event.target.value)} min={from || undefined} />
          </label>
        </Card>
      )}

      {sessionsQuery.isPending && (
        <div className="space-y-2">
          <Skeleton className="h-20 w-full" />
          <Skeleton className="h-20 w-full" />
          <Skeleton className="h-20 w-full" />
        </div>
      )}

      {sessionsQuery.isError && (
        <ErrorBanner message="Не удалось загрузить историю" onRetry={() => sessionsQuery.refetch()} />
      )}

      {sessionsQuery.data && items.length === 0 && (
        <Card>
          <EmptyState
            title="Завершённых тренировок нет"
            description="Как только вы завершите первую тренировку, она появится здесь со всеми подходами."
            action={
              <Link to="/">
                <Button variant="primary">К тренировкам</Button>
              </Link>
            }
          />
        </Card>
      )}

      <ul className="space-y-2">
        {items.map((session) => (
          <li key={session.id}>
            <Link to={`/history/${session.id}`} className="block">
              <Card className="p-4 transition hover:border-line hover:bg-surface-raised">
                <div className="flex items-baseline justify-between gap-2">
                  <h2 className="min-w-0 truncate font-medium">{session.name}</h2>
                  <span className="shrink-0 text-sm tabular text-ink-faint">{formatTime(session.startedAt)}</span>
                </div>
                <p className="mt-0.5 text-sm text-ink-muted">{formatDate(session.startedAt)}</p>
                <dl className="mt-3 grid grid-cols-3 gap-2 text-sm">
                  <div>
                    <dt className="text-[11px] uppercase tracking-wide text-ink-faint">Упражнения</dt>
                    <dd className="tabular">{session.exerciseCount}</dd>
                  </div>
                  <div>
                    <dt className="text-[11px] uppercase tracking-wide text-ink-faint">Подходы</dt>
                    <dd className="tabular">{session.completedSetCount}</dd>
                  </div>
                  <div>
                    <dt className="text-[11px] uppercase tracking-wide text-ink-faint">Объём</dt>
                    <dd className="tabular">{formatVolume(session.volumeKg)}</dd>
                  </div>
                </dl>
                {session.durationMinutes !== null && (
                  <p className="mt-2 text-xs text-ink-faint">{formatDuration(session.durationMinutes)}</p>
                )}
              </Card>
            </Link>
          </li>
        ))}
      </ul>

      {items.length > 0 && (
        <p className="mt-4 text-center text-sm text-ink-faint">
          {items.length} {plural(items.length, 'тренировка', 'тренировки', 'тренировок')}
        </p>
      )}
    </div>
  )
}

function endOfToday(now: Date): string {
  const end = new Date(now)
  end.setHours(23, 59, 59, 999)
  return end.toISOString()
}
