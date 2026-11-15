// Single HTTP client for the API.
//
// Every screen goes through these functions: no component calls fetch directly,
// so error shaping, JSON casing and URL building exist in exactly one place.

import type {
  ApiErrorPayload,
  CurrentUser,
  Exercise,
  ExerciseBest,
  ExerciseStats,
  MuscleGroup,
  SessionFilter,
  SessionSummary,
  StatsPeriod,
  StatsSummary,
  Template,
  TemplateSummary,
  WorkoutSession,
} from './types'

// Re-exported so components import domain types from one place.
export type { CurrentUser }

const BASE = '/api'

/** ApiError carries the server's structured error so UI can show details. */
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly details: Record<string, string>

  constructor(status: number, code: string, message: string, details: Record<string, string> = {}) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.details = details
  }
}

/**
 * CSRF token holder.
 *
 * Stored in a module variable rather than a cookie: the server compares it to the
 * value bound to the session, and a cross-site page cannot read it. Set by the
 * auth provider after login and after a reload.
 */
let csrfToken = ''

export function setCSRFToken(token: string): void {
  csrfToken = token
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const method = (init?.method ?? 'GET').toUpperCase()
  const needsCSRF = !['GET', 'HEAD', 'OPTIONS'].includes(method)
  let response: Response
  try {
    response = await fetch(`${BASE}${path}`, {
      // Session cookie travels with the request; the browser sends it because the
      // API is same-origin (nginx in production, the Vite proxy in development).
      credentials: 'same-origin',
      ...init,
      headers: {
        Accept: 'application/json',
        ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
        ...(needsCSRF && csrfToken ? { 'X-CSRF-Token': csrfToken } : {}),
        ...init?.headers,
      },
    })
  } catch (cause) {
    throw new ApiError(0, 'network_error', 'Нет связи с сервером. Проверьте подключение.', {})
  }

  if (response.status === 204) {
    return undefined as T
  }

  const text = await response.text()
  const payload: unknown = text ? safeParse(text) : null

  if (response.status === 401) {
    // The session ended (expired, revoked, or the account was disabled). Signal it
    // so the shell can return to the login screen instead of showing empty data.
    window.dispatchEvent(new Event('ironnote:unauthorized'))
  }

  if (!response.ok) {
    const parsed = payload as ApiErrorPayload | null
    if (parsed?.error) {
      throw new ApiError(
        response.status,
        parsed.error.code,
        parsed.error.message,
        parsed.error.details ?? {},
      )
    }
    throw new ApiError(response.status, 'internal_error', 'Не удалось выполнить запрос.', {})
  }

  return payload as T
}

function safeParse(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return null
  }
}

function query(params: Record<string, string | number | boolean | undefined | null>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === '') continue
    search.set(key, String(value))
  }
  const raw = search.toString()
  return raw ? `?${raw}` : ''
}

const json = (body: unknown): RequestInit => ({ body: JSON.stringify(body) })

// --- authentication --------------------------------------------------------

export const authApi = {
  login: async (username: string, password: string) => {
    const result = await request<{ user: CurrentUser; csrfToken: string }>('/auth/login', {
      method: 'POST',
      ...json({ username, password }),
    })
    setCSRFToken(result.csrfToken)
    return result
  },
  logout: (csrf: string) =>
    request<void>('/auth/logout', {
      method: 'POST',
      headers: { 'X-CSRF-Token': csrf },
    }),
  me: async () => {
    const result = await request<{ user: CurrentUser; csrfToken: string }>('/auth/me')
    setCSRFToken(result.csrfToken)
    return result
  },
  changePassword: (currentPassword: string, newPassword: string) =>
    request<{ status: string }>('/auth/password', {
      method: 'POST',
      ...json({ currentPassword, newPassword }),
    }),
}

export const usersApi = {
  list: () => request<{ items: CurrentUser[] }>('/users').then((r) => r.items),
  create: (body: { username: string; displayName: string; password: string; role: 'admin' | 'user' }) =>
    request<CurrentUser>('/users', { method: 'POST', ...json(body) }),
  update: (id: number, body: { displayName?: string; role?: 'admin' | 'user'; isActive?: boolean }) =>
    request<CurrentUser>(`/users/${id}`, { method: 'PATCH', ...json(body) }),
  remove: (id: number) => request<void>(`/users/${id}`, { method: 'DELETE' }),
  resetPassword: (id: number, newPassword: string) =>
    request<void>(`/users/${id}/password`, { method: 'POST', ...json({ newPassword }) }),
}

// --- muscle groups ---------------------------------------------------------

export const muscleGroupsApi = {
  list: () => request<{ items: MuscleGroup[] }>('/muscle-groups').then((r) => r.items),
  create: (body: { name: string; description: string }) =>
    request<MuscleGroup>('/muscle-groups', { method: 'POST', ...json(body) }),
  update: (id: number, body: { name: string; description: string }) =>
    request<MuscleGroup>(`/muscle-groups/${id}`, { method: 'PATCH', ...json(body) }),
  remove: (id: number) => request<void>(`/muscle-groups/${id}`, { method: 'DELETE' }),
}

// --- exercises -------------------------------------------------------------

export interface ExerciseInput {
  name: string
  description: string
  equipment: string
  kind: import('./types').ExerciseKind
  muscleGroups: { muscleGroupId: number; isPrimary: boolean }[]
}

