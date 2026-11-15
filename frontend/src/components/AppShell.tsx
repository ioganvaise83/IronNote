// Application chrome: header, responsive navigation, and the layout contract.
//
// Mobile: fixed bottom navigation (safe-area aware) with five destinations.
// Desktop (lg+): the same five destinations become a left sidebar. One source of
// truth for the routes keeps the two from drifting apart.

import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useState, type ReactNode } from 'react'
import { useAuth } from '../lib/auth'
import { Button } from './ui'

interface NavItem {
  to: string
  label: string
  icon: ReactNode
  /** Hide the bottom bar on these routes: the active workout is a focus mode. */
  hideNav?: boolean
}

function WorkoutsIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true">
      <path d="M4 9v6M7 6v12M17 6v12M20 9v6M7 12h10" strokeLinecap="round" />
    </svg>
  )
}
function HistoryIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true">
      <circle cx="12" cy="12" r="8.5" />
      <path d="M12 7.5V12l3 2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}
function ExercisesIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true">
      <path d="M4 19.5V6.5A2 2 0 0 1 6 4.5h11.5" strokeLinecap="round" />
      <path d="M4 12h9M4 19.5h14a2 2 0 0 0 2-2v-1.5" strokeLinecap="round" />
      <circle cx="16.5" cy="8.5" r="2.5" />
    </svg>
  )
}
function StatsIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true">
      <path d="M5 19V10M12 19V5M19 19v-6" strokeLinecap="round" />
    </svg>
  )
}
function SettingsIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true">
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-2.9 1.2v.2a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-2.9-1.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1A1.7 1.7 0 0 0 4.6 15H4.4a2 2 0 1 1 0-4h.2a1.7 1.7 0 0 0 1.2-2.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1A1.7 1.7 0 0 0 11.5 4.6V4.4a2 2 0 1 1 4 0v.2a1.7 1.7 0 0 0 2.9 1.2l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0 1.2 2.9h.2a2 2 0 1 1 0 4h-.2a1.7 1.7 0 0 0-1.1.9z" />
    </svg>
  )
}

export const NAV_ITEMS: NavItem[] = [
  { to: '/', label: 'Тренировки', icon: <WorkoutsIcon /> },
  { to: '/history', label: 'История', icon: <HistoryIcon /> },
  { to: '/exercises', label: 'Упражнения', icon: <ExercisesIcon /> },
  { to: '/stats', label: 'Статистика', icon: <StatsIcon /> },
  { to: '/settings', label: 'Настройки', icon: <SettingsIcon /> },
]

export function AppShell() {
  const location = useLocation()
  const isActiveWorkout = /^\/workouts\/[^/]+$/.test(location.pathname)

  return (
    <div className="min-h-dvh bg-canvas lg:flex">
      {/* Desktop sidebar: same navigation, wider canvas */}
      <aside className="hidden border-r border-line lg:flex lg:w-60 lg:shrink-0 lg:flex-col lg:sticky lg:top-0 lg:h-dvh">
        <div className="px-5 py-6">
          <Brand />
        </div>
        <nav aria-label="Основная навигация" className="flex flex-col gap-1 px-3">
          {NAV_ITEMS.map((item) => (
            <SidebarLink key={item.to} item={item} />
          ))}
          <AdminSidebarLink />
        </nav>
        <div className="mt-auto px-3 pb-4">
          <AccountCard />
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <main
          id="main"
          className={`mx-auto w-full max-w-2xl flex-1 px-4 pt-4 ${
            isActiveWorkout ? 'pb-10' : 'page-with-nav lg:pb-10'
          }`}
        >
          <Outlet />
        </main>

        {!isActiveWorkout && (
          <nav
            aria-label="Основная навигация"
            className="fixed inset-x-0 bottom-0 z-40 border-t border-line bg-surface/95 backdrop-blur lg:hidden"
            style={{ paddingBottom: 'var(--safe-bottom)' }}
          >
            <ul className="mx-auto flex w-full max-w-2xl items-stretch justify-between overflow-hidden px-1">
              {NAV_ITEMS.map((item) => (
                <li key={item.to} className="min-w-0 flex-1">
                  <BottomLink item={item} />
                </li>
              ))}
            </ul>
          </nav>
        )}
      </div>
    </div>
  )
}

function Brand() {
  return (
    <div className="flex items-center gap-2">
      <span className="grid h-9 w-9 place-items-center rounded-xl bg-accent text-white">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" className="h-5 w-5">
          <path d="M4 9v6M7 6v12M17 6v12M20 9v6M7 12h10" strokeLinecap="round" />
        </svg>
      </span>
      <span className="text-lg font-semibold tracking-tight">IronNote</span>
    </div>
  )
}

