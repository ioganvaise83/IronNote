// Screen 5 — Статистика. Deliberately plain: period selector, four headline
// numbers, sessions by month, and per-exercise progress charts. No forecasting,
// no advice — the requirement is explicit about that.

import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Bar,
  BarChart,
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { PageHeader } from '../components/AppShell'
import { Card, EmptyState, ErrorBanner, Skeleton } from '../components/ui'
import { exercisesApi, statsApi } from '../lib/api'
import type { StatsPeriod } from '../lib/types'
import {
  formatDateShort,
  formatNumber,
  formatVolume,
  formatWeight,
  monthLabel,
  plural,
} from '../lib/format'
import { queryKeys } from '../lib/queryKeys'

type Range = '30d' | '90d' | '180d' | '365d' | 'all'

const ranges: { id: Range; label: string }[] = [
  { id: '30d', label: '30 дней' },
  { id: '90d', label: '3 месяца' },
  { id: '180d', label: '6 месяцев' },
  { id: '365d', label: 'Год' },
  { id: 'all', label: 'Всё' },
]

function periodFor(range: Range): StatsPeriod {
  if (range === 'all') return {}
  const days = range === '30d' ? 30 : range === '90d' ? 90 : range === '180d' ? 180 : 365
  const from = new Date()
  from.setDate(from.getDate() - days)
  return { from: from.toISOString().slice(0, 10) }
}

