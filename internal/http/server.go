package http

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"goledger/web"
)

type Server struct {
	DB *sql.DB
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/accounts", s.HandleAccounts)
	mux.HandleFunc("/api/accounts/", s.HandleAccountSubroutes)
	mux.HandleFunc("/api/journal-entries", s.HandleJournals)
	mux.HandleFunc("/api/journal-entries/", s.HandleJournalSubroutes)

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
