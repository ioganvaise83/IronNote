// Detail view of one finished workout, plus editing of a completed session.
// Editing reuses the same endpoints as the active screen; the table is read-only
// here and the user can reopen the workout to change sets.

import { useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { PageHeader } from '../components/AppShell'
import { Button, Card, ErrorBanner, Input, Skeleton } from '../components/ui'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { ApiError, sessionsApi } from '../lib/api'
import {
  formatDateTime,
  formatDistance,
  formatDuration,
  formatSetDuration,
  formatVolume,
  formatWeight,
  pace,
  plural,
} from '../lib/format'
import { queryKeys } from '../lib/queryKeys'

export function SessionDetailPage() {
  const { id } = useParams()
  const sessionId = Number(id)
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [error, setError] = useState<string | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [notes, setNotes] = useState<string | null>(null)
  const [editTime, setEditTime] = useState(false)
  const [startValue, setStartValue] = useState('')
  const [durationValue, setDurationValue] = useState('')

  const sessionQuery = useQuery({
    queryKey: queryKeys.session(sessionId),
    queryFn: () => sessionsApi.get(sessionId),
    enabled: Number.isFinite(sessionId) && sessionId > 0,
  })

  const session = sessionQuery.data

  const reopen = useMutation({
    mutationFn: () => sessionsApi.reopen(sessionId),
    onSuccess: (updated) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.session(sessionId) })
      void queryClient.invalidateQueries({ queryKey: queryKeys.sessions({}) })
      void queryClient.invalidateQueries({ queryKey: queryKeys.activeSession })
      navigate(`/workouts/${updated.id}`)
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : 'Не удалось открыть тренировку'),
  })

  const saveNotes = useMutation({
    mutationFn: (value: string) => sessionsApi.update(sessionId, { notes: value }),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: queryKeys.session(sessionId) }),
    onError: (err) => setError(err instanceof ApiError ? err.message : 'Не удалось сохранить заметку'),
  })

  const saveTime = useMutation({
    mutationFn: (body: { startedAt?: string; durationMinutes?: number }) =>
      sessionsApi.update(sessionId, body),
    onSuccess: () => {
      setEditTime(false)
      void queryClient.invalidateQueries({ queryKey: queryKeys.session(sessionId) })
      void queryClient.invalidateQueries({ queryKey: queryKeys.sessions({}) })
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : 'Не удалось сохранить время'),
  })

  const remove = useMutation({
    mutationFn: () => sessionsApi.remove(sessionId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.sessions({}) })
      void queryClient.invalidateQueries({ queryKey: queryKeys.statsSummary({}) })
      navigate('/history')
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : 'Не удалось удалить тренировку'),
  })

  if (sessionQuery.isPending) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-40 w-full" />
      </div>
    )
  }

  if (sessionQuery.isError || !session) {
    return (
      <div>
        <PageHeader title="Тренировка" back={{ to: '/history' }} />
        <ErrorBanner message="Тренировка не найдена" onRetry={() => navigate('/history')} />
      </div>
    )
  }

  const noteValue = notes ?? session.notes

  return (
    <div>
      <PageHeader
        title={session.name}
        subtitle={formatDateTime(session.startedAt)}
        back={{ to: '/history', label: 'История' }}
        actions={
          <button
            type="button"
            onClick={() => setConfirmDelete(true)}
            className="grid h-10 w-10 place-items-center rounded-lg text-ink-faint hover:bg-danger/10 hover:text-danger"
            aria-label="Удалить тренировку"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-5 w-5">
              <path d="M6 7h12M10 7V5h4v2M8 7l1 12h6l1-12" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </button>
        }
      />

      {error && (
        <div className="mb-3">
          <ErrorBanner message={error} onRetry={() => setError(null)} />
        </div>
      )}

      <Card className="mb-4 p-4">
        <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Metric label="Длительность" value={formatDuration(session.durationMinutes)} />
          <Metric label="Упражнения" value={String(session.exerciseCount)} />
          <Metric label="Подходы" value={String(session.completedSetCount)} />
          <Metric label="Объём" value={formatVolume(session.volumeKg)} />
        </dl>
        {(session.durationSeconds > 0 || session.distanceMeters > 0) && (
          <dl className="mt-3 grid grid-cols-2 gap-3 border-t border-line pt-3 sm:grid-cols-4">
            <Metric label="Время кардио" value={formatSetDuration(session.durationSeconds)} />
            <Metric label="Дистанция" value={formatDistance(session.distanceMeters)} />
          </dl>
        )}
        {session.finishedAt && (
          <p className="mt-3 text-xs text-ink-faint">
            Завершена в {formatDateTime(session.finishedAt)}
            {session.templateId !== null && ' · по шаблону'}
          </p>
        )}
      </Card>

      <ul className="space-y-3">
        {session.exercises.map((exercise, index) => (
          <li key={exercise.id}>
            <Card className="overflow-hidden">
              <div className="px-3 pb-1 pt-3">
                <h2 className="truncate font-semibold">
                  <span className="mr-1.5 text-ink-faint tabular">{index + 1}.</span>
                  {exercise.exerciseName}
                </h2>
                {exercise.notes && <p className="mt-0.5 text-sm text-ink-muted">{exercise.notes}</p>}
              </div>
              {exercise.kind === 'duration' ? (
                <table className="w-full text-sm">
                  <thead>
                    <tr className="text-[11px] uppercase tracking-wide text-ink-faint">
                      <th className="w-12 py-1.5 text-center font-normal">Подх.</th>
                      <th className="py-1.5 text-left font-normal">Время</th>
                      <th className="py-1.5 text-left font-normal">Дистанция</th>
                      <th className="py-1.5 pr-3 text-right font-normal">Темп</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-line/60">
                    {exercise.sets.map((set) => (
                      <tr key={set.id} className={set.isWarmup ? 'text-ink-faint' : ''}>
                        <td className="py-2 text-center tabular">
                          {set.setNumber}
                          {set.isWarmup && <span className="ml-1 text-[10px] uppercase">р</span>}
                        </td>
                        <td className="py-2 tabular">{formatSetDuration(set.durationSeconds)}</td>
                        <td className="py-2 tabular">
                          {set.distanceMeters === null ? '—' : formatDistance(set.distanceMeters)}
                        </td>
                        <td className="py-2 pr-3 text-right tabular text-ink-faint">
                          {pace(set.durationSeconds, set.distanceMeters)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              ) : (
                <table className="w-full text-sm">
                  <thead>
                    <tr className="text-[11px] uppercase tracking-wide text-ink-faint">
                      <th className="w-12 py-1.5 text-center font-normal">Подх.</th>
                      <th className="py-1.5 text-left font-normal">
                        {exercise.kind === 'bodyweight' ? 'Свой вес' : 'Вес'}
                      </th>
                      <th className="py-1.5 text-left font-normal">Повторы</th>
                      <th className="py-1.5 pr-3 text-right font-normal">Объём</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-line/60">
                    {exercise.sets.map((set) => (
                      <tr key={set.id} className={set.isWarmup ? 'text-ink-faint' : ''}>
                        <td className="py-2 text-center tabular">
                          {set.setNumber}
                          {set.isWarmup && <span className="ml-1 text-[10px] uppercase">р</span>}
                        </td>
                        <td className="py-2 tabular">
                          {exercise.kind === 'bodyweight'
                            ? '—'
                            : set.weightKg === null
                              ? '—'
                              : `${formatWeight(set.weightKg)} кг`}
                        </td>
                        <td className="py-2 tabular">{set.repetitions}</td>
                        <td className="py-2 pr-3 text-right tabular">
                          {set.weightKg !== null && !set.isWarmup
                            ? formatWeight(set.weightKg * set.repetitions)
                            : '—'}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
              {exercise.sets.length === 0 && (
                <p className="px-3 pb-3 text-sm text-ink-faint">Подходы не записаны</p>
              )}
            </Card>
          </li>
        ))}
      </ul>

      <Card className="mt-4 p-4">
        <label className="block">
          <span className="field-label">Заметка к тренировке</span>
          <textarea
            value={noteValue}
            onChange={(event) => setNotes(event.target.value)}
            onBlur={() => {
              if (notes !== null && notes !== session.notes) saveNotes.mutate(notes)
            }}
            rows={3}
            placeholder="Как прошло, что получилось, что изменить"
            className="field resize-none py-2"
          />
        </label>
      </Card>

      {editTime ? (
        <Card className="mt-4 space-y-3 p-4">
          <h2 className="font-medium">Дата и продолжительность</h2>
          <label className="block">
            <span className="field-label">Начало</span>
            <Input
              type="datetime-local"
              value={startValue}
              onChange={(event) => setStartValue(event.target.value)}
            />
          </label>
          <label className="block">
            <span className="field-label">Продолжительность, минут</span>
            <Input
              type="text"
              inputMode="numeric"
              value={durationValue}
              onChange={(event) => setDurationValue(event.target.value)}
              placeholder="60"
            />
          </label>
          <div className="flex gap-2">
            <Button variant="ghost" block onClick={() => setEditTime(false)}>
              Отмена
            </Button>
            <Button
              variant="primary"
              block
              loading={saveTime.isPending}
              onClick={() => {
                const body: { startedAt?: string; durationMinutes?: number } = {}
                if (startValue) {
                  // datetime-local has no zone: the browser's local time is what the
                  // user means, so send it as an absolute instant.
                  body.startedAt = new Date(startValue).toISOString()
                }
                const mins = Number(durationValue.trim().replace(',', '.'))
                if (Number.isFinite(mins) && mins > 0) body.durationMinutes = mins
                saveTime.mutate(body)
              }}
            >
              Сохранить
            </Button>
          </div>
        </Card>
      ) : (
        <div className="mt-4 space-y-2">
          <Button
            variant="secondary"
            block
            onClick={() => {
              setStartValue(toDateTimeLocal(session.startedAt))
              // Round up to a whole minute: a workout ended within the same minute
              // has a duration of 0.1 min, and showing "0" would be unsaveable.
              setDurationValue(
                session.durationMinutes === null
                  ? ''
                  : String(Math.max(1, Math.round(session.durationMinutes))),
              )
              setEditTime(true)
            }}
          >
            Изменить дату и продолжительность
          </Button>
          <Button variant="secondary" block onClick={() => reopen.mutate()} loading={reopen.isPending}>
            Редактировать подходы
          </Button>
          <Button variant="danger" block onClick={() => setConfirmDelete(true)}>
            Удалить тренировку
          </Button>
          <p className="text-center text-xs text-ink-faint">
            «Редактировать подходы» откроет экран тренировки: можно изменить упражнения и подходы, затем
            завершить снова.
          </p>
        </div>
      )}

      <ConfirmDialog
        open={confirmDelete}
        title="Удалить тренировку?"
        description="Она исчезнет из истории и статистики. Это действие нельзя отменить."
        busy={remove.isPending}
        onConfirm={() => remove.mutate()}
        onCancel={() => setConfirmDelete(false)}
      />
    </div>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-[11px] uppercase tracking-wide text-ink-faint">{label}</dt>
      <dd className="mt-0.5 text-base font-semibold tabular">{value}</dd>
    </div>
  )
}

void plural

/** ISO instant → the value a datetime-local input expects, in local time. */
function toDateTimeLocal(iso: string): string {
  const d = new Date(iso)
  const pad = (n: number) => n.toString().padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}
