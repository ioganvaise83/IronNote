// Route guards.
//
// These are a usability layer, not a security boundary: the server authorises every
// request independently. Hiding an admin button stops accidents; it is not what
// stops an attacker.

import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useAuth, useIsAdmin } from '../lib/auth'
import { Spinner } from './ui'

/** Redirects to the login screen when there is no session. */
export function RequireAuth() {
  const { user, loading } = useAuth()
  const location = useLocation()

  if (loading) return <FullScreenSpinner />
  if (!user) return <Navigate to="/login" replace state={{ from: location.pathname }} />
  // A forced password change intercepts every other route.
  if (user.mustChangePassword) return <Navigate to="/change-password" replace />
  return <Outlet />
}

/** For pages that must remain reachable while a password change is pending. */
export function RequireSession() {
  const { user, loading } = useAuth()
  if (loading) return <FullScreenSpinner />
  if (!user) return <Navigate to="/login" replace />
  return <Outlet />
}

export function RequireAdmin() {
  const isAdmin = useIsAdmin()
  if (!isAdmin) return <Navigate to="/" replace />
  return <Outlet />
}

/** Sends an already-signed-in user away from the login screen. */
export function RedirectIfAuthed() {
  const { user, loading } = useAuth()
  if (loading) return <FullScreenSpinner />
  if (user) {
    return <Navigate to={user.mustChangePassword ? '/change-password' : '/'} replace />
  }
  return <Outlet />
}

function FullScreenSpinner() {
  return (
    <div className="flex min-h-dvh items-center justify-center bg-canvas text-ink-muted">
      <Spinner className="h-6 w-6" />
    </div>
  )
}
