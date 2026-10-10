package http

import (
	"context"
	"net"
	"net/http"
	"time"

	"goledger/internal/auth"
)

type contextKey string

const userContextKey contextKey = "user"
const sessionCookieName = "session_id"
const csrfCookieName = "csrf_token"

func (s *Server) isSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if s.TrustedProxy != "" && r.Header.Get("X-Forwarded-Proto") == "https" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil && (s.TrustedProxy == "*" || host == s.TrustedProxy) {
			return true
		}
	}
	return false
}

func (s *Server) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := decodeJSONBody(w, r, &req); err != nil {
		return
	}

	user, err := auth.Authenticate(s.DB, req.Username, req.Password)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Prevent Session Fixation by invalidating any existing session before creating a new one
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_ = auth.DeleteSession(s.DB, cookie.Value)
	}

	token, err := auth.CreateSession(s.DB, user.ID, 24*time.Hour)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	csrfToken, err := auth.GenerateCSRFToken()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate csrf token")
		return
	}

	isSecure := s.isSecure(r)

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    csrfToken,
		Path:     "/",
		HttpOnly: false, // Javascript needs to read this to attach to headers
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "logged in",
		"username": user.Username,
		"role":     user.Role,
	})
}

func (s *Server) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		_ = auth.DeleteSession(s.DB, cookie.Value)
	}

	isSecure := s.isSecure(r)

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: false,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})

	respondJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

func (s *Server) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			respondError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		user, err := auth.ValidateSession(s.DB, cookie.Value)
		if err != nil {
			respondError(w, http.StatusUnauthorized, "invalid or expired session")
			return
		}

		// CSRF check for state-changing requests
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete || r.Method == http.MethodPatch {
			csrfCookie, err := r.Cookie(csrfCookieName)
			if err != nil {
				respondError(w, http.StatusForbidden, "missing csrf cookie")
				return
			}

			headerToken := r.Header.Get("X-CSRF-Token")
			if !auth.VerifyCSRFToken(csrfCookie.Value, headerToken) {
				respondError(w, http.StatusForbidden, "invalid csrf token")
				return
			}
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RoleMiddleware(roles ...auth.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := r.Context().Value(userContextKey).(*auth.User)
			if !ok || user == nil {
				respondError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			hasRole := false
			for _, role := range roles {
				if user.Role == role {
					hasRole = true
					break
				}
			}

			if !hasRole {
				respondError(w, http.StatusForbidden, "insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ViewerMiddleware prevents viewers from making state-changing requests on routes they otherwise have access to.
func ViewerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := r.Context().Value(userContextKey).(*auth.User)
		if ok && user != nil && user.Role == auth.RoleViewer {
			if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
				respondError(w, http.StatusForbidden, "viewers cannot modify data")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