export function StatsPage() {
  const [range, setRange] = useState<Range>('90d')
  const [exerciseId, setExerciseId] = useState<number | null>(null)
  const period = useMemo(() => periodFor(range), [range])

  const summaryQuery = useQuery({
    queryKey: queryKeys.statsSummary(period),
    queryFn: () => statsApi.summary(period),
  })
  const bestsQuery = useQuery({
    queryKey: queryKeys.statsExercises(period),
    queryFn: () => statsApi.exercises(period),
  })

  const exerciseQuery = useQuery({
    queryKey: queryKeys.statsExercise(exerciseId ?? 0, period),
    queryFn: () => statsApi.exercise(exerciseId as number, period),
    enabled: exerciseId !== null,
  })

  const summary = summaryQuery.data
  const monthly = summary?.monthly.map((point) => ({ ...point, label: monthLabel(point.month) })) ?? []
  const series = exerciseQuery.data?.series.map((point) => ({
    ...point,
    label: formatDateShort(point.date),
    volume: Math.round(point.volumeKg),
  })) ?? []

  return (
    <div>
      <PageHeader title="Статистика" subtitle="Только по завершённым тренировкам" />

      <div className="-mx-1 mb-4 flex gap-2 overflow-x-auto px-1 pb-1 no-scrollbar" role="tablist" aria-label="Период">
        {ranges.map((item) => (
          <button
            key={item.id}
            type="button"
            role="tab"
            aria-selected={range === item.id}
            onClick={() => setRange(item.id)}
            className={`chip ${range === item.id ? 'chip-active' : ''}`}
          >
            {item.label}
          </button>
        ))}
      </div>

      {summaryQuery.isPending && (
        <div className="space-y-3">
          <Skeleton className="h-28 w-full" />
          <Skeleton className="h-48 w-full" />
        </div>
      )}

      {summaryQuery.isError && (
        <ErrorBanner message="Не удалось загрузить статистику" onRetry={() => summaryQuery.refetch()} />
      )}

      {summary && (
        <>
          <Card className="mb-3 p-4">
            <dl className="grid grid-cols-2 gap-4">
              <Stat label="Тренировок" value={String(summary.sessionCount)} />
              <Stat label="Подходов" value={String(summary.completedSetCount)} />
              <Stat label="Общий объём" value={formatVolume(summary.volumeKg)} />
              <Stat
                label="Средняя длительность"
                value={summary.averageDurationMinutes > 0 ? `${Math.round(summary.averageDurationMinutes)} мин` : '—'}
              />
            </dl>
          </Card>

          {summary.sessionCount === 0 ? (
            <Card>
              <EmptyState
                title="За этот период нет тренировок"
                description="Выберите другой период или завершите тренировку, чтобы увидеть статистику."
              />
            </Card>
          ) : (
            <>
              <Card className="mb-3 p-4">
                <h2 className="section-title mb-3">Тренировки по месяцам</h2>
                <div className="h-40 w-full">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart data={monthly} margin={{ top: 4, right: 4, bottom: 0, left: -20 }}>
                      <CartesianGrid stroke="#262c36" strokeDasharray="3 3" vertical={false} />
                      <XAxis dataKey="label" tick={{ fill: '#9aa4b2', fontSize: 11 }} axisLine={false} tickLine={false} />
                      <YAxis allowDecimals={false} tick={{ fill: '#6b7482', fontSize: 11 }} axisLine={false} tickLine={false} />
                      <Tooltip
                        contentStyle={tooltipStyle}
                        formatter={(value: number) => [`${value}`, 'тренировок']}
                        labelStyle={{ color: '#f2f4f7' }}
                      />
                      <Bar dataKey="sessionCount" fill="#2f7cf6" radius={[4, 4, 0, 0]} maxBarSize={40} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              </Card>

              <Card className="mb-3 p-4">
                <h2 className="section-title mb-2">Упражнение</h2>
                <select
                  value={exerciseId ?? ''}
                  onChange={(event) => setExerciseId(event.target.value ? Number(event.target.value) : null)}
                  className="field"
                  aria-label="Выберите упражнение"
                >
                  <option value="">Выберите упражнение</option>
                  {bestsQuery.data?.map((item) => (
                    <option key={item.exerciseId} value={item.exerciseId}>
                      {item.exerciseName}
                    </option>
                  ))}
                </select>

                {bestsQuery.isError && (
                  <p className="mt-2 text-sm text-danger">Не удалось загрузить список упражнений.</p>
                )}

                {exerciseId === null && bestsQuery.data?.length === 0 && (
                  <p className="mt-3 text-sm text-ink-muted">
                    За выбранный период ещё нет выполненных подходов с указанным весом.
                  </p>
                )}

                {exerciseQuery.isPending && exerciseId !== null && (
                  <div className="mt-3 space-y-2">
                    <Skeleton className="h-6 w-32" />
                    <Skeleton className="h-40 w-full" />
                  </div>
                )}

                {exerciseQuery.data && exerciseId !== null && (
                  <div className="mt-3">
                    <dl className="grid grid-cols-3 gap-2 text-sm">
                      <div>
                        <dt className="text-[11px] uppercase tracking-wide text-ink-faint">Макс. вес</dt>
                        <dd className="font-semibold tabular">{formatWeight(exerciseQuery.data.maxWeightKg)} кг</dd>
                      </div>
                      <div>
                        <dt className="text-[11px] uppercase tracking-wide text-ink-faint">Факт. 1ПМ</dt>
                        <dd className="font-semibold tabular">
                          {exerciseQuery.data.maxSingleKg === null
                            ? '—'
                            : `${formatWeight(exerciseQuery.data.maxSingleKg)} кг`}
                        </dd>
                      </div>
                      <div>
                        <dt className="text-[11px] uppercase tracking-wide text-ink-faint">Расч. 1ПМ</dt>
                        <dd className="font-semibold tabular text-accent">
                          {exerciseQuery.data.estimated1RMKg === null
                            ? '—'
                            : `${formatWeight(exerciseQuery.data.estimated1RMKg)} кг`}
                        </dd>
                      </div>
                    </dl>
                    <p className="mt-1 text-[11px] text-ink-faint">
                      Расчётный 1ПМ — оценка по формуле Эпли: вес × (1 + повторения / 30). Это не фактический
                      максимум; фактический учитывается только для подходов на 1 повторение.
                    </p>

                    {series.length > 0 ? (
                      <>
                        <h3 className="mt-4 text-xs font-medium uppercase tracking-wide text-ink-faint">
                          Объём по тренировкам, кг
                        </h3>
                        <div className="h-40 w-full">
                          <ResponsiveContainer width="100%" height="100%">
                            <LineChart data={series} margin={{ top: 8, right: 8, bottom: 0, left: -12 }}>
                              <CartesianGrid stroke="#262c36" strokeDasharray="3 3" vertical={false} />
                              <XAxis dataKey="label" tick={{ fill: '#9aa4b2', fontSize: 11 }} axisLine={false} tickLine={false} />
                              <YAxis tick={{ fill: '#6b7482', fontSize: 11 }} axisLine={false} tickLine={false} />
                              <Tooltip
                                contentStyle={tooltipStyle}
                                labelStyle={{ color: '#f2f4f7' }}
                                formatter={(value: number) => [`${formatNumber(value)} кг`, 'объём']}
                              />
                              <Line
                                type="monotone"
                                dataKey="volume"
                                stroke="#2f7cf6"
                                strokeWidth={2}
                                dot={{ r: 3, fill: '#2f7cf6' }}
                                activeDot={{ r: 5 }}
                              />
                            </LineChart>
                          </ResponsiveContainer>
                        </div>

                        <h3 className="mt-4 text-xs font-medium uppercase tracking-wide text-ink-faint">
                          Расчётный 1ПМ, кг
                        </h3>
                        <div className="h-40 w-full">
                          <ResponsiveContainer width="100%" height="100%">
                            <LineChart data={series} margin={{ top: 8, right: 8, bottom: 0, left: -12 }}>
                              <CartesianGrid stroke="#262c36" strokeDasharray="3 3" vertical={false} />
                              <XAxis dataKey="label" tick={{ fill: '#9aa4b2', fontSize: 11 }} axisLine={false} tickLine={false} />
                              <YAxis tick={{ fill: '#6b7482', fontSize: 11 }} axisLine={false} tickLine={false} />
                              <Tooltip
                                contentStyle={tooltipStyle}
                                labelStyle={{ color: '#f2f4f7' }}
                                formatter={(value: number) => [`${formatNumber(value)} кг`, 'оценка 1ПМ']}
                              />
                              <Line
                                type="monotone"
                                dataKey="estimated1RMKg"
                                stroke="#33c27f"
                                strokeWidth={2}
                                dot={{ r: 3, fill: '#33c27f' }}
                                activeDot={{ r: 5 }}
                              />
                            </LineChart>
                          </ResponsiveContainer>
                        </div>
                      </>
                    ) : (
                      <p className="mt-3 text-sm text-ink-muted">Нет данных за выбранный период.</p>
                    )}

                    <div className="mt-4">
                      <h3 className="text-xs font-medium uppercase tracking-wide text-ink-faint">Лучшие результаты</h3>
                      <ul className="mt-1 space-y-1 text-sm text-ink-muted">
                        <li>
                          Объём за период:{' '}
                          <span className="text-ink">{formatVolume(exerciseQuery.data.totalVolumeKg)}</span>
                        </li>
                        <li>
                          Рабочих подходов:{' '}
                          <span className="text-ink">
                            {exerciseQuery.data.totalSets}{' '}
                            {plural(exerciseQuery.data.totalSets, 'подход', 'подхода', 'подходов')}
                          </span>
                        </li>
                      </ul>
                    </div>
                  </div>
                )}
              </Card>

              <Card className="p-4">
                <h2 className="section-title mb-3">Упражнения за период</h2>
                {bestsQuery.isPending && <Skeleton className="h-24 w-full" />}
                {bestsQuery.data?.length === 0 && (
                  <p className="text-sm text-ink-muted">Пока нет данных.</p>
                )}
                <ul className="divide-y divide-line/60">
                  {bestsQuery.data?.map((item) => (
                    <li key={item.exerciseId} className="py-2.5">
                      <button
                        type="button"
                        onClick={() => setExerciseId(item.exerciseId)}
                        className="flex w-full items-center justify-between gap-3 text-left"
                      >
                        <span className="min-w-0">
                          <span className="block truncate font-medium">{item.exerciseName}</span>
                          <span className="block text-xs text-ink-faint">
                            {item.completedSetCount}{' '}
                            {plural(item.completedSetCount, 'подход', 'подхода', 'подходов')} ·{' '}
                            {formatVolume(item.volumeKg)}
                          </span>
                        </span>
                        <span className="shrink-0 text-right text-sm">
                          <span className="block tabular">{formatWeight(item.maxWeightKg)} кг</span>
                          <span className="block text-xs text-ink-faint">
                            расч. 1ПМ {formatWeight(item.estimated1RMKg)}
                          </span>
                        </span>
                      </button>
                    </li>
                  ))}
                </ul>
              </Card>
            </>
          )}
        </>
      )}
    </div>
  )
}

const tooltipStyle = {
  background: '#1b1f26',
  border: '1px solid #262c36',
  borderRadius: '0.5rem',
  fontSize: '12px',
  color: '#f2f4f7',
} as const

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-[11px] uppercase tracking-wide text-ink-faint">{label}</dt>
      <dd className="mt-0.5 text-xl font-semibold tabular">{value}</dd>
    </div>
  )
}

void exercisesApi
