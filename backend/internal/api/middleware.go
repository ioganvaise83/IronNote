package api

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/ioganvaise83/ironnote/internal/apperror"
	"github.com/ioganvaise83/ironnote/internal/auth"
)

// logRequests emits one structured line per request with status and duration.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.log.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"duration_ms", float64(time.Since(start).Microseconds())/1000.0,
			"request_id", requestID(r),
		)
	})
}

func requestID(r *http.Request) string { return middleware.GetReqID(r.Context()) }

// securityHeaders are sent by the API as well as by nginx, so the headers are
// present even if the API is exposed directly by mistake.
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		// The API returns JSON only; a strict CSP here is defence in depth in case
		// a proxy ever renders something from this origin by accident.
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		if s.secureCookies {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// authenticate resolves the session cookie (or Bearer token) into an actor.
//
// A missing or invalid session is not an error at this layer: the route decides
// whether anonymous access is allowed. That keeps /api/auth/login reachable while
// every other route rejects anonymous callers via requireAuth.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := sessionTokenFrom(r)
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}
		actor, err := s.auth.Authenticate(r.Context(), token)
		if err != nil {
			// Expired, revoked, or disabled account: drop the stale cookie so the
			// browser stops sending it.
			if err == auth.ErrUnauthenticated {
				s.clearSessionCookie(w)
			}
			next.ServeHTTP(w, r)
			return
		}
		ctx := auth.WithActor(r.Context(), actor)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// sessionTokenFrom prefers the cookie and falls back to an Authorization header,
// which is useful for scripts and curl without weakening the browser path.
func sessionTokenFrom(r *http.Request) string {
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	}
	return ""
}

