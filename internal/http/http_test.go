package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"goledger/internal/database"
	api "goledger/internal/http"
	"goledger/internal/ledger"
)

func setupTestServer(t *testing.T) (*api.Server, *http.ServeMux) {
	db, err := database.InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Failed to initialize test DB: %v", err)
	}

	srv := &api.Server{DB: db}
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	return srv, mux
}

func TestAccountEndpoints(t *testing.T) {
	srv, mux := setupTestServer(t)
	defer srv.DB.Close()

	// 1. Create valid account
	payload := []byte(`{"code":"1000", "name":"Cash", "type":"ASSET", "currency":"USD"}`)
	req, _ := http.NewRequest("POST", "/api/accounts", bytes.NewBuffer(payload))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201 Created, got %v", rr.Code)
	}

	// 2. Invalid account type
	payload = []byte(`{"code":"1001", "name":"Invalid", "type":"UNKNOWN", "currency":"USD"}`)
	req, _ = http.NewRequest("POST", "/api/accounts", bytes.NewBuffer(payload))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %v", rr.Code)
	}

	// 3. Duplicate code
	payload = []byte(`{"code":"1000", "name":"Duplicate", "type":"ASSET", "currency":"USD"}`)
	req, _ = http.NewRequest("POST", "/api/accounts", bytes.NewBuffer(payload))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409 Conflict, got %v", rr.Code)
	}

	// 4. List accounts
	req, _ = http.NewRequest("GET", "/api/accounts", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %v", rr.Code)
	}
	var accounts []api.Account
	json.NewDecoder(rr.Body).Decode(&accounts)
	if len(accounts) != 1 || accounts[0].Code != "1000" {
		t.Errorf("expected 1 account with code 1000, got %v", accounts)
	}
}

func TestJournalEndpoints(t *testing.T) {
	srv, mux := setupTestServer(t)
	defer srv.DB.Close()

	// Create test accounts
	srv.DB.Exec("INSERT INTO accounts (id, code, name, type, currency, status) VALUES (1, '1000', 'Cash', 'ASSET', 'USD', 'ACTIVE')")
	srv.DB.Exec("INSERT INTO accounts (id, code, name, type, currency, status) VALUES (2, '4000', 'Revenue', 'INCOME', 'USD', 'ACTIVE')")

	// 1. Post balanced journal using raw JSON to explicitly verify JSON tags decoding
	rawJSONBalanced := []byte(`{
		"date": "2023-10-01",
		"description": "Initial Deposit raw JSON",
		"lines": [
			{"account_id": 1, "debit_amount": 500, "credit_amount": 0},
			{"account_id": 2, "debit_amount": 0, "credit_amount": 500}
		]
	}`)
	req, _ := http.NewRequest("POST", "/api/journal-entries", bytes.NewBuffer(rawJSONBalanced))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201 Created for raw JSON, got %v: %s", rr.Code, rr.Body.String())
	}

	// 2. Post balanced journal using struct marshaling
	jReq := api.CreateJournalRequest{
		Date:        "2023-10-02",
		Description: "Second Deposit",
		Lines: []ledger.JournalLine{
			{AccountID: 1, DebitAmount: 300, CreditAmount: 0},
			{AccountID: 2, DebitAmount: 0, CreditAmount: 300},
		},
	}
	body, _ := json.Marshal(jReq)
	req, _ = http.NewRequest("POST", "/api/journal-entries", bytes.NewBuffer(body))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201 Created, got %v: %s", rr.Code, rr.Body.String())
	}

	// 3. Unbalanced entry using raw JSON
	rawJSONUnbalanced := []byte(`{
		"date": "2023-10-03",
		"description": "Unbalanced raw JSON",
		"lines": [
			{"account_id": 1, "debit_amount": 500, "credit_amount": 0},
			{"account_id": 2, "debit_amount": 0, "credit_amount": 400}
		]
	}`)
	req, _ = http.NewRequest("POST", "/api/journal-entries", bytes.NewBuffer(rawJSONUnbalanced))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for unbalanced raw JSON, got %v", rr.Code)
	}

	// 4. Nonexistent account using struct
	jReq.Lines[1].AccountID = 999
	body, _ = json.Marshal(jReq)
	req, _ = http.NewRequest("POST", "/api/journal-entries", bytes.NewBuffer(body))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found, got %v", rr.Code)
	}
}

func TestAccountLedgerEndpoint(t *testing.T) {
	srv, mux := setupTestServer(t)
	defer srv.DB.Close()

	srv.DB.Exec("INSERT INTO accounts (id, code, name, type, currency, status) VALUES (1, '1000', 'Cash', 'ASSET', 'USD', 'ACTIVE')")
	srv.DB.Exec("INSERT INTO journal_entries (id, date, description, status) VALUES (1, '2023-10-01', 'Test', 'POSTED')")
	srv.DB.Exec("INSERT INTO journal_lines (journal_entry_id, account_id, debit_amount, credit_amount) VALUES (1, 1, 1000, 0)")
	srv.DB.Exec("INSERT INTO journal_entries (id, date, description, status) VALUES (2, '2023-10-02', 'Test 2', 'POSTED')")
	srv.DB.Exec("INSERT INTO journal_lines (journal_entry_id, account_id, debit_amount, credit_amount) VALUES (2, 1, 0, 400)")

	req, _ := http.NewRequest("GET", "/api/accounts/1/ledger", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %v", rr.Code)
	}

	var res api.LedgerResponse
	json.NewDecoder(rr.Body).Decode(&res)
	if res.Balance != 600 { // 1000 debit - 400 credit on ASSET account = 600
		t.Errorf("expected balance 600, got %d", res.Balance)
	}
	if len(res.Entries) != 2 {
		t.Errorf("expected 2 ledger entries, got %d", len(res.Entries))
	}
}

func TestStaticFilesAndAPI(t *testing.T) {
	srv, mux := setupTestServer(t)
	defer srv.DB.Close()

	tests := []struct {
		name         string
		path         string
		expectedCode int
	}{
		{"Root returns index.html", "/", http.StatusOK},
		{"CSS returns style.css", "/css/style.css", http.StatusOK},
		{"JS returns app.js", "/js/app.js", http.StatusOK},
		{"API accounts returns JSON", "/api/accounts", http.StatusOK},
		{"Non-existent returns 404", "/nonexistent", http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tc.path, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			if rr.Code != tc.expectedCode {
				t.Errorf("expected %d for %s, got %d", tc.expectedCode, tc.path, rr.Code)
			}
		})
	}
}
