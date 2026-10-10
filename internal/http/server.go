package http

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"goledger/internal/auth"
	"goledger/web"
)

type Server struct {
	DB           *sql.DB
	TrustedProxy string
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/auth/login", s.HandleLogin)
	mux.HandleFunc("/api/auth/logout", s.HandleLogout)

	// Protected API routes
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/api/accounts", s.HandleAccounts)
	apiMux.HandleFunc("/api/accounts/", s.HandleAccountSubroutes)
	apiMux.HandleFunc("/api/journal-entries", s.HandleJournals)
	apiMux.HandleFunc("/api/journal-entries/", s.HandleJournalSubroutes)

	// Wrap apiMux with Auth and Role middlewares
	protectedHandler := s.AuthMiddleware(RoleMiddleware(auth.RoleAdmin, auth.RoleAccountant, auth.RoleViewer)(ViewerMiddleware(apiMux)))
	mux.Handle("/api/", protectedHandler)

	mux.HandleFunc("/health/live", s.HandleLive)
	mux.HandleFunc("/health/ready", s.HandleReady)

	fs := http.FileServer(http.FS(web.FS))
	mux.Handle("/", fs)
}

func respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload != nil {
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			log.Printf("Error encoding JSON response: %v", err)
		}
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst interface{}) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1048576) // 1 MiB
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return err
	}

	if err := dec.Decode(&struct{}{}); err != io.EOF {
		respondError(w, http.StatusBadRequest, "body must contain only one JSON value")
		return errors.New("trailing data")
	}
	return nil
}

func SecureHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// Subroute dispatchers
func (s *Server) HandleAccountSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/accounts/")
	parts := strings.Split(path, "/")
	if len(parts) == 2 && parts[1] == "ledger" {
		s.HandleAccountLedger(w, r, parts[0])
		return
	}
	respondError(w, http.StatusNotFound, "not found")
}

func (s *Server) HandleJournalSubroutes(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/journal-entries/")
	if idStr != "" && !strings.Contains(idStr, "/") {
		s.HandleGetJournal(w, r, idStr)
		return
	}
	respondError(w, http.StatusNotFound, "not found")
}

func (s *Server) HandleLive(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) HandleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.DB.PingContext(r.Context()); err != nil {
		respondError(w, http.StatusServiceUnavailable, "database not ready")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok", "database": "ready"})
}

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.status = code
		rw.wroteHeader = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.status = http.StatusOK
		rw.wroteHeader = true
	}
	return rw.ResponseWriter.Write(b)
}

func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{
				ResponseWriter: w,
				status:         http.StatusOK, // Default status if nothing is written
			}
			next.ServeHTTP(rw, r)
			logger.Info("request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rw.status),
				slog.Duration("duration", time.Since(start)),
			)
		})
	}
}
