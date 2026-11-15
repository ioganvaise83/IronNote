// Display formatting. All dates arrive as RFC 3339 UTC instants and are rendered
// in the browser's locale, which is what a user expects for their own log.

const LOCALE = 'ru-RU'

export function formatWeight(value: number | null | undefined): string {
  if (value === null || value === undefined) return '—'
  // 60.0 → "60", 61.25 → "61,25": trailing zeros are noise on a gym screen.
  return new Intl.NumberFormat(LOCALE, { maximumFractionDigits: 2 }).format(value)
}

export function formatNumber(value: number): string {
  return new Intl.NumberFormat(LOCALE, { maximumFractionDigits: 1 }).format(value)
}

export function formatVolume(kg: number): string {
  if (kg >= 10000) return `${Math.round(kg / 1000)} т`
  return `${formatNumber(kg)} кг`
}

export function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString(LOCALE, { day: 'numeric', month: 'long', year: 'numeric' })
}

export function formatDateShort(iso: string): string {
  return new Date(iso).toLocaleDateString(LOCALE, { day: '2-digit', month: '2-digit' })
}

export function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString(LOCALE, { hour: '2-digit', minute: '2-digit' })
}

export function formatDateTime(iso: string): string {
  return `${formatDate(iso)}, ${formatTime(iso)}`
}

/** Weekday + day, useful as a list header. */
export function formatDayHeader(iso: string): string {
  return new Date(iso).toLocaleDateString(LOCALE, {
    weekday: 'short',
    day: 'numeric',
    month: 'long',
  })
}

export function formatDuration(minutes: number | null | undefined): string {
  if (minutes === null || minutes === undefined) return '—'
  const total = Math.max(0, Math.round(minutes))
  const hours = Math.floor(total / 60)
  const mins = total % 60
  if (hours === 0) return `${mins} мин`
  if (mins === 0) return `${hours} ч`
  return `${hours} ч ${mins} мин`
}

/** Elapsed time since a start instant, as "1:04:09" or "4:09". */
export function formatElapsed(startedAt: string, now: Date = new Date()): string {
  const diffMs = Math.max(0, now.getTime() - new Date(startedAt).getTime())
  const totalSeconds = Math.floor(diffMs / 1000)
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  const pad = (n: number) => n.toString().padStart(2, '0')
  return hours > 0 ? `${hours}:${pad(minutes)}:${pad(seconds)}` : `${minutes}:${pad(seconds)}`
}

/** ISO date (YYYY-MM-DD) in local time, for <input type="date">. */
export function toDateInput(iso: string): string {
  const d = new Date(iso)
  const pad = (n: number) => n.toString().padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

export function monthLabel(month: string): string {
  // "2026-03" → "март"
  const [year, m] = month.split('-')
  if (!year || !m) return month
  const date = new Date(Number(year), Number(m) - 1, 1)
  return date.toLocaleDateString(LOCALE, { month: 'short' })
}

export function plural(count: number, one: string, few: string, many: string): string {
  const mod10 = count % 10
  const mod100 = count % 100
  if (mod10 === 1 && mod100 !== 11) return one
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 10 || mod100 >= 20)) return few
  return many
}

export function setSummary(weight: number | null, repetitions: number): string {
  return weight === null
    ? `${repetitions} ×`
    : `${formatWeight(weight)} кг × ${repetitions}`
}

/** "30 мин", "1 ч 15 мин" — the length of one timed set. */
export function formatSetDuration(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined || seconds <= 0) return '—'
  const total = Math.round(seconds / 60)
  if (total < 60) return `${total} мин`
  const hours = Math.floor(total / 60)
  const mins = total % 60
  return mins === 0 ? `${hours} ч` : `${hours} ч ${mins} мин`
}

/** Kilometres for anything from a lap upwards, metres below that. */
export function formatDistance(meters: number | null | undefined): string {
  if (meters === null || meters === undefined || meters <= 0) return '—'
  if (meters >= 1000) {
    return `${new Intl.NumberFormat(LOCALE, { maximumFractionDigits: 2 }).format(meters / 1000)} км`
  }
  return `${Math.round(meters)} м`
}

/** Minutes per kilometre, the number runners and swimmers actually think in. */
export function pace(seconds: number | null, meters: number | null): string {
  if (!seconds || !meters || meters <= 0) return '—'
  const secondsPerKm = seconds / (meters / 1000)
  const mins = Math.floor(secondsPerKm / 60)
  const secs = Math.round(secondsPerKm % 60)
  return `${mins}:${secs.toString().padStart(2, '0')} /км`
}

/** Total cardio time as "1 ч 45 мин". */
export function formatTotalTime(seconds: number | null | undefined): string {
  return formatSetDuration(seconds)
}
