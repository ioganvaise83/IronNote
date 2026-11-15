// Domain shapes returned by the API.
//
// These mirror the JSON contract exactly. Weight and repetitions are nullable on
// purpose: a bodyweight set has no weight, and that is different from zero.

export type UserRole = 'admin' | 'user'

/**
 * What a movement measures.
 * - weighted: weight × repetitions (жим, приседания)
 * - bodyweight: repetitions only (подтягивания, отжимания)
 * - duration: time, optionally distance (бег, плавание, эллипсоид)
 */
export type ExerciseKind = 'weighted' | 'bodyweight' | 'duration'

export interface CurrentUser {
  id: number
  username: string
  displayName: string
  role: UserRole
  isActive: boolean
  mustChangePassword: boolean
  lastLoginAt: string | null
  createdAt: string
  updatedAt: string
}

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
  kind: ExerciseKind
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
  plannedDurationSeconds: number | null
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
  kind: ExerciseKind
  weightKg: number | null
  /** 0 for a timed set: the server sends 0 because repetitions is nullable. */
  repetitions: number
  durationSeconds: number | null
  distanceMeters: number | null
  isWarmup: boolean
  isCompleted: boolean
}

export interface SessionExercise {
  id: number
  exerciseId: number
  exerciseName: string
  equipment: string
  kind: ExerciseKind
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
  /** Total time recorded by timed sets, not the wall-clock workout length. */
  durationSeconds: number
  distanceMeters: number
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
  /** Total time recorded by timed sets, not the wall-clock workout length. */
  durationSeconds: number
  distanceMeters: number
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
  totalSeconds: number
  totalMeters: number
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
  totalSeconds: number
  totalMeters: number
  longestSeconds: number | null
  lastPerformedAt: string | null
}

export interface ExerciseSeriesPoint {
  sessionId: number
  date: string
  volumeKg: number
  maxWeightKg: number | null
  estimated1RMKg: number | null
  seconds: number
  meters: number
}

export interface SetHistoryEntry {
  sessionId: number
  date: string
  setNumber: number
  weightKg: number | null
  repetitions: number
  durationSeconds: number | null
  distanceMeters: number | null
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
  totalSeconds: number
  totalMeters: number
  longestSeconds: number | null
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
