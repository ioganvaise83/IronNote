// Screen 2 — Активная тренировка. The centre of the product.
//
// Layout rules for one-handed gym use:
//   * every set is editable in place, no modal for weight or reps;
//   * "add set" copies the previous set, so the common case is one tap;
//   * the finish action is sticky at the bottom, above the safe area;
//   * exercise reordering and deletion live behind an explicit menu, not inline,
//     because they are rare and destructive.

import { useCallback, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Button, Card, EmptyState, ErrorBanner, Input, Skeleton } from '../components/ui'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { WorkoutSetRow } from '../components/WorkoutSetRow'
import { ApiError, sessionsApi, type SetInput } from '../lib/api'
import type { SessionExercise, WorkoutSession } from '../lib/types'
import { formatElapsed, formatWeight, formatVolume, plural } from '../lib/format'
import { queryKeys } from '../lib/queryKeys'
import { ExercisePicker } from '../components/ExercisePicker'

export function ActiveWorkoutPage() {
  const { id } = useParams()
  const sessionId = Number(id)
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [actionError, setActionError] = useState<string | null>(null)
  const [pickerOpen, setPickerOpen] = useState(false)
  const [confirmFinish, setConfirmFinish] = useState(false)
  const [removeExercise, setRemoveExercise] = useState<SessionExercise | null>(null)

  const sessionQuery = useQuery({
    queryKey: queryKeys.session(sessionId),
    queryFn: () => sessionsApi.get(sessionId),
    enabled: Number.isFinite(sessionId) && sessionId > 0,
  })

  const session = sessionQuery.data
  const readOnly = session?.status === 'completed'

  const invalidate = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: queryKeys.session(sessionId) })
    void queryClient.invalidateQueries({ queryKey: queryKeys.activeSession })
  }, [queryClient, sessionId])

  const updateSet = useMutation({
    mutationFn: (vars: { exerciseId: number; setId: number; patch: SetInput }) =>
      sessionsApi.updateSet(sessionId, vars.exerciseId, vars.setId, vars.patch),
    onSuccess: invalidate,
    onError: (error) => setActionError(describeError(error)),
  })

  const addSet = useMutation({
    mutationFn: (vars: { exerciseId: number; body: SetInput }) =>
      sessionsApi.addSet(sessionId, vars.exerciseId, vars.body),
    onSuccess: invalidate,
    onError: (error) => setActionError(describeError(error)),
  })

  const deleteSet = useMutation({
    mutationFn: (vars: { exerciseId: number; setId: number }) =>
      sessionsApi.removeSet(sessionId, vars.exerciseId, vars.setId),
    onSuccess: invalidate,
    onError: (error) => setActionError(describeError(error)),
  })

  const addExercise = useMutation({
    mutationFn: (exerciseId: number) => sessionsApi.addExercise(sessionId, exerciseId),
    onSuccess: () => {
      setPickerOpen(false)
      invalidate()
    },
    onError: (error) => setActionError(describeError(error)),
  })

  const moveExercise = useMutation({
    mutationFn: (vars: { exerciseId: number; direction: -1 | 1 }) =>
      sessionsApi.moveExercise(sessionId, vars.exerciseId, vars.direction),
    onSuccess: invalidate,
    onError: (error) => setActionError(describeError(error)),
  })

  const deleteExercise = useMutation({
    mutationFn: (exerciseRowId: number) => sessionsApi.removeExercise(sessionId, exerciseRowId),
    onSuccess: () => {
      setRemoveExercise(null)
      invalidate()
    },
    onError: (error) => setActionError(describeError(error)),
  })

  const finish = useMutation({
    mutationFn: () => sessionsApi.finish(sessionId),
    onSuccess: () => {
      setConfirmFinish(false)
      void queryClient.invalidateQueries({ queryKey: queryKeys.sessions({}) })
      void queryClient.invalidateQueries({ queryKey: queryKeys.statsSummary({}) })
      navigate(`/history/${sessionId}`)
    },
    onError: (error) => setActionError(describeError(error)),
  })

  const updateSession = useMutation({
    mutationFn: (body: { name?: string; notes?: string }) => sessionsApi.update(sessionId, body),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.session(sessionId) })
    },
    onError: (error) => setActionError(describeError(error)),
  })

  const totals = useMemo(() => computeTotals(session), [session])

  if (sessionQuery.isPending) {
    return (
      <div className="space-y-3 pt-2">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-32 w-full" />
        <Skeleton className="h-32 w-full" />
      </div>
    )
  }

  if (sessionQuery.isError || !session) {
    return (
      <div className="pt-6">
        <ErrorBanner
          message={sessionQuery.error instanceof ApiError ? sessionQuery.error.message : 'Тренировка не найдена'}
          onRetry={() => sessionQuery.refetch()}
        />
        <Button className="mt-4" block onClick={() => navigate('/')}>
          К тренировкам
        </Button>
      </div>
    )
  }

  return (
    <div className="pb-28">
      <header className="sticky top-0 z-20 -mx-4 mb-4 border-b border-line bg-canvas/95 px-4 pb-3 pt-2 backdrop-blur safe-top">
        <div className="flex items-start justify-between gap-2">
          <button type="button" onClick={() => navigate('/')} className="-ml-2 grid h-11 w-11 place-items-center rounded-lg text-ink-muted hover:bg-surface-hover hover:text-ink" aria-label="К списку тренировок">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="h-5 w-5">
              <path d="M15 18l-6-6 6-6" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </button>
          <div className="min-w-0 flex-1">
            <SessionName
              name={session.name}
              disabled={updateSession.isPending}
              onCommit={(name) => name && updateSession.mutate({ name })}
            />
            <p className="mt-0.5 flex items-center gap-2 text-xs text-ink-muted">
              <Elapsed startedAt={session.startedAt} />
              <span aria-hidden="true">·</span>
              <span>{totals.setCount} {plural(totals.setCount, 'подход', 'подхода', 'подходов')}</span>
              <span aria-hidden="true">·</span>
              <span className="tabular">{formatVolume(totals.volume)}</span>
              {readOnly && <span className="text-warning">· завершена</span>}
            </p>
          </div>
        </div>
      </header>

      {actionError && (
        <div className="mb-3">
          <ErrorBanner message={actionError} onRetry={() => setActionError(null)} />
        </div>
      )}

      {session.exercises.length === 0 ? (
        <Card>
          <EmptyState
            title="Добавьте первое упражнение"
            description="Начните с любого движения — подходы можно заполнять по ходу."
            action={
              <Button variant="primary" onClick={() => setPickerOpen(true)} disabled={readOnly}>
                Добавить упражнение
              </Button>
            }
          />
        </Card>
      ) : (
        <ul className="space-y-3">
          {session.exercises.map((exercise, index) => (
            <li key={exercise.id}>
              <ExerciseCard
                session={session}
                exercise={exercise}
                index={index}
                readOnly={readOnly}
                busy={addSet.isPending || deleteSet.isPending || updateSet.isPending}
                onSaveSet={(setId, patch) =>
                  updateSet.mutate({ exerciseId: exercise.id, setId, patch })
                }
                onAddSet={(body) => addSet.mutate({ exerciseId: exercise.id, body })}
                onDeleteSet={(setId) => deleteSet.mutate({ exerciseId: exercise.id, setId })}
                onMove={(direction) => moveExercise.mutate({ exerciseId: exercise.id, direction })}
                onRemove={() => setRemoveExercise(exercise)}
                canMoveUp={index > 0}
                canMoveDown={index < session.exercises.length - 1}
              />
            </li>
          ))}
        </ul>
      )}

      {!readOnly && (
        <div className="mt-4 grid grid-cols-2 gap-2">
          <Button variant="secondary" onClick={() => setPickerOpen(true)}>
            + Упражнение
          </Button>
          <Button variant="primary" onClick={() => setConfirmFinish(true)}>
            Завершить
          </Button>
        </div>
      )}

      {readOnly && (
        <div className="mt-4 space-y-2">
          <Button variant="primary" block onClick={() => navigate(`/history/${session.id}`)}>
            Открыть в истории
          </Button>
        </div>
      )}

      {!readOnly && (
        <div className="fixed inset-x-0 bottom-0 z-30 border-t border-line bg-surface/95 px-4 py-3 backdrop-blur lg:hidden" style={{ paddingBottom: 'calc(var(--safe-bottom) + 0.75rem)' }}>
          <div className="mx-auto flex max-w-2xl items-center justify-between gap-3">
            <div className="min-w-0">
              <p className="text-xs text-ink-faint">Объём тренировки</p>
              <p className="truncate text-base font-semibold tabular">{formatVolume(totals.volume)}</p>
            </div>
            <Button variant="primary" onClick={() => setConfirmFinish(true)}>
              Завершить
            </Button>
          </div>
        </div>
      )}

      <ExercisePicker
        open={pickerOpen}
        onClose={() => setPickerOpen(false)}
        onPick={(exerciseId) => addExercise.mutate(exerciseId)}
        busy={addExercise.isPending}
      />

      <ConfirmDialog
        open={confirmFinish}
        title="Завершить тренировку?"
        description={
          totals.setCount === 0
            ? 'В тренировке нет ни одного выполненного подхода. Она сохранится в истории пустой.'
            : `Будет сохранено ${totals.setCount} ${plural(totals.setCount, 'подход', 'подхода', 'подходов')} и объём ${formatVolume(totals.volume)}. Завершённую тренировку можно отредактировать позже.`
        }
        confirmLabel="Завершить"
        destructive={false}
        busy={finish.isPending}
        onConfirm={() => finish.mutate()}
        onCancel={() => setConfirmFinish(false)}
      />

      <ConfirmDialog
        open={removeExercise !== null}
        title="Удалить упражнение из тренировки?"
        description={
          removeExercise && removeExercise.sets.length > 0
            ? `Вместе с ним удалятся ${removeExercise.sets.length} ${plural(removeExercise.sets.length, 'подход', 'подхода', 'подходов')}.`
            : undefined
        }
        busy={deleteExercise.isPending}
        onConfirm={() => removeExercise && deleteExercise.mutate(removeExercise.id)}
        onCancel={() => setRemoveExercise(null)}
      />
    </div>
  )
}

