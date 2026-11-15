// Create/edit an exercise. Muscle groups are a multi-select with one optional
// primary — the schema allows one primary per exercise, so the UI does too.

import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { PageHeader } from '../components/AppShell'
import { Button, Card, ErrorBanner, Field, Input, Skeleton } from '../components/ui'
import { ApiError, exercisesApi, muscleGroupsApi, type ExerciseInput } from '../lib/api'
import { queryKeys } from '../lib/queryKeys'

export function ExerciseFormPage() {
  const { id } = useParams()
  const isNew = id === undefined
  const exerciseId = Number(id)
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [equipment, setEquipment] = useState('')
  const [selected, setSelected] = useState<number[]>([])
  const [primary, setPrimary] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [details, setDetails] = useState<Record<string, string>>({})
  const [loaded, setLoaded] = useState(isNew)

  const groupsQuery = useQuery({ queryKey: queryKeys.muscleGroups, queryFn: muscleGroupsApi.list })
  const exerciseQuery = useQuery({
    queryKey: queryKeys.exercise(exerciseId),
    queryFn: () => exercisesApi.get(exerciseId),
    enabled: !isNew && Number.isFinite(exerciseId),
  })

  useEffect(() => {
    const exercise = exerciseQuery.data
    if (!exercise || loaded) return
    setName(exercise.name)
    setDescription(exercise.description)
    setEquipment(exercise.equipment)
    setSelected(exercise.muscleGroups.map((group) => group.muscleGroupId))
    setPrimary(exercise.muscleGroups.find((group) => group.isPrimary)?.muscleGroupId ?? null)
    setLoaded(true)
  }, [exerciseQuery.data, loaded])

  const save = useMutation({
    mutationFn: (body: ExerciseInput) =>
      isNew ? exercisesApi.create(body) : exercisesApi.update(exerciseId, body),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['exercises'] })
      navigate('/exercises')
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        setError(err.message)
        setDetails(err.details)
      } else {
        setError('Не удалось сохранить упражнение')
      }
    },
  })

  function toggle(groupId: number) {
    setSelected((prev) => {
      if (prev.includes(groupId)) {
        if (primary === groupId) setPrimary(null)
        return prev.filter((value) => value !== groupId)
      }
      return [...prev, groupId]
    })
  }

  function submit() {
    setError(null)
    setDetails({})
    if (!name.trim()) {
      setDetails({ name: 'Введите название упражнения' })
      return
    }
    save.mutate({
      name: name.trim(),
      description: description.trim(),
      equipment: equipment.trim(),
      muscleGroups: selected.map((muscleGroupId) => ({
        muscleGroupId,
        isPrimary: muscleGroupId === primary,
      })),
    })
  }

  if (!isNew && exerciseQuery.isPending) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-8 w-40" />
        <Skeleton className="h-20 w-full" />
        <Skeleton className="h-40 w-full" />
      </div>
    )
  }

  return (
    <div>
      <PageHeader
        title={isNew ? 'Новое упражнение' : 'Упражнение'}
        back={{ to: '/exercises', label: 'Упражнения' }}
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
            placeholder="Например, Жим штанги лёжа"
            maxLength={120}
          />
        </Field>

        <Field label="Описание" hint="Необязательно. Техника, на что обращать внимание.">
          <textarea
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            rows={3}
            placeholder="Короткое описание"
            className="field resize-none py-2"
          />
        </Field>

        <Field label="Снаряжение" hint="Необязательно: штанга, гантели, тренажёр…">
          <Input
            value={equipment}
            onChange={(event) => setEquipment(event.target.value)}
            placeholder="Штанга"
            maxLength={120}
          />
        </Field>
      </div>

      <section className="mt-5">
        <h2 className="section-title mb-2">Группы мышц</h2>
        {details.muscleGroups && <p className="mb-2 text-sm text-danger">{details.muscleGroups}</p>}
        <Card className="p-3">
          {groupsQuery.isPending && <Skeleton className="h-8 w-full" />}
          {groupsQuery.data && groupsQuery.data.length === 0 && (
            <p className="text-sm text-ink-muted">
              Справочник групп мышц пуст. Добавьте их в настройках.
            </p>
          )}
          <div className="flex flex-wrap gap-2">
            {groupsQuery.data?.map((group) => {
              const isSelected = selected.includes(group.id)
              return (
                <button
                  key={group.id}
                  type="button"
                  onClick={() => toggle(group.id)}
                  aria-pressed={isSelected}
                  className={`chip ${isSelected ? 'chip-active' : ''}`}
                >
                  {group.name}
                  {primary === group.id && <span className="text-[10px] uppercase text-accent">осн.</span>}
                </button>
              )
            })}
          </div>

          {selected.length > 0 && (
            <label className="mt-3 block">
              <span className="field-label">Основная группа мышц</span>
              <select
                value={primary ?? ''}
                onChange={(event) => setPrimary(event.target.value ? Number(event.target.value) : null)}
                className="field"
              >
                <option value="">Не указана</option>
                {selected.map((groupId) => {
                  const group = groupsQuery.data?.find((item) => item.id === groupId)
                  return (
                    <option key={groupId} value={groupId}>
                      {group?.name ?? groupId}
                    </option>
                  )
                })}
              </select>
            </label>
          )}
        </Card>
      </section>

      <div className="mt-5 space-y-2">
        <Button variant="primary" block onClick={submit} loading={save.isPending}>
          {isNew ? 'Создать упражнение' : 'Сохранить'}
        </Button>
        <Button variant="ghost" block onClick={() => navigate('/exercises')}>
          Отмена
        </Button>
      </div>
    </div>
  )
}
