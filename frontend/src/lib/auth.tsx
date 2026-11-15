// Authentication state for the whole app.
//
// The session lives in an HttpOnly cookie the JavaScript cannot read, so the only
// way to know who we are is to ask the server. The CSRF token does come back in
// the response body and is held in memory here — it is sent as a header on
// mutations, which is what a forged cross-site form cannot do.

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { ApiError, authApi, type CurrentUser } from './api'

interface AuthState {
  user: CurrentUser | null
  csrfToken: string
  /** True while the initial "who am I" request is in flight. */
  loading: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
  refresh: () => Promise<void>
}

const AuthContext = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<CurrentUser | null>(null)
  const [csrfToken, setCsrf] = useState('')
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(async () => {
    try {
      const result = await authApi.me()
      setUser(result.user)
      setCsrf(result.csrfToken)
    } catch (error) {
      // 401 is the normal "not signed in" answer, not a failure worth surfacing.
      if (error instanceof ApiError && error.status === 401) {
        setUser(null)
        setCsrf('')
      }
    }
  }, [])

  useEffect(() => {
    let cancelled = false
    void (async () => {
      await refresh()
      if (!cancelled) setLoading(false)
    })()
    return () => {
      cancelled = true
    }
  }, [refresh])

  // Any request that comes back 401 means the session is gone (expired, revoked,
  // account disabled). Drop the cached identity so the route guards redirect.
  useEffect(() => {
    const onUnauthorized = () => {
      setUser((current) => {
        // Ignore the 401 from a failed login attempt: there was no session yet.
        return current === null ? current : null
      })
      setCsrf('')
    }
    window.addEventListener('ironnote:unauthorized', onUnauthorized)
    return () => window.removeEventListener('ironnote:unauthorized', onUnauthorized)
  }, [])

  const login = useCallback(async (username: string, password: string) => {
    const result = await authApi.login(username, password)
    setUser(result.user)
    setCsrf(result.csrfToken)
  }, [])

  const logout = useCallback(async () => {
    try {
      await authApi.logout(csrfToken)
    } finally {
      setUser(null)
      setCsrf('')
    }
  }, [csrfToken])

  const value = useMemo(
    () => ({ user, csrfToken, loading, login, logout, refresh }),
    [user, csrfToken, loading, login, logout, refresh],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used inside AuthProvider')
  return ctx
}

/** Convenience: true when the signed-in user is an administrator. */
export function useIsAdmin(): boolean {
  return useAuth().user?.role === 'admin'
}
