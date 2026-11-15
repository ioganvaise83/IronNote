// One row of the set table.
//
// This is the hottest interaction in the app: a user standing at a rack, one hand
// free. Design choices that follow from that:
//   * inputs are always visible, never behind a modal;
//   * each field saves on blur or Enter, so there is no "save" button to find;
//   * the right keypad appears for each measure (decimal for weight, numeric for
//     reps, time in minutes for cardio).
//
// The row adapts to the exercise kind: a timed movement shows minutes and an
// optional distance instead of weight and repetitions.

import { useEffect, useRef, useState, forwardRef } from 'react'
import type { ExerciseKind, WorkoutSet } from '../lib/types'
import { formatWeight } from '../lib/format'

interface Props {
  set: WorkoutSet
  kind?: ExerciseKind
  readOnly?: boolean
  saving?: boolean
  onSave: (patch: {
    weightKg?: number | null
    repetitions?: number
    durationSeconds?: number | null
    distanceMeters?: number | null
    isCompleted?: boolean
    isWarmup?: boolean
  }) => void
  onDelete: () => void
  onCopy: () => void
}

/** Seconds → the "mm:ss" style value shown in the duration input. */
function secondsToMinutes(seconds: number | null): string {
  if (seconds === null || seconds <= 0) return ''
  const mins = seconds / 60
  return Number.isInteger(mins) ? String(mins) : mins.toFixed(1)
}

export function WorkoutSetRow({ set, kind = 'weighted', readOnly = false, saving = false, onSave, onDelete, onCopy }: Props) {
  const timed = kind === 'duration'
  const loaded = kind === 'weighted'

  const [weight, setWeight] = useState(set.weightKg === null ? '' : String(set.weightKg))
  const [reps, setReps] = useState(String(set.repetitions || ''))
  const [minutes, setMinutes] = useState(secondsToMinutes(set.durationSeconds))
  const [distance, setDistance] = useState(set.distanceMeters === null ? '' : String(set.distanceMeters))
  const weightRef = useRef<HTMLInputElement>(null)
  const repsRef = useRef<HTMLInputElement>(null)
  const minutesRef = useRef<HTMLInputElement>(null)
  const distanceRef = useRef<HTMLInputElement>(null)

  // Server value wins when it changes underneath us (copy, undo, refetch).
  useEffect(() => setWeight(set.weightKg === null ? '' : String(set.weightKg)), [set.weightKg])
  useEffect(() => setReps(set.repetitions ? String(set.repetitions) : ''), [set.repetitions])
  useEffect(() => setMinutes(secondsToMinutes(set.durationSeconds)), [set.durationSeconds])
  useEffect(() => setDistance(set.distanceMeters === null ? '' : String(set.distanceMeters)), [set.distanceMeters])

  function commitWeight() {
    if (readOnly) return
    const trimmed = weight.trim().replace(',', '.')
    if (trimmed === '') {
      if (set.weightKg !== null) onSave({ weightKg: null })
      return
    }
    const parsed = Number(trimmed)
    if (!Number.isFinite(parsed) || parsed < 0) {
      setWeight(set.weightKg === null ? '' : String(set.weightKg))
      return
    }
    if (parsed !== set.weightKg) onSave({ weightKg: parsed })
  }

  function commitReps() {
    if (readOnly) return
    const parsed = Number(reps.trim())
    if (!Number.isFinite(parsed) || parsed < 1) {
      setReps(set.repetitions ? String(set.repetitions) : '')
      return
    }
    const rounded = Math.round(parsed)
    if (rounded !== set.repetitions) onSave({ repetitions: rounded })
  }

  function commitMinutes() {
    if (readOnly) return
    const trimmed = minutes.trim().replace(',', '.')
    if (trimmed === '') {
      // A timed set cannot be empty; restore rather than send an invalid value.
      setMinutes(secondsToMinutes(set.durationSeconds))
      return
    }
    const parsed = Number(trimmed)
    if (!Number.isFinite(parsed) || parsed <= 0) {
      setMinutes(secondsToMinutes(set.durationSeconds))
      return
    }
    const seconds = Math.round(parsed * 60)
    if (seconds !== set.durationSeconds) onSave({ durationSeconds: seconds })
  }

  function commitDistance() {
    if (readOnly) return
    const trimmed = distance.trim().replace(',', '.')
    if (trimmed === '') {
      if (set.distanceMeters !== null) onSave({ distanceMeters: null })
      return
    }
    const parsed = Number(trimmed)
    if (!Number.isFinite(parsed) || parsed < 0) {
      setDistance(set.distanceMeters === null ? '' : String(set.distanceMeters))
      return
    }
    if (parsed !== set.distanceMeters) onSave({ distanceMeters: parsed })
  }

  const rowTone = set.isWarmup ? 'bg-warning/5' : set.isCompleted ? 'bg-surface' : 'bg-surface-raised'

  return (
    <div
      className={`grid items-center gap-1.5 px-2 py-1.5 ${rowTone} ${
        timed
          ? 'grid-cols-[2.75rem_1fr_1fr_2.75rem_2.75rem]'
          : 'grid-cols-[2.75rem_1fr_1fr_2.75rem_2.75rem] sm:grid-cols-[2.75rem_1fr_1fr_5rem_2.75rem_2.75rem]'
      }`}
    >
      <div className="flex justify-center">
        <button
          type="button"
          onClick={() => !readOnly && onSave({ isCompleted: !set.isCompleted })}
          disabled={readOnly}
          aria-label={set.isCompleted ? `Подход ${set.setNumber} выполнен` : `Отметить подход ${set.setNumber} выполненным`}
          aria-pressed={set.isCompleted}
          className={`grid h-11 w-11 place-items-center rounded-full border transition ${
            set.isCompleted ? 'border-success bg-success/20 text-success' : 'border-line text-ink-faint hover:border-ink-faint'
          }`}
        >
          {set.isCompleted ? (
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" className="h-4 w-4" aria-hidden="true">
              <path d="M5 13l4 4L19 7" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          ) : (
            <span className="text-xs font-semibold tabular">{set.setNumber}</span>
          )}
        </button>
      </div>

      {timed ? (
        <>
          <NumberCell
            ref={minutesRef}
            value={minutes}
            onChange={setMinutes}
            onCommit={commitMinutes}
            onEnter={() => distanceRef.current?.focus()}
            readOnly={readOnly}
            placeholder="—"
            ariaLabel={`Длительность подхода ${set.setNumber}, минуты`}
            suffix="мин"
            disabled={saving}
          />
          <NumberCell
            ref={distanceRef}
            value={distance}
            onChange={setDistance}
            onCommit={commitDistance}
            onEnter={commitDistance}
            readOnly={readOnly}
            placeholder="—"
            ariaLabel={`Дистанция подхода ${set.setNumber}, метры`}
            suffix="м"
            disabled={saving}
            optional
          />
          <div className="hidden text-center text-sm text-ink-faint sm:block">
            {set.durationSeconds ? `${Math.round(set.durationSeconds / 60)} мин` : '—'}
          </div>
        </>
      ) : (
        <>
          {loaded ? (
            <NumberCell
              ref={weightRef}
              value={weight}
              onChange={setWeight}
              onCommit={commitWeight}
              onEnter={() => repsRef.current?.focus()}
              readOnly={readOnly}
              placeholder="—"
              ariaLabel={`Вес подхода ${set.setNumber}, кг`}
              suffix="кг"
              disabled={saving}
            />
          ) : (
            <div className="grid h-10 place-items-center rounded-lg bg-surface-raised/60 text-xs text-ink-faint" aria-label="Собственный вес">
              свой вес
            </div>
          )}
          <NumberCell
            ref={repsRef}
            value={reps}
            onChange={setReps}
            onCommit={commitReps}
            onEnter={commitReps}
            readOnly={readOnly}
            placeholder="—"
            ariaLabel={`Повторения подхода ${set.setNumber}`}
            suffix="раз"
            disabled={saving}
          />
          <div className="hidden text-center text-sm text-ink-faint sm:block">
            {set.weightKg !== null && set.repetitions > 0 ? formatWeight(set.weightKg * set.repetitions) : '—'}
          </div>
        </>
      )}

      {readOnly ? (
        <div />
      ) : (
        <button
          type="button"
          onClick={onCopy}
          disabled={saving}
          aria-label={`Скопировать подход ${set.setNumber} как следующий`}
          className="grid h-tap w-tap place-items-center rounded-lg text-ink-faint hover:bg-surface-hover hover:text-ink"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-4 w-4" aria-hidden="true">
            <rect x="9" y="9" width="11" height="11" rx="2" />
            <path d="M5 15V5a2 2 0 0 1 2-2h8" strokeLinecap="round" />
          </svg>
        </button>
      )}

      {readOnly ? (
        <div />
      ) : (
        <button
          type="button"
          onClick={onDelete}
          disabled={saving}
          aria-label={`Удалить подход ${set.setNumber}`}
          className="grid h-tap w-tap place-items-center rounded-lg text-ink-faint hover:bg-danger/10 hover:text-danger"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-4 w-4" aria-hidden="true">
            <path d="M6 7h12M10 7V5h4v2M8 7l1 12h6l1-12" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        </button>
      )}
    </div>
  )
}

