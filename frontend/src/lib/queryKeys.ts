// Centralised React Query keys: one typo cannot orphan a stale cache entry.

import type { SessionFilter, StatsPeriod } from './types'

export const queryKeys = {
  muscleGroups: ['muscle-groups'] as const,
  exercises: (params: { search?: string; muscleGroupId?: number; includeArchived?: boolean }) =>
    ['exercises', params] as const,
  exercise: (id: number) => ['exercises', 'detail', id] as const,
  templates: ['templates'] as const,
  template: (id: number) => ['templates', 'detail', id] as const,
  sessions: (filter: SessionFilter) => ['sessions', filter] as const,
  session: (id: number) => ['sessions', 'detail', id] as const,
  activeSession: ['sessions', 'active'] as const,
  statsSummary: (period: StatsPeriod) => ['stats', 'summary', period] as const,
  statsExercises: (period: StatsPeriod) => ['stats', 'exercises', period] as const,
  statsExercise: (id: number, period: StatsPeriod) => ['stats', 'exercise', id, period] as const,
}
