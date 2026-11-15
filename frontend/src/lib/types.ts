// Domain shapes returned by the API.
//
// These mirror the JSON contract exactly. Weight and repetitions are nullable on
// purpose: a bodyweight set has no weight, and that is different from zero.

export interface MuscleGroup {
  id: number
  name: string
  description: string
  createdAt: string
  updatedAt: string
}

export interface ExerciseGroup {
  muscleGroupId: number
  muscleGroupName: string
  isPrimary: boolean
}

export interface Exercise {
  id: number
  name: string
  description: string
  equipment: string
  isArchived: boolean
  muscleGroups: ExerciseGroup[]
  createdAt: string
  updatedAt: string
}

export interface TemplateExercise {
  id: number
  exerciseId: number
  exerciseName: string
  exerciseArchived: boolean
  sortOrder: number
  plannedSets: number | null
  plannedReps: number | null
  plannedWeightKg: number | null
}

export interface Template {
  id: number
  name: string
  description: string
  exercises: TemplateExercise[]
  createdAt: string
  updatedAt: string
}

export interface TemplateSummary {
  id: number
  name: string
  description: string
  exerciseCount: number
  plannedSets: number
  createdAt: string
  updatedAt: string
}

export interface WorkoutSet {
  id: number
  setNumber: number
  weightKg: number | null
  repetitions: number
  isWarmup: boolean
  isCompleted: boolean
}

export interface SessionExercise {
  id: number
  exerciseId: number
  exerciseName: string
  equipment: string
  exerciseArchived: boolean
  sortOrder: number
  notes: string
  sets: WorkoutSet[]
  previousSets: WorkoutSet[]
  previousAt: string | null
}

export type SessionStatus = 'in_progress' | 'completed'

export interface WorkoutSession {
  id: number
  templateId: number | null
  name: string
  startedAt: string
  finishedAt: string | null
  status: SessionStatus
  notes: string
  exercises: SessionExercise[]
  exerciseCount: number
  completedSetCount: number
  volumeKg: number
  durationMinutes: number | null
}

export interface SessionSummary {
  id: number
  templateId: number | null
  name: string
  startedAt: string
  finishedAt: string | null
  status: SessionStatus
  notes: string
  exerciseCount: number
  completedSetCount: number
  volumeKg: number
  durationMinutes: number | null
}

export interface MonthlyPoint {
  month: string
  sessionCount: number
}

export interface StatsSummary {
  from: string
  to: string
  sessionCount: number
  completedSetCount: number
  volumeKg: number
  exerciseCount: number
  averageDurationMinutes: number
  monthly: MonthlyPoint[]
}

export interface ExerciseBest {
  exerciseId: number
  exerciseName: string
  completedSetCount: number
  volumeKg: number
  maxWeightKg: number | null
  maxSingleKg: number | null
  estimated1RMKg: number | null
  bestSetVolumeKg: number | null
  lastPerformedAt: string | null
}

export interface ExerciseSeriesPoint {
  sessionId: number
  date: string
  volumeKg: number
  maxWeightKg: number | null
  estimated1RMKg: number | null
}

export interface SetHistoryEntry {
  sessionId: number
  date: string
  setNumber: number
  weightKg: number | null
  repetitions: number
  isWarmup: boolean
}

export interface ExerciseStats {
  from: string
  to: string
  exerciseId: number
  exerciseName: string
  series: ExerciseSeriesPoint[]
  maxWeightKg: number | null
  maxSingleKg: number | null
  estimated1RMKg: number | null
  totalVolumeKg: number
  totalSets: number
  recentSets: SetHistoryEntry[]
}

export interface SessionFilter {
  status?: SessionStatus
  from?: string
  to?: string
}

export interface StatsPeriod {
  from?: string
  to?: string
}

export interface ApiErrorPayload {
  error: {
    code: string
    message: string
    details?: Record<string, string>
  }
}