interface NumberCellProps {
  value: string
  onChange: (value: string) => void
  onCommit: () => void
  onEnter: () => void
  readOnly: boolean
  placeholder: string
  ariaLabel: string
  suffix: string
  disabled: boolean
  optional?: boolean
}

const NumberCell = forwardRef<HTMLInputElement, NumberCellProps>(function NumberCell(
  { value, onChange, onCommit, onEnter, readOnly, placeholder, ariaLabel, suffix, disabled, optional = false },
  ref,
) {
  return (
    <label className="relative block">
      <input
        ref={ref}
        type="text"
        inputMode="decimal"
        autoComplete="off"
        enterKeyHint="next"
        value={value}
        placeholder={placeholder}
        readOnly={readOnly}
        disabled={disabled}
        aria-label={ariaLabel}
        onChange={(event) => onChange(event.target.value)}
        onBlur={onCommit}
        onKeyDown={(event) => {
          if (event.key === 'Enter') {
            event.preventDefault()
            onCommit()
            onEnter()
          }
        }}
        className={`w-full rounded-lg border border-line bg-canvas px-2 py-2 pr-7 text-center text-base tabular
          text-ink placeholder:text-ink-faint read-only:border-transparent read-only:bg-transparent
          focus:border-accent focus:outline-none ${disabled ? 'opacity-60' : ''} ${optional ? 'opacity-80' : ''}`}
      />
      <span className="pointer-events-none absolute right-1.5 top-1/2 -translate-y-1/2 text-[11px] text-ink-faint">
        {suffix}
      </span>
    </label>
  )
})
