import { describe, expect, it } from 'vitest'
import {
  formatDistance,
  formatDuration,
  formatSetDuration,
  pace,
  formatElapsed,
  formatVolume,
  formatWeight,
  monthLabel,
  plural,
  setSummary,
  toDateInput,
} from './format'

describe('formatWeight', () => {
  it('renders a missing weight as a dash, not zero', () => {
    expect(formatWeight(null)).toBe('—')
    expect(formatWeight(undefined)).toBe('—')
  })

  it('drops needless decimals but keeps plate precision', () => {
    expect(formatWeight(60)).toBe('60')
    expect(formatWeight(61.25)).toBe('61,25')
  })
})

describe('formatVolume', () => {
  it('switches to tonnes for large totals', () => {
    expect(formatVolume(12500)).toBe('13 т')
    expect(formatVolume(1500)).toBe('1 500 кг')
  })
})

describe('formatDuration', () => {
  it('formats minutes, hours and mixed durations', () => {
    expect(formatDuration(45)).toBe('45 мин')
    expect(formatDuration(120)).toBe('2 ч')
    expect(formatDuration(75)).toBe('1 ч 15 мин')
    expect(formatDuration(null)).toBe('—')
  })
})

describe('formatElapsed', () => {
  it('counts up from the start instant', () => {
    const start = new Date('2026-03-01T10:00:00Z')
    const now = new Date('2026-03-01T10:04:09Z')
    expect(formatElapsed(start.toISOString(), now)).toBe('4:09')
    const later = new Date('2026-03-01T11:04:09Z')
    expect(formatElapsed(start.toISOString(), later)).toBe('1:04:09')
  })
})

describe('plural', () => {
  it('chooses the Russian form for each count class', () => {
    expect(plural(1, 'подход', 'подхода', 'подходов')).toBe('подход')
    expect(plural(3, 'подход', 'подхода', 'подходов')).toBe('подхода')
    expect(plural(5, 'подход', 'подхода', 'подходов')).toBe('подходов')
    expect(plural(11, 'подход', 'подхода', 'подходов')).toBe('подходов')
    expect(plural(21, 'подход', 'подхода', 'подходов')).toBe('подход')
  })
})

describe('setSummary', () => {
  it('omits the weight for bodyweight sets', () => {
    expect(setSummary(80, 10)).toBe('80 кг × 10')
    expect(setSummary(null, 12)).toBe('12 ×')
  })
})

describe('monthLabel', () => {
  it('formats a month key', () => {
    expect(monthLabel('2026-03')).toMatch(/март|мар/i)
  })
})

describe('toDateInput', () => {
  it('produces a local YYYY-MM-DD value for a date input', () => {
    expect(toDateInput(new Date(2026, 2, 7, 15, 30).toISOString())).toBe('2026-03-07')
  })
})

describe('cardio formatting', () => {
  it('renders set duration in minutes and hours', () => {
    expect(formatSetDuration(1800)).toBe('30 мин')
    expect(formatSetDuration(4500)).toBe('1 ч 15 мин')
    expect(formatSetDuration(7200)).toBe('2 ч')
    expect(formatSetDuration(null)).toBe('—')
  })

  it('renders distance in km above a kilometre', () => {
    expect(formatDistance(5000)).toBe('5 км')
    expect(formatDistance(750)).toBe('750 м')
    expect(formatDistance(null)).toBe('—')
  })

  it('computes pace per kilometre', () => {
    expect(pace(1800, 5000)).toBe('6:00 /км')
    expect(pace(900, 3000)).toBe('5:00 /км')
    expect(pace(1800, null)).toBe('—')
  })
})