// requireAuth rejects anonymous requests.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, ok := auth.ActorFrom(r.Context())
		if !ok {
			writeError(w, r, s.log, apperror.Unauthorized("Требуется вход в систему"))
			return
		}
		// A forced password change blocks everything except the endpoints needed
		// to perform it, so a temporary password cannot be used indefinitely.
		if actor.MustChangePassword && !passwordChangePath(r.URL.Path) {
			writeError(w, r, s.log, apperror.Forbidden("Сначала смените пароль"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func passwordChangePath(path string) bool {
	switch path {
	case "/api/auth/me", "/api/auth/logout", "/api/auth/password":
		return true
	}
	return false
}

// requireAdmin is the route-level half of the role check; the service layer
// re-checks, so a future route added without this wrapper is still safe.
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, ok := auth.ActorFrom(r.Context())
		if !ok {
			writeError(w, r, s.log, apperror.Unauthorized("Требуется вход в систему"))
			return
		}
		if !actor.IsAdmin() {
			writeError(w, r, s.log, apperror.Forbidden("Недостаточно прав"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireCSRF enforces the double-submit style check for cookie-authenticated
// writes. Requests authenticated by a Bearer header do not need it: a cross-site
// form cannot set that header, which is exactly what CSRF exploits.
func (s *Server) requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}

		if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			next.ServeHTTP(w, r)
			return
		}

		actor, ok := auth.ActorFrom(r.Context())
		if !ok {
			writeError(w, r, s.log, apperror.Unauthorized("Требуется вход в систему"))
			return
		}

		// Defence in depth: modern browsers label the initiator, and a cross-site
		// page cannot claim "same-origin". Absent on older clients, so this only
		// rejects a header that is present and clearly foreign.
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
			s.log.Warn("csrf sec-fetch-site rejected", "sec_fetch_site", site, "path", r.URL.Path)
			writeError(w, r, s.log, apperror.Forbidden("Запрос отклонён проверкой источника"))
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !s.originAllowed(origin, r.Host) {
			s.log.Warn("csrf origin mismatch", "origin", origin, "host", r.Host, "path", r.URL.Path)
			writeError(w, r, s.log, apperror.Forbidden("Запрос отклонён проверкой источника"))
			return
		}

		presented := r.Header.Get("X-CSRF-Token")
		if presented == "" {
			writeError(w, r, s.log, apperror.Forbidden("Отсутствует CSRF-токен"))
			return
		}
		expected, err := s.auth.CSRFToken(r.Context(), actor.SessionID)
		if err != nil || !auth.TokensEqual(presented, expected) {
			writeError(w, r, s.log, apperror.Forbidden("Недействительный CSRF-токен"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// originAllowed decides whether a cross-site form could have produced this
// request.
//
// The check is "does Origin point at *this* server": a browser sets Origin itself
// and a cross-site page cannot forge it to our hostname, so a match with the Host
// we were reached at is the meaningful test. An explicit allowlist is consulted
// first for deployments behind a proxy that rewrites Host.
//
// Earlier versions only accepted IRONNOTE_PUBLIC_ORIGIN, which rejected perfectly
// valid same-origin requests when that variable was unset — the normal case for a
// LAN install.
func (s *Server) originAllowed(origin, host string) bool {
	normalized := strings.TrimRight(strings.TrimSpace(origin), "/")
	if normalized == "" {
		return false
	}

	if s.publicOrigin != "" && strings.EqualFold(normalized, s.publicOrigin) {
		return true
	}
	for _, allowed := range s.corsOrigins {
		if strings.EqualFold(normalized, strings.TrimRight(allowed, "/")) {
			return true
		}
	}

	// Same-origin: compare hostnames, ignoring the port and scheme. nginx's
	// `$host` drops the port, so comparing full authorities would reject
	// http://localhost:8080 against Host "localhost".
	originHost := hostOnly(normalized)
	requestHost := hostOnly(withScheme(normalized, host))
	if originHost == "" || requestHost == "" {
		return false
	}
	return strings.EqualFold(originHost, requestHost)
}

// hostOnly extracts the hostname (no port) from a URL or a bare host:port value.
func hostOnly(value string) string {
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
		return parsed.Hostname()
	}
	if name, _, err := net.SplitHostPort(value); err == nil {
		return name
	}
	return value
}

// withScheme lets a bare Host header go through the same URL parser.
func withScheme(origin, host string) string {
	if host == "" {
		return ""
	}
	scheme := "http"
	if parsed, err := url.Parse(origin); err == nil && parsed.Scheme != "" {
		scheme = parsed.Scheme
	}
	return scheme + "://" + host
}

// --- in-process rate limiting ------------------------------------------------
//
// Login gets its own stricter limiter (plus database-backed lockout); everything
// else gets a generous per-IP budget that only stops abuse, not normal use.

type bucket struct {
	tokens   float64
	lastSeen time.Time
}

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64 // tokens per second
	burst   float64
}

func newRateLimiter(rate, burst float64) *rateLimiter {
	return &rateLimiter{buckets: make(map[string]*bucket), rate: rate, burst: burst}
}

// allow consumes one token if available. The map is swept when it grows, so a
// spray of spoofed source addresses cannot exhaust memory.
func (l *rateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.buckets) > 10_000 {
		for k, b := range l.buckets {
			if now.Sub(b.lastSeen) > 10*time.Minute {
				delete(l.buckets, k)
			}
		}
		// Still too big after sweeping: refuse rather than grow without bound.
		if len(l.buckets) > 20_000 {
			return false
		}
	}

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, lastSeen: now}
		l.buckets[key] = b
	}
	elapsed := now.Sub(b.lastSeen).Seconds()
	if elapsed > 0 {
		b.tokens = minFloat(l.burst, b.tokens+elapsed*l.rate)
	}
	b.lastSeen = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// clientIP resolves the address used for rate limiting.
//
// X-Forwarded-For is honoured only when the immediate peer is a configured trusted
// proxy. Otherwise any client could send a fake header and get a fresh bucket per
// request, which would make the limiter useless.
func (s *Server) clientIP(r *http.Request) string {
	peer := remoteHost(r.RemoteAddr)
	if !s.trustedProxy(peer) {
		return peer
	}
	// Walk right to left and take the first address that is not itself a trusted
	// proxy: that is the closest hop we can vouch for.
	if header := r.Header.Get("X-Forwarded-For"); header != "" {
		parts := strings.Split(header, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			candidate := strings.TrimSpace(parts[i])
			if candidate == "" {
				continue
			}
			if !s.trustedProxy(candidate) {
				return candidate
			}
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	return peer
}

func (s *Server) trustedProxy(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, network := range s.trustedProxies {
		if network.Contains(parsed) {
			return true
		}
	}
	return false
}

func remoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

func (s *Server) rateLimit(limiter *rateLimiter, message string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := s.clientIP(r)
			if !limiter.allow(key, time.Now()) {
				w.Header().Set("Retry-After", strconv.Itoa(30))
				writeError(w, r, s.log, apperror.TooMany(message))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// withTimeout bounds handler work so a slow query cannot pin a connection open.
func withTimeout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
