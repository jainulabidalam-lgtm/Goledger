package http_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"goledger/internal/auth"
)

func TestAuthFlow(t *testing.T) {
	srv, mux := setupRawTestServer(t)
	defer srv.DB.Close()

	// Provision test user
	pass, _ := auth.HashPassword("secret")
	srv.DB.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)", "testadmin", pass, auth.RoleAdmin)
	srv.DB.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)", "testviewer", pass, auth.RoleViewer)

	// Test 1: Unauthorized access to API
	req, _ := http.NewRequest("GET", "/api/accounts", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized, got %d", rr.Code)
	}

	// Test 2: Login Failure
	body := `{"username":"testadmin","password":"wrongpassword"}`
	req, _ = http.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized for bad login, got %d", rr.Code)
	}

	// Test 3: Login Success
	body = `{"username":"testadmin","password":"secret"}`
	req, _ = http.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for login, got %d", rr.Code)
	}

	var sessionCookie *http.Cookie
	var csrfCookie *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == "session_id" {
			sessionCookie = c
		}
		if c.Name == "csrf_token" {
			csrfCookie = c
		}
	}

	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatal("Expected session_id cookie")
	}
	if csrfCookie == nil || csrfCookie.Value == "" {
		t.Fatal("Expected csrf_token cookie")
	}

	// Test 4: Access API with Session
	req, _ = http.NewRequest("GET", "/api/accounts", nil)
	req.AddCookie(sessionCookie)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for API access, got %d", rr.Code)
	}

	// Test 5: CSRF Failure on State Change
	req, _ = http.NewRequest("POST", "/api/accounts", bytes.NewBuffer([]byte(`{}`)))
	req.AddCookie(sessionCookie)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden without CSRF, got %d", rr.Code)
	}

	// Test 6: CSRF Success on State Change (with invalid payload resulting in 400, but passing CSRF)
	req, _ = http.NewRequest("POST", "/api/accounts", bytes.NewBuffer([]byte(`{"code":"123","name":"Test","type":"Asset","currency":"USD"}`)))
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code == http.StatusForbidden {
		t.Errorf("Expected CSRF check to pass, but got 403 Forbidden")
	}

	// Test 7: Viewer Role Restriction
	body = `{"username":"testviewer","password":"secret"}`
	req, _ = http.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	for _, c := range rr.Result().Cookies() {
		if c.Name == "session_id" {
			sessionCookie = c
		}
		if c.Name == "csrf_token" {
			csrfCookie = c
		}
	}

	req, _ = http.NewRequest("POST", "/api/accounts", bytes.NewBuffer([]byte(`{"code":"124","name":"Test2","type":"Asset","currency":"USD"}`)))
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden for Viewer role attempting POST, got %d", rr.Code)
	}

	// Test 8: Logout
	req, _ = http.NewRequest("POST", "/api/auth/logout", nil)
	req.AddCookie(sessionCookie)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for logout, got %d", rr.Code)
	}

	for _, c := range rr.Result().Cookies() {
		if c.Name == "session_id" {
			if c.Value != "" || c.MaxAge != -1 {
				t.Errorf("Expected cleared session cookie")
			}
		}
	}

	// Test 9: Access after logout
	req, _ = http.NewRequest("GET", "/api/accounts", nil)
	req.AddCookie(sessionCookie)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 after logout, got %d", rr.Code)
	}
}

func TestSessionExpiry(t *testing.T) {
	srv, mux := setupRawTestServer(t)
	defer srv.DB.Close()

	pass, _ := auth.HashPassword("secret")
	srv.DB.Exec("INSERT INTO users (id, username, password_hash, role) VALUES (1, 'expiretest', ?, 'viewer')", pass)

	// Insert an expired session
	token, hash, _ := auth.GenerateSessionToken()
	srv.DB.Exec("INSERT INTO sessions (user_id, token_hash, expires_at) VALUES (1, ?, datetime('now', '-1 hour'))", hash)

	req, _ := http.NewRequest("GET", "/api/accounts", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: token})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized for expired session, got %d", rr.Code)
	}

	// Verify session was deleted
	var count int
	srv.DB.QueryRow("SELECT COUNT(*) FROM sessions WHERE token_hash = ?", hash).Scan(&count)
	if count != 0 {
		t.Errorf("Expected expired session to be deleted, but it remains")
	}
}

func TestSessionRotation(t *testing.T) {
	srv, mux := setupRawTestServer(t)
	defer srv.DB.Close()

	pass, _ := auth.HashPassword("secret")
	srv.DB.Exec("INSERT INTO users (id, username, password_hash, role) VALUES (1, 'rotateuser', ?, 'admin')", pass)

	token1, hash1, _ := auth.GenerateSessionToken()
	srv.DB.Exec("INSERT INTO sessions (user_id, token_hash, expires_at) VALUES (1, ?, datetime('now', '+1 hour'))", hash1)

	body := `{"username":"rotateuser","password":"secret"}`
	req, _ := http.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "session_id", Value: token1})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for login, got %d", rr.Code)
	}

	// Verify old session was deleted
	var count int
	srv.DB.QueryRow("SELECT COUNT(*) FROM sessions WHERE token_hash = ?", hash1).Scan(&count)
	if count != 0 {
		t.Errorf("Expected old session to be deleted upon new login (fixation prevention), but it remains")
	}
}

func TestSecureCookie(t *testing.T) {
	srv, mux := setupRawTestServer(t)
	defer srv.DB.Close()
	srv.TrustedProxy = "10.0.0.1"

	pass, _ := auth.HashPassword("secret")
	srv.DB.Exec("INSERT INTO users (id, username, password_hash, role) VALUES (1, 'secureuser', ?, 'admin')", pass)

	// Test with untrusted proxy
	body := `{"username":"secureuser","password":"secret"}`
	req, _ := http.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.RemoteAddr = "192.168.1.1:1234"
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	secureCookieMissing := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == "session_id" && c.Secure {
			secureCookieMissing = true
		}
	}
	if secureCookieMissing {
		t.Errorf("Expected cookie not to be secure from untrusted proxy")
	}

	// Test with trusted proxy
	req, _ = http.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-Proto", "https")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	secureCookiePresent := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == "session_id" && c.Secure {
			secureCookiePresent = true
		}
	}
	if !secureCookiePresent {
		t.Errorf("Expected cookie to be secure from trusted proxy")
	}
}
