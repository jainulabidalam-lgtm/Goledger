package http_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	api "goledger/internal/http"
)

func TestSecurityHeaders(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := api.SecureHeadersMiddleware(mux)

	req, _ := http.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("Expected X-Content-Type-Options: nosniff, got %q", rr.Header().Get("X-Content-Type-Options"))
	}
	if rr.Header().Get("Referrer-Policy") != "strict-origin-when-cross-origin" {
		t.Errorf("Expected Referrer-Policy: strict-origin-when-cross-origin, got %q", rr.Header().Get("Referrer-Policy"))
	}
}

func TestStrictJSONDecoding(t *testing.T) {
	srv, mux := setupTestServer(t)
	defer srv.DB.Close()

	tests := []struct {
		name       string
		payload    string
		statusCode int
	}{
		{
			name:       "Unknown field",
			payload:    `{"code":"1000", "name":"Cash", "type":"ASSET", "currency":"USD", "unknown_field": "xyz"}`,
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "Malformed JSON",
			payload:    `{"code":"1000", "name":"Cash", "type":"ASSET", "currency":"USD"`,
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "Trailing JSON",
			payload:    `{"code":"1000", "name":"Cash", "type":"ASSET", "currency":"USD"}{} `,
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "Oversized body",
			payload:    `{"code":"1000", "name":"` + strings.Repeat("a", 1048576) + `", "type":"ASSET", "currency":"USD"}`,
			statusCode: http.StatusBadRequest, // Or RequestEntityTooLarge if we catch that specific error, but our implementation returns 400
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/api/accounts", bytes.NewBufferString(tc.payload))
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tc.statusCode {
				t.Errorf("expected %v, got %v", tc.statusCode, rr.Code)
			}
		})
	}
}

func TestInvalidCurrency(t *testing.T) {
	srv, mux := setupTestServer(t)
	defer srv.DB.Close()

	payload := `{"code":"1000", "name":"Cash", "type":"ASSET", "currency":"GBP"}`
	req, _ := http.NewRequest("POST", "/api/accounts", bytes.NewBufferString(payload))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for invalid currency, got %v", rr.Code)
	}
}

func TestJournalLineDBError(t *testing.T) {
	srv, mux := setupTestServer(t)
	defer srv.DB.Close()

	// Insert a valid journal
	srv.DB.Exec("INSERT INTO journal_entries (id, date, description, status) VALUES (1, '2023-10-01', 'Test', 'POSTED')")
	// Corrupt the schema to force a DB error when querying journal_lines
	srv.DB.Exec("DROP TABLE journal_lines")

	req, _ := http.NewRequest("GET", "/api/journal-entries/1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 Internal Server Error, got %v", rr.Code)
	}
}
