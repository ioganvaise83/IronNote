// A timed movement (бег, плавание) records minutes, not weight × reps. These tests
// pin the input contract so the row does not fall back to asking for a weight.

import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { WorkoutSetRow } from './WorkoutSetRow'
import type { WorkoutSet } from '../lib/types'

function timedSet(overrides: Partial<WorkoutSet> = {}): WorkoutSet {
  return {
    id: 1,
    setNumber: 1,
    kind: 'duration',
    weightKg: null,
    repetitions: 0,
    durationSeconds: 1800,
    distanceMeters: 5000,
    isWarmup: false,
    isCompleted: true,
    ...overrides,
  }
}

describe('WorkoutSetRow for timed exercises', () => {
  it('shows minutes and distance instead of weight and reps', () => {
    render(<WorkoutSetRow set={timedSet()} kind="duration" onSave={vi.fn()} onDelete={vi.fn()} onCopy={vi.fn()} />)

    expect(screen.getByLabelText('Длительность подхода 1, минуты')).toHaveValue('30')
    expect(screen.getByLabelText('Дистанция подхода 1, метры')).toHaveValue('5000')
    expect(screen.queryByLabelText('Вес подхода 1, кг')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Повторения подхода 1')).not.toBeInTheDocument()
  })

  it('saves duration in seconds when minutes are edited', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    render(<WorkoutSetRow set={timedSet()} kind="duration" onSave={onSave} onDelete={vi.fn()} onCopy={vi.fn()} />)

    const field = screen.getByLabelText('Длительность подхода 1, минуты')
    await user.clear(field)
    await user.type(field, '45')
    await user.tab()

    expect(onSave).toHaveBeenCalledWith({ durationSeconds: 45 * 60 })
  })

  it('accepts fractional minutes', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    render(<WorkoutSetRow set={timedSet()} kind="duration" onSave={onSave} onDelete={vi.fn()} onCopy={vi.fn()} />)

    const field = screen.getByLabelText('Длительность подхода 1, минуты')
    await user.clear(field)
    await user.type(field, '1,5')
    await user.tab()

    expect(onSave).toHaveBeenCalledWith({ durationSeconds: 90 })
  })

  it('refuses an empty duration by restoring the saved value', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    render(<WorkoutSetRow set={timedSet()} kind="duration" onSave={onSave} onDelete={vi.fn()} onCopy={vi.fn()} />)

    const field = screen.getByLabelText('Длительность подхода 1, минуты')
    await user.clear(field)
    await user.tab()

    expect(onSave).not.toHaveBeenCalled()
    expect(screen.getByLabelText('Длительность подхода 1, минуты')).toHaveValue('30')
  })

  it('saves an edited distance', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    render(<WorkoutSetRow set={timedSet()} kind="duration" onSave={onSave} onDelete={vi.fn()} onCopy={vi.fn()} />)

    const field = screen.getByLabelText('Дистанция подхода 1, метры')
    await user.clear(field)
    await user.type(field, '6000')
    await user.tab()

    expect(onSave).toHaveBeenCalledWith({ distanceMeters: 6000 })
  })

  it('shows a bodyweight exercise without a weight input', () => {
    const set: WorkoutSet = {
      ...timedSet(),
      kind: 'bodyweight',
      durationSeconds: null,
      distanceMeters: null,
      repetitions: 12,
      weightKg: null,
    }
    render(<WorkoutSetRow set={set} kind="bodyweight" onSave={vi.fn()} onDelete={vi.fn()} onCopy={vi.fn()} />)

    expect(screen.getByLabelText('Повторения подхода 1')).toHaveValue('12')
    expect(screen.queryByLabelText('Вес подхода 1, кг')).not.toBeInTheDocument()
    expect(screen.getByLabelText('Собственный вес')).toBeInTheDocument()
  })
})