function ExerciseCard({
  session,
  exercise,
  index,
  readOnly,
  busy,
  onSaveSet,
  onAddSet,
  onDeleteSet,
  onMove,
  onRemove,
  canMoveUp,
  canMoveDown,
}: {
  session: WorkoutSession
  exercise: SessionExercise
  index: number
  readOnly: boolean
  busy: boolean
  onSaveSet: (setId: number, patch: SetInput) => void
  onAddSet: (body: SetInput) => void
  onDeleteSet: (setId: number) => void
  onMove: (direction: -1 | 1) => void
  onRemove: () => void
  canMoveUp: boolean
  canMoveDown: boolean
}) {
  const [menuOpen, setMenuOpen] = useState(false)
  const [notesOpen, setNotesOpen] = useState(false)
  const [notes, setNotes] = useState(exercise.notes)
  const last = exercise.sets[exercise.sets.length - 1]

  // "Add set" copies the last set (weight + reps) so the usual case is one tap.
  const nextBody = (): SetInput => ({ repetitions: 10, copyPrevious: true })

  return (
    <Card className="overflow-hidden">
      <div className="flex items-start gap-2 px-3 pb-2 pt-3">
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-base font-semibold">
            <span className="mr-1.5 text-ink-faint tabular">{index + 1}.</span>
            {exercise.exerciseName}
          </h2>
          <p className="mt-0.5 text-xs text-ink-faint">
            {exercise.sets.length} {plural(exercise.sets.length, 'подход', 'подхода', 'подходов')}
            {exercise.previousSets.length > 0 && (
              <>
                {' · в прошлый раз: '}
                {exercise.previousSets
                  .map((s) => `${formatWeight(s.weightKg)}×${s.repetitions}`)
                  .join(', ')}
              </>
            )}
          </p>
        </div>
        {!readOnly && (
          <div className="relative shrink-0">
            <button
              type="button"
              onClick={() => setMenuOpen((v) => !v)}
              aria-label={`Действия с упражнением ${exercise.exerciseName}`}
              aria-expanded={menuOpen}
              className="grid h-11 w-11 place-items-center rounded-lg text-ink-faint hover:bg-surface-hover hover:text-ink"
            >
              <svg viewBox="0 0 24 24" fill="currentColor" className="h-4 w-4">
                <circle cx="12" cy="5" r="1.8" />
                <circle cx="12" cy="12" r="1.8" />
                <circle cx="12" cy="19" r="1.8" />
              </svg>
            </button>
            {menuOpen && (
              <>
                <div className="fixed inset-0 z-10" onClick={() => setMenuOpen(false)} aria-hidden="true" />
                <div className="absolute right-0 top-10 z-20 w-52 overflow-hidden rounded-xl border border-line bg-surface-raised py-1 shadow-xl">
                  <MenuItem
                    disabled={!canMoveUp}
                    onClick={() => {
                      onMove(-1)
                      setMenuOpen(false)
                    }}
                  >
                    Переместить выше
                  </MenuItem>
                  <MenuItem
                    disabled={!canMoveDown}
                    onClick={() => {
                      onMove(1)
                      setMenuOpen(false)
                    }}
                  >
                    Переместить ниже
                  </MenuItem>
                  <MenuItem
                    onClick={() => {
                      setNotesOpen(true)
                      setMenuOpen(false)
                    }}
                  >
                    {exercise.notes ? 'Изменить заметку' : 'Добавить заметку'}
                  </MenuItem>
                  <MenuItem
                    danger
                    onClick={() => {
                      onRemove()
                      setMenuOpen(false)
                    }}
                  >
                    Удалить упражнение
                  </MenuItem>
                </div>
              </>
            )}
          </div>
        )}
      </div>

      {exercise.notes && !notesOpen && (
        <p className="mx-3 mb-2 rounded-lg bg-surface-raised px-2.5 py-1.5 text-sm text-ink-muted">
          {exercise.notes}
        </p>
      )}

      <div className="mx-1 flex items-center gap-1.5 px-2 pb-1 text-[10px] uppercase tracking-wide text-ink-faint">
        <span className="w-11 text-center">Подх.</span>
        <span className="flex-1 text-center">Вес</span>
        <span className="flex-1 text-center">Повторы</span>
        <span className="hidden w-[5rem] text-center sm:block">Объём</span>
        <span className="w-[2.75rem]" />
        <span className="w-[2.75rem]" />
      </div>

      <div className="divide-y divide-line/60">
        {exercise.sets.map((set) => (
          <WorkoutSetRow
            key={set.id}
            set={set}
            readOnly={readOnly}
            saving={busy}
            onSave={(patch) => onSaveSet(set.id, patch)}
            onDelete={() => onDeleteSet(set.id)}
            onCopy={() => onAddSet({ repetitions: set.repetitions, weightKg: set.weightKg })}
          />
        ))}
      </div>

      {!readOnly && (
        <div className="flex gap-2 border-t border-line/60 p-2">
          <Button
            variant="secondary"
            block
            onClick={() => onAddSet(nextBody())}
            disabled={busy}
            className="text-sm"
          >
            {last ? `+ Подход ${formatWeight(last.weightKg)}×${last.repetitions}` : '+ Подход'}
          </Button>
        </div>
      )}

      {notesOpen && (
        <div className="border-t border-line p-2">
          <Input
            value={notes}
            onChange={(event) => setNotes(event.target.value)}
            placeholder="Заметка к упражнению"
            autoFocus
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                event.preventDefault()
                void sessionsApi.updateExerciseNotes(session.id, exercise.id, notes)
                setNotesOpen(false)
              }
            }}
            onBlur={() => {
              void sessionsApi.updateExerciseNotes(session.id, exercise.id, notes)
              setNotesOpen(false)
            }}
          />
        </div>
      )}

      {readOnly && <div className="h-2" />}
    </Card>
  )
}