export const exercisesApi = {
  list: (params: { search?: string; muscleGroupId?: number; includeArchived?: boolean } = {}) =>
    request<{ items: Exercise[] }>(`/exercises${query(params)}`).then((r) => r.items),
  get: (id: number) => request<Exercise>(`/exercises/${id}`),
  create: (body: ExerciseInput) => request<Exercise>('/exercises', { method: 'POST', ...json(body) }),
  update: (id: number, body: ExerciseInput) =>
    request<Exercise>(`/exercises/${id}`, { method: 'PATCH', ...json(body) }),
  remove: (id: number) => request<void>(`/exercises/${id}`, { method: 'DELETE' }),
  archive: (id: number, archived: boolean) =>
    request<Exercise>(`/exercises/${id}/${archived ? 'archive' : 'unarchive'}`, { method: 'POST' }),
}

// --- templates -------------------------------------------------------------

export interface TemplateInput {
  name: string
  description: string
  exercises: {
    exerciseId: number
    plannedSets: number | null
    plannedReps: number | null
    plannedWeightKg: number | null
    plannedDurationSeconds: number | null
  }[]
}

export const templatesApi = {
  list: () => request<{ items: TemplateSummary[] }>('/templates').then((r) => r.items),
  get: (id: number) => request<Template>(`/templates/${id}`),
  create: (body: TemplateInput) => request<Template>('/templates', { method: 'POST', ...json(body) }),
  update: (id: number, body: TemplateInput) =>
    request<Template>(`/templates/${id}`, { method: 'PUT', ...json(body) }),
  remove: (id: number) => request<void>(`/templates/${id}`, { method: 'DELETE' }),
  start: (id: number) => request<WorkoutSession>(`/templates/${id}/start`, { method: 'POST' }),
}

// --- sessions --------------------------------------------------------------

export interface SetInput {
  weightKg?: number | null
  repetitions?: number
  // For timed exercises: seconds, plus an optional distance.
  durationSeconds?: number | null
  distanceMeters?: number | null
  isWarmup?: boolean
  isCompleted?: boolean
  copyPrevious?: boolean
}

export const sessionsApi = {
  list: (filter: SessionFilter = {}) =>
    request<{ items: SessionSummary[] }>(`/sessions${query({ ...filter })}`).then((r) => r.items),
  get: (id: number) => request<WorkoutSession>(`/sessions/${id}`),
  active: () => request<{ session: WorkoutSession | null }>('/sessions/active').then((r) => r.session),
  start: (body: { name?: string; templateId?: number; notes?: string }) =>
    request<WorkoutSession>('/sessions', { method: 'POST', ...json(body) }),
  update: (
    id: number,
    body: { name?: string; notes?: string; startedAt?: string; durationMinutes?: number },
  ) =>
    request<WorkoutSession>(`/sessions/${id}`, { method: 'PATCH', ...json(body) }),
  finish: (id: number) => request<WorkoutSession>(`/sessions/${id}/finish`, { method: 'POST' }),
  reopen: (id: number) => request<WorkoutSession>(`/sessions/${id}/reopen`, { method: 'POST' }),
  remove: (id: number) => request<void>(`/sessions/${id}`, { method: 'DELETE' }),
  addExercise: (sessionId: number, exerciseId: number, notes = '') =>
    request<{ exercise: { id: number } }>(`/sessions/${sessionId}/exercises`, {
      method: 'POST',
      ...json({ exerciseId, notes }),
    }),
  moveExercise: (sessionId: number, sessionExerciseId: number, direction: -1 | 1) =>
    request<WorkoutSession>(`/sessions/${sessionId}/exercises/${sessionExerciseId}`, {
      method: 'PATCH',
      ...json({ direction }),
    }),
  removeExercise: (sessionId: number, sessionExerciseId: number) =>
    request<void>(`/sessions/${sessionId}/exercises/${sessionExerciseId}`, { method: 'DELETE' }),
  updateExerciseNotes: (sessionId: number, sessionExerciseId: number, notes: string) =>
    request<void>(`/sessions/${sessionId}/exercises/${sessionExerciseId}/notes`, {
      method: 'PATCH',
      ...json({ notes }),
    }),
  addSet: (sessionId: number, sessionExerciseId: number, body: SetInput) =>
    request<import('./types').WorkoutSet>(`/sessions/${sessionId}/exercises/${sessionExerciseId}/sets`, {
      method: 'POST',
      ...json(body),
    }),
  updateSet: (sessionId: number, sessionExerciseId: number, setId: number, body: SetInput) =>
    request<import('./types').WorkoutSet>(
      `/sessions/${sessionId}/exercises/${sessionExerciseId}/sets/${setId}`,
      { method: 'PATCH', ...json(body) },
    ),
  removeSet: (sessionId: number, sessionExerciseId: number, setId: number) =>
    request<void>(`/sessions/${sessionId}/exercises/${sessionExerciseId}/sets/${setId}`, {
      method: 'DELETE',
    }),
}

// --- statistics ------------------------------------------------------------

export const statsApi = {
  summary: (period: StatsPeriod = {}) =>
    request<StatsSummary>(`/stats/summary${query({ ...period })}`),
  exercises: (period: StatsPeriod = {}) =>
    request<{ items: ExerciseBest[] }>(`/stats/exercises${query({ ...period })}`).then((r) => r.items),
  exercise: (id: number, period: StatsPeriod = {}) =>
    request<ExerciseStats>(`/stats/exercises/${id}${query({ ...period })}`),
}
