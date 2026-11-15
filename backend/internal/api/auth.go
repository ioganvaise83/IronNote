package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ioganvaise83/ironnote/internal/apperror"
	"github.com/ioganvaise83/ironnote/internal/auth"
)

const sessionCookieName = "ironnote_session"

// setSessionCookie issues the session cookie.
//
// HttpOnly keeps it away from JavaScript (so an XSS cannot exfiltrate the
// session); SameSite=Lax blocks cross-site POSTs while still allowing a link into
// the app to keep you signed in; Secure is set whenever TLS is terminated in front
// of us.
func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type userResponse struct {
	ID                 int64   `json:"id"`
	Username           string  `json:"username"`
	DisplayName        string  `json:"displayName"`
	Role               string  `json:"role"`
	IsActive           bool    `json:"isActive"`
	MustChangePassword bool    `json:"mustChangePassword"`
	LastLoginAt        *string `json:"lastLoginAt"`
	CreatedAt          string  `json:"createdAt"`
	UpdatedAt          string  `json:"updatedAt"`
}

func userJSON(u auth.User) userResponse {
	return userResponse{
		ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, Role: string(u.Role),
		IsActive: u.IsActive, MustChangePassword: u.MustChangePassword,
		LastLoginAt: timeJSONPtr(u.LastLoginAt),
		CreatedAt:   timeJSON(u.CreatedAt), UpdatedAt: timeJSON(u.UpdatedAt),
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	if len(req.Username) > 64 || len(req.Password) > 256 {
		// Reject absurd payloads before touching the database or hashing.
		writeError(w, r, s.log, apperror.Unauthorized("Неверный логин или пароль"))
		return
	}

	result, err := s.auth.Login(r.Context(), req.Username, req.Password,
		s.clientIP(r), r.UserAgent())
	if err != nil {
		writeError(w, r, s.log, loginError(err))
		return
	}

	s.setSessionCookie(w, result.Token, result.ExpiresAt)
	writeJSON(w, http.StatusOK, map[string]any{
		"user":      userJSON(result.User),
		"csrfToken": result.CSRFToken,
	})
}

// loginError maps auth failures to responses that do not leak account existence.
func loginError(err error) error {
	switch {
	case errors.Is(err, auth.ErrTooManyAttempts):
		return apperror.TooMany("Слишком много попыток входа. Попробуйте позже.")
	case errors.Is(err, auth.ErrAccountDisabled):
		// Only reached after a correct password, so revealing it is safe and useful.
		return apperror.Forbidden("Учётная запись отключена")
	case errors.Is(err, auth.ErrAccountLocked):
		return apperror.Forbidden("Учётная запись временно заблокирована из-за неудачных попыток")
	default:
		// Covers wrong password, unknown user, and anything unexpected: the client
		// cannot tell them apart.
		return apperror.Unauthorized("Неверный логин или пароль")
	}
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if actor, ok := auth.ActorFrom(r.Context()); ok {
		if err := s.auth.Logout(r.Context(), actor.SessionID); err != nil {
			s.log.Warn("logout failed", "error", err)
		}
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorFrom(r.Context())
	if !ok {
		writeError(w, r, s.log, apperror.Unauthorized("Требуется вход в систему"))
		return
	}
	user, err := s.auth.GetUser(r.Context(), actor.UserID)
	if err != nil {
		writeError(w, r, s.log, apperror.Unauthorized("Требуется вход в систему"))
		return
	}
	csrf, err := s.auth.CSRFToken(r.Context(), actor.SessionID)
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":      userJSON(user),
		"csrfToken": csrf,
	})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorFrom(r.Context())
	if !ok {
		writeError(w, r, s.log, apperror.Unauthorized("Требуется вход в систему"))
		return
	}
	var req changePasswordRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	if err := s.auth.ChangeOwnPassword(r.Context(), actor, req.CurrentPassword, req.NewPassword); err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			writeError(w, r, s.log, apperror.Validation("Текущий пароль неверен", map[string]string{
				"currentPassword": "Неверный пароль",
			}))
		default:
			// Policy violations arrive as plain errors with a Russian message.
			writeError(w, r, s.log, apperror.Validation(err.Error(), map[string]string{
				"newPassword": err.Error(),
			}))
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- admin: user management -------------------------------------------------

type createUserRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
	Role        string `json:"role"`
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.auth.ListUsers(r.Context())
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	out := make([]userResponse, 0, len(users))
	for _, u := range users {
		out = append(out, userJSON(u))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.ActorFrom(r.Context())
	var req createUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	user, err := s.auth.CreateUser(r.Context(), actor, auth.CreateUserInput{
		Username:    req.Username,
		DisplayName: req.DisplayName,
		Password:    req.Password,
		Role:        auth.ParseRole(req.Role),
	})
	if err != nil {
		writeError(w, r, s.log, mapAuthAdminError(err))
		return
	}
	writeJSON(w, http.StatusCreated, userJSON(user))
}

type updateUserRequest struct {
	DisplayName *string `json:"displayName"`
	Role        *string `json:"role"`
	IsActive    *bool   `json:"isActive"`
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.ActorFrom(r.Context())
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	var req updateUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	in := auth.UpdateUserInput{DisplayName: req.DisplayName, IsActive: req.IsActive}
	if req.Role != nil {
		role := auth.ParseRole(*req.Role)
		in.Role = &role
	}
	user, err := s.auth.UpdateUser(r.Context(), actor, id, in)
	if err != nil {
		writeError(w, r, s.log, mapAuthAdminError(err))
		return
	}
	writeJSON(w, http.StatusOK, userJSON(user))
}

type resetPasswordRequest struct {
	NewPassword string `json:"newPassword"`
}

func (s *Server) handleResetUserPassword(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.ActorFrom(r.Context())
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	var req resetPasswordRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	if err := s.auth.ResetPassword(r.Context(), actor, id, req.NewPassword); err != nil {
		writeError(w, r, s.log, mapAuthAdminError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.ActorFrom(r.Context())
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	if err := s.auth.DeleteUser(r.Context(), actor, id); err != nil {
		writeError(w, r, s.log, mapAuthAdminError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// mapAuthAdminError turns service-level errors into HTTP errors without ever
// exposing SQL text. Rule messages written for humans pass through.
func mapAuthAdminError(err error) error {
	switch {
	case errors.Is(err, auth.ErrForbidden):
		return apperror.Forbidden("Недостаточно прав")
	case errors.Is(err, auth.ErrLastAdmin):
		return apperror.Conflict("Нельзя оставить приложение без администратора")
	default:
		message := err.Error()
		// Policy and "user not found" messages are user-facing by construction;
		// anything that looks like a driver error is replaced.
		if looksInternal(message) {
			return apperror.Internal(err)
		}
		return apperror.Validation(message, nil)
	}
}

func looksInternal(message string) bool {
	lower := strings.ToLower(message)
	for _, marker := range []string{"sql", "pgx", "connection", "row", "scan", "constraint"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