function MenuItem({
  children,
  onClick,
  disabled,
  danger,
}: {
  children: React.ReactNode
  onClick: () => void
  disabled?: boolean
  danger?: boolean
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={`block w-full px-3 py-2.5 text-left text-sm transition disabled:opacity-40 ${
        danger ? 'text-danger hover:bg-danger/10' : 'text-ink hover:bg-surface-hover'
      }`}
    >
      {children}
    </button>
  )
}

function SessionName({ name, onCommit, disabled }: { name: string; onCommit: (name: string) => void; disabled: boolean }) {
  const [value, setValue] = useState(name)
  const [editing, setEditing] = useState(false)

  if (!editing) {
    return (
      <button
        type="button"
        onClick={() => setEditing(true)}
        className="-ml-1 block max-w-full truncate rounded-lg px-1 py-1 text-left text-xl font-semibold tracking-tight hover:bg-surface-hover"
      >
        {name}
      </button>
    )
  }
  return (
    <input
      value={value}
      onChange={(event) => setValue(event.target.value)}
      autoFocus
      disabled={disabled}
      onBlur={() => {
        setEditing(false)
        if (value.trim() && value !== name) onCommit(value.trim())
      }}
      onKeyDown={(event) => {
        if (event.key === 'Enter') {
          event.currentTarget.blur()
        }
        if (event.key === 'Escape') {
          setValue(name)
          setEditing(false)
        }
      }}
      className="w-full rounded-lg border border-line bg-canvas px-2 py-1 text-xl font-semibold"
      aria-label="Название тренировки"
    />
  )
}

function Elapsed({ startedAt }: { startedAt: string }) {
  const [now, setNow] = useState(() => new Date())
  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), 1000)
    return () => window.clearInterval(timer)
  }, [])
  return <span className="tabular">{formatElapsed(startedAt, now)}</span>
}

function computeTotals(session: WorkoutSession | undefined): { volume: number; setCount: number } {
  if (!session) return { volume: 0, setCount: 0 }
  let volume = 0
  let setCount = 0
  for (const exercise of session.exercises) {
    for (const set of exercise.sets) {
      if (!set.isCompleted || set.isWarmup) continue
      setCount += 1
      if (set.weightKg !== null) volume += set.weightKg * set.repetitions
    }
  }
  return { volume, setCount }
}

function describeError(error: unknown): string {
  if (error instanceof ApiError) return error.message
  return 'Не удалось сохранить изменение'
}

import { useEffect } from 'react'
