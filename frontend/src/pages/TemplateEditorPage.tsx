// Template editor: an ordered plan, not a workout. Planned values here are
// targets that get copied into a session when it starts.

import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { PageHeader } from '../components/AppShell'
import { Button, Card, EmptyState, ErrorBanner, Field, Input, Skeleton } from '../components/ui'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { ExercisePicker } from '../components/ExercisePicker'
import { ApiError, templatesApi, type TemplateInput } from '../lib/api'
import type { Exercise, ExerciseKind } from '../lib/types'
import { plural } from '../lib/format'
import { queryKeys } from '../lib/queryKeys'

interface DraftExercise {
  key: string
  exerciseId: number
  name: string
  kind: ExerciseKind
  plannedSets: string
  plannedReps: string
  plannedWeightKg: string
  plannedMinutes: string
}

export function TemplateEditorPage() {
  const { id } = useParams()
  const isNew = id === undefined
  const templateId = Number(id)
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [exercises, setExercises] = useState<DraftExercise[]>([])
  const [error, setError] = useState<string | null>(null)
  const [details, setDetails] = useState<Record<string, string>>({})
  const [pickerOpen, setPickerOpen] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [loaded, setLoaded] = useState(isNew)

  const templateQuery = useQuery({
    queryKey: queryKeys.template(templateId),
    queryFn: () => templatesApi.get(templateId),
    enabled: !isNew && Number.isFinite(templateId),
  })

  useEffect(() => {
    const template = templateQuery.data
    if (!template || loaded) return
    setName(template.name)
    setDescription(template.description)
    setExercises(
      template.exercises.map((exercise) => ({
        key: String(exercise.id),
        exerciseId: exercise.exerciseId,
        name: exercise.exerciseName,
        kind: (exercise as { kind?: ExerciseKind }).kind ?? 'weighted',
        plannedMinutes:
          exercise.plannedDurationSeconds === null ? '' : String(exercise.plannedDurationSeconds / 60),
        plannedSets: exercise.plannedSets === null ? '' : String(exercise.plannedSets),
        plannedReps: exercise.plannedReps === null ? '' : String(exercise.plannedReps),
        plannedWeightKg: exercise.plannedWeightKg === null ? '' : String(exercise.plannedWeightKg),
      })),
    )
    setLoaded(true)
  }, [templateQuery.data, loaded])

  const save = useMutation({
    mutationFn: (body: TemplateInput) =>
      isNew ? templatesApi.create(body) : templatesApi.update(templateId, body),
    onSuccess: (template) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.templates })
      void queryClient.invalidateQueries({ queryKey: queryKeys.template(template.id) })
      navigate('/')
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        setError(err.message)
        setDetails(err.details)
      } else {
        setError('Не удалось сохранить шаблон')
      }
    },
  })

  const remove = useMutation({
    mutationFn: () => templatesApi.remove(templateId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.templates })
      navigate('/')
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : 'Не удалось удалить шаблон'),
  })

  function submit() {
    setError(null)
    setDetails({})
    if (!name.trim()) {
      setDetails({ name: 'Введите название шаблона' })
      return
    }
    if (exercises.length === 0) {
      setDetails({ exercises: 'Добавьте хотя бы одно упражнение' })
      return
    }
    save.mutate({
      name: name.trim(),
      description: description.trim(),
      exercises: exercises.map((exercise) => {
        const timed = exercise.kind === 'duration'
        const minutes = parseOptionalFloat(exercise.plannedMinutes)
        return {
          exerciseId: exercise.exerciseId,
          plannedSets: timed ? null : parseOptionalInt(exercise.plannedSets),
          plannedReps: timed ? null : parseOptionalInt(exercise.plannedReps),
          plannedWeightKg: timed ? null : parseOptionalFloat(exercise.plannedWeightKg),
          plannedDurationSeconds: minutes === null ? null : Math.round(minutes * 60),
        }
      }),
    })
  }

  function addExercise(exerciseId: number) {
    // The picker returns only the id, so read the cached catalogue for the name.
    const cached = queryClient.getQueryData<Exercise[]>(
      queryKeys.exercises({ includeArchived: false }),
    )
    const picked = cached?.find((item) => item.id === exerciseId)
    setExercises((prev) => [
      ...prev,
      {
        key: `${exerciseId}-${Date.now()}`,
        exerciseId,
        name: picked?.name ?? 'Упражнение',
        kind: picked?.kind ?? 'weighted',
        plannedSets: '3',
        plannedReps: '8',
        plannedWeightKg: '',
        plannedMinutes: '30',
      },
    ])
    setPickerOpen(false)
  }

  function move(index: number, direction: -1 | 1) {
    setExercises((prev) => {
      const next = [...prev]
      const target = index + direction
      if (target < 0 || target >= next.length) return prev
      const a = next[index]
      const b = next[target]
      if (!a || !b) return prev
      next[index] = b
      next[target] = a
      return next
    })
  }

  if (!isNew && templateQuery.isPending) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-8 w-40" />
        <Skeleton className="h-24 w-full" />
      </div>
    )
  }

  if (!isNew && templateQuery.isError) {
    return (
      <ErrorBanner
        message="Шаблон не найден"
        onRetry={() => navigate('/')}
      />
    )
  }

  return (
    <div>
      <PageHeader
        title={isNew ? 'Новый шаблон' : 'Шаблон'}
        back={{ to: '/' }}
        actions={
          !isNew && (
            <button
              type="button"
              onClick={() => setConfirmDelete(true)}
              className="grid h-10 w-10 place-items-center rounded-lg text-ink-faint hover:bg-danger/10 hover:text-danger"
              aria-label="Удалить шаблон"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-5 w-5">
                <path d="M6 7h12M10 7V5h4v2M8 7l1 12h6l1-12" strokeLinecap="round" strokeLinejoin="round" />
              </svg>
            </button>
          )
        }
      />

      {error && (
        <div className="mb-3">
          <ErrorBanner message={error} details={details} />
        </div>
      )}

      <div className="space-y-3">
        <Field label="Название" error={details.name}>
          <Input
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Например, Грудь"
            maxLength={120}
          />
        </Field>
        <Field label="Описание" hint="Необязательно">
          <Input
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            placeholder="Короткая заметка о шаблоне"
          />
        </Field>
      </div>

      <section className="mt-6">
        <div className="mb-2 flex items-center justify-between">
          <h2 className="section-title">
            Упражнения{exercises.length > 0 ? ` · ${exercises.length}` : ''}
          </h2>
          <button
            type="button"
            onClick={() => setPickerOpen(true)}
            className="-mr-2 inline-flex min-h-tap items-center rounded-lg px-2 text-sm font-medium text-accent"
          >
            Добавить
          </button>
        </div>

        {details.exercises && <p className="mb-2 text-sm text-danger">{details.exercises}</p>}

        {exercises.length === 0 ? (
          <Card>
            <EmptyState
              title="В шаблоне нет упражнений"
              description="Добавьте движения в нужном порядке и укажите планируемые подходы и повторения."
              action={<Button variant="primary" onClick={() => setPickerOpen(true)}>Добавить упражнение</Button>}
            />
          </Card>
        ) : (
          <ul className="space-y-2">
            {exercises.map((exercise, index) => (
              <li key={exercise.key}>
                <Card className="p-3">
                  <div className="flex items-start gap-2">
                    <div className="min-w-0 flex-1">
                      <p className="truncate font-medium">
                        <span className="mr-1.5 text-ink-faint tabular">{index + 1}.</span>
                        {exercise.name}
                      </p>
                      {exercise.kind === 'duration' ? (
                        <div className="mt-2">
                          <label className="block">
                            <span className="mb-1 block text-[11px] uppercase tracking-wide text-ink-faint">
                              Планируемое время, мин
                            </span>
                            <Input
                              type="text"
                              inputMode="decimal"
                              value={exercise.plannedMinutes}
                              onChange={(event) => update(index, { plannedMinutes: event.target.value })}
                              className="text-center"
                              placeholder="30"
                              aria-label={`Планируемая длительность для ${exercise.name}`}
                            />
                          </label>
                        </div>
                      ) : (
                        <div className="mt-2 grid grid-cols-3 gap-2">
                          <label className="block">
                            <span className="mb-1 block text-[11px] uppercase tracking-wide text-ink-faint">Подходы</span>
                            <Input
                              type="text"
                              inputMode="numeric"
                              value={exercise.plannedSets}
                              onChange={(event) => update(index, { plannedSets: event.target.value })}
                              className="text-center"
                              aria-label={`Планируемые подходы для ${exercise.name}`}
                            />
                          </label>
                          <label className="block">
                            <span className="mb-1 block text-[11px] uppercase tracking-wide text-ink-faint">Повторы</span>
                            <Input
                              type="text"
                              inputMode="numeric"
                              value={exercise.plannedReps}
                              onChange={(event) => update(index, { plannedReps: event.target.value })}
                              className="text-center"
                              aria-label={`Планируемые повторения для ${exercise.name}`}
                            />
                          </label>
                          <label className="block">
                            <span className="mb-1 block text-[11px] uppercase tracking-wide text-ink-faint">
                              {exercise.kind === 'bodyweight' ? 'Вес (не нужен)' : 'Вес, кг'}
                            </span>
                            <Input
                              type="text"
                              inputMode="decimal"
                              value={exercise.plannedWeightKg}
                              onChange={(event) => update(index, { plannedWeightKg: event.target.value })}
                              className="text-center"
                              placeholder="—"
                              disabled={exercise.kind === 'bodyweight'}
                              aria-label={`Планируемый вес для ${exercise.name}`}
                            />
                          </label>
                        </div>
                      )}
                    </div>
                    <div className="flex shrink-0 flex-col">
                      <button
                        type="button"
                        onClick={() => move(index, -1)}
                        disabled={index === 0}
                        aria-label="Выше"
                        className="grid h-8 w-8 place-items-center rounded text-ink-faint hover:text-ink disabled:opacity-30"
                      >
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="h-4 w-4">
                          <path d="M6 15l6-6 6 6" strokeLinecap="round" />
                        </svg>
                      </button>
                      <button
                        type="button"
                        onClick={() => move(index, 1)}
                        disabled={index === exercises.length - 1}
                        aria-label="Ниже"
                        className="grid h-8 w-8 place-items-center rounded text-ink-faint hover:text-ink disabled:opacity-30"
                      >
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="h-4 w-4">
                          <path d="M6 9l6 6 6-6" strokeLinecap="round" />
                        </svg>
                      </button>
                      <button
                        type="button"
                        onClick={() => setExercises((prev) => prev.filter((_, i) => i !== index))}
                        aria-label="Убрать упражнение"
                        className="grid h-8 w-8 place-items-center rounded text-ink-faint hover:text-danger"
                      >
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="h-4 w-4">
                          <path d="M6 6l12 12M18 6L6 18" strokeLinecap="round" />
                        </svg>
                      </button>
                    </div>
                  </div>
                </Card>
              </li>
            ))}
          </ul>
        )}
      </section>

      <p className="mt-4 text-xs text-ink-faint">
        Шаблон — это заготовка. Изменение шаблона не меняет уже завершённые тренировки.
      </p>

      <div className="mt-4 space-y-2">
        <Button variant="primary" block onClick={submit} loading={save.isPending}>
          {isNew ? 'Создать шаблон' : 'Сохранить'}
        </Button>
        <Button variant="ghost" block onClick={() => navigate('/')}>
          Отмена
        </Button>
      </div>

      <ExercisePicker open={pickerOpen} onClose={() => setPickerOpen(false)} onPick={addExercise} />

      <ConfirmDialog
        open={confirmDelete}
        title="Удалить шаблон?"
        description="Завершённые тренировки, начатые по нему, останутся в истории."
        busy={remove.isPending}
        onConfirm={() => remove.mutate()}
        onCancel={() => setConfirmDelete(false)}
      />
    </div>
  )

  function update(index: number, patch: Partial<DraftExercise>) {
    setExercises((prev) => prev.map((item, i) => (i === index ? { ...item, ...patch } : item)))
  }
}

function parseOptionalInt(raw: string): number | null {
  const trimmed = raw.trim()
  if (!trimmed) return null
  const parsed = Number(trimmed)
  return Number.isFinite(parsed) ? Math.round(parsed) : null
}

function parseOptionalFloat(raw: string): number | null {
  const trimmed = raw.trim().replace(',', '.')
  if (!trimmed) return null
  const parsed = Number(trimmed)
  return Number.isFinite(parsed) ? parsed : null
}

void plural
