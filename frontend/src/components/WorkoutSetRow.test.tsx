// The set row is where the product succeeds or fails: it must accept a weight,
// accept reps, and persist without a modal or a save button.

import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { WorkoutSetRow } from './WorkoutSetRow'
import type { WorkoutSet } from '../lib/types'

function makeSet(overrides: Partial<WorkoutSet> = {}): WorkoutSet {
  return {
    id: 1,
    setNumber: 1,
    kind: 'weighted',
    weightKg: 80,
    repetitions: 10,
    durationSeconds: null,
    distanceMeters: null,
    isWarmup: false,
    isCompleted: true,
    ...overrides,
  }
}

describe('WorkoutSetRow', () => {
  it('shows the current values in editable inputs', () => {
    render(<WorkoutSetRow set={makeSet()} onSave={vi.fn()} onDelete={vi.fn()} onCopy={vi.fn()} />)
    expect(screen.getByLabelText('Вес подхода 1, кг')).toHaveValue('80')
    expect(screen.getByLabelText('Повторения подхода 1')).toHaveValue('10')
  })

  it('saves a changed weight on blur, without a save button', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    render(<WorkoutSetRow set={makeSet()} onSave={onSave} onDelete={vi.fn()} onCopy={vi.fn()} />)

    const weight = screen.getByLabelText('Вес подхода 1, кг')
    await user.clear(weight)
    await user.type(weight, '82,5')
    await user.tab()

    expect(onSave).toHaveBeenCalledWith({ weightKg: 82.5 })
  })

  it('saves repetitions when Enter is pressed', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    render(<WorkoutSetRow set={makeSet()} onSave={onSave} onDelete={vi.fn()} onCopy={vi.fn()} />)

    const reps = screen.getByLabelText('Повторения подхода 1')
    await user.clear(reps)
    await user.type(reps, '12{Enter}')

    expect(onSave).toHaveBeenCalledWith({ repetitions: 12 })
  })

  it('does not call the API when nothing changed', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    render(<WorkoutSetRow set={makeSet()} onSave={onSave} onDelete={vi.fn()} onCopy={vi.fn()} />)

    const weight = screen.getByLabelText('Вес подхода 1, кг')
    await user.click(weight)
    await user.tab()

    expect(onSave).not.toHaveBeenCalled()
  })

  it('rejects invalid input by restoring the previous value', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    render(<WorkoutSetRow set={makeSet()} onSave={onSave} onDelete={vi.fn()} onCopy={vi.fn()} />)

    const reps = screen.getByLabelText('Повторения подхода 1')
    await user.clear(reps)
    await user.type(reps, '0')
    await user.tab()

    expect(onSave).not.toHaveBeenCalled()
    expect(screen.getByLabelText('Повторения подхода 1')).toHaveValue('10')
  })

  it('toggles completion from the set-number button', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    render(<WorkoutSetRow set={makeSet({ isCompleted: false })} onSave={onSave} onDelete={vi.fn()} onCopy={vi.fn()} />)

    await user.click(screen.getByRole('button', { name: 'Отметить подход 1 выполненным' }))
    expect(onSave).toHaveBeenCalledWith({ isCompleted: true })
  })

  it('is read-only for a finished workout', () => {
    render(<WorkoutSetRow set={makeSet()} readOnly onSave={vi.fn()} onDelete={vi.fn()} onCopy={vi.fn()} />)
    expect(screen.getByLabelText('Вес подхода 1, кг')).toHaveAttribute('readonly')
    expect(screen.queryByRole('button', { name: /Удалить подход/ })).not.toBeInTheDocument()
  })

  it('handles a bodyweight set (no weight recorded)', () => {
    render(<WorkoutSetRow set={makeSet({ weightKg: null })} onSave={vi.fn()} onDelete={vi.fn()} onCopy={vi.fn()} />)
    expect(screen.getByLabelText('Вес подхода 1, кг')).toHaveValue('')
  })
})
