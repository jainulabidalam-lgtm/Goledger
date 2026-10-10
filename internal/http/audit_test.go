package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	api "goledger/internal/http"
)

func TestReversalAndAudit(t *testing.T) {
	srv, mux := setupTestServer(t)
	defer srv.DB.Close()

	// 1. Create Accounts
	payload1 := []byte(`{"code":"A100", "name":"Asset", "type":"ASSET", "currency":"USD"}`)
	req, _ := http.NewRequest("POST", "/api/accounts", bytes.NewBuffer(payload1))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("Failed to create account A100: %d", rr.Code)
	}
	var accAsset api.Account
	json.NewDecoder(rr.Body).Decode(&accAsset)

	payload2 := []byte(`{"code":"I100", "name":"Income", "type":"INCOME", "currency":"USD"}`)
	req, _ = http.NewRequest("POST", "/api/accounts", bytes.NewBuffer(payload2))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("Failed to create account I100: %d", rr.Code)
	}
	var accIncome api.Account
	json.NewDecoder(rr.Body).Decode(&accIncome)

	// 2. Post a Journal
	journalBody := `{
		"date": "2026-10-10",
		"description": "Initial Sale",
		"lines": [
			{"account_id": ` + strconv.FormatInt(accAsset.ID, 10) + `, "debit_amount": 500, "credit_amount": 0},
			{"account_id": ` + strconv.FormatInt(accIncome.ID, 10) + `, "debit_amount": 0, "credit_amount": 500}
		]
	}`

	req, _ = http.NewRequest("POST", "/api/journal-entries", bytes.NewBuffer([]byte(journalBody)))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("Failed to create journal: %d, %s", rr.Code, rr.Body.String())
	}

	// Fetch all journals and pick the first one.
	req, _ = http.NewRequest("GET", "/api/journal-entries", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	var journals []map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &journals)
	if len(journals) == 0 {
		t.Fatalf("Failed to retrieve journals")
	}
	originalEntryID := int64(journals[0]["id"].(float64))

	// 3. Verify Audit Log contains CREATE ACCOUNT and CREATE JOURNAL
	var count int
	err := srv.DB.QueryRow("SELECT count(*) FROM audit_logs").Scan(&count)
	if err != nil || count == 0 {
		t.Fatalf("Expected audit logs, got %d", count)
	}

	// 4. Reverse Journal
	req, _ = http.NewRequest("POST", "/api/journal-entries/"+strconv.FormatInt(originalEntryID, 10)+"/reverse", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for reversal, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	// 5. Verify Reversal
	req, _ = http.NewRequest("GET", "/api/journal-entries", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	json.Unmarshal(rr.Body.Bytes(), &journals)

	if len(journals) < 2 {
		t.Fatalf("Expected at least 2 journals, got %d", len(journals))
	}

	var originalStatus string
	for _, j := range journals {
		if int64(j["id"].(float64)) == originalEntryID {
			originalStatus = j["status"].(string)
			break
		}
	}
	if originalStatus != "REVERSED" {
		t.Errorf("Expected original journal status to be REVERSED, got %s", originalStatus)
	}

	// 6. Verify Reversal Audit Log
	err = srv.DB.QueryRow("SELECT count(*) FROM audit_logs WHERE action = 'REVERSE' AND entity_type = 'JOURNAL_ENTRY'").Scan(&count)
	if err != nil || count == 0 {
		t.Fatalf("Expected reversal audit log")
	}

	// 7. Prevent Reversing already reversed entry
	req, _ = http.NewRequest("POST", "/api/journal-entries/"+strconv.FormatInt(originalEntryID, 10)+"/reverse", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 when reversing a REVERSED entry, got %d", rr.Code)
	}
}