function SidebarLink({ item }: { item: NavItem }) {
  return (
    <NavLink
      to={item.to}
      end={item.to === '/'}
      className={({ isActive }) =>
        `flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition ${
          isActive ? 'bg-accent-soft text-ink' : 'text-ink-muted hover:bg-surface-hover hover:text-ink'
        }`
      }
    >
      <span className="h-5 w-5">{item.icon}</span>
      {item.label}
    </NavLink>
  )
}

/** Admin-only destination, kept out of NAV_ITEMS so normal users never see it. */
function AdminSidebarLink() {
  const { user } = useAuth()
  if (user?.role !== 'admin') return null
  return (
    <NavLink
      to="/users"
      className={({ isActive }) =>
        `flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition ${
          isActive ? 'bg-accent-soft text-ink' : 'text-ink-muted hover:bg-surface-hover hover:text-ink'
        }`
      }
    >
      <span className="h-5 w-5">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
          <circle cx="9" cy="8" r="3.2" />
          <path d="M3.5 19a5.5 5.5 0 0 1 11 0M16 11.2a2.8 2.8 0 1 0 0-5.6M17 19a4.6 4.6 0 0 0-2-3.8" strokeLinecap="round" />
        </svg>
      </span>
      Пользователи
    </NavLink>
  )
}

function BottomLink({ item }: { item: NavItem }) {
  return (
    <NavLink
      to={item.to}
      end={item.to === '/'}
      className={({ isActive }) =>
        `flex h-[var(--nav-height)] min-w-0 flex-col items-center justify-center gap-0.5 px-0.5
          text-[10px] font-medium leading-tight transition xs:text-[11px] ${
          isActive ? 'text-accent' : 'text-ink-faint'
        }`
      }
    >
      {({ isActive }) => (
        <>
          <span className={`h-6 w-6 shrink-0 ${isActive ? 'text-accent' : ''}`}>{item.icon}</span>
          <span className="w-full truncate text-center">{item.label}</span>
        </>
      )}
    </NavLink>
  )
}

/** Page header with an optional back link and trailing actions. */
export function PageHeader({
  title,
  subtitle,
  back,
  actions,
}: {
  title: string
  subtitle?: string
  back?: { to: string; label?: string }
  actions?: ReactNode
}) {
  return (
    <header className="mb-4 flex items-start justify-between gap-3">
      <div className="min-w-0">
        {back && (
          <NavLink
            to={back.to}
            className="-ml-2 mb-1 inline-flex min-h-tap items-center gap-1 rounded-lg px-2 text-sm text-ink-muted hover:text-ink"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="h-4 w-4">
              <path d="M15 18l-6-6 6-6" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
            {back.label ?? 'Назад'}
          </NavLink>
        )}
        <h1 className="truncate text-2xl font-semibold tracking-tight">{title}</h1>
        {subtitle && <p className="mt-1 text-sm text-ink-muted">{subtitle}</p>}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </header>
  )
}

/**
 * Signed-in identity, logout and the admin-only Users link.
 *
 * Rendered twice (sidebar and mobile settings entry) so it lives in one
 * component; the caller decides placement.
 */
function AccountCard() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const [busy, setBusy] = useState(false)

  if (!user) return null

  async function signOut() {
    setBusy(true)
    try {
      await logout()
      navigate('/login', { replace: true })
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="rounded-xl border border-line bg-surface-raised p-3">
      <p className="truncate text-sm font-medium">{user.displayName || user.username}</p>
      <p className="truncate text-xs text-ink-faint">
        {user.username}
        {user.role === 'admin' ? ' · администратор' : ''}
      </p>
      <div className="mt-2 flex flex-col gap-1">
        <NavLink
          to="/change-password"
          className="rounded-lg px-2 py-1.5 text-xs text-ink-muted hover:bg-surface-hover hover:text-ink"
        >
          Сменить пароль
        </NavLink>
        {user.role === 'admin' && (
          <NavLink
            to="/users"
            className="rounded-lg px-2 py-1.5 text-xs text-ink-muted hover:bg-surface-hover hover:text-ink"
          >
            Пользователи
          </NavLink>
        )}
        <Button variant="ghost" className="justify-start px-2 text-xs" onClick={() => void signOut()} loading={busy}>
          Выйти
        </Button>
      </div>
    </div>
  )
}

export { AccountCard }
