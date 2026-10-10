package ledger_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"goledger/internal/database"
	"goledger/internal/ledger"
)

func setupTestDB(t *testing.T) *sql.DB {
	// Use an in-memory SQLite database for tests
	db, err := database.InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init test db: %v", err)
	}

	// Create test accounts
	_, err = db.Exec(`
		INSERT INTO accounts (id, code, name, type, currency, status) VALUES 
		(1, '1000', 'Cash', 'ASSET', 'USD', 'ACTIVE'),
		(2, '4000', 'Revenue', 'INCOME', 'USD', 'ACTIVE'),
		(3, '5000', 'EUR Bank', 'ASSET', 'EUR', 'ACTIVE');
	`)
	if err != nil {
		t.Fatalf("failed to seed test db: %v", err)
	}

	return db
}

func TestPostEntry_Valid(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	entry := ledger.JournalEntry{
		Date:        time.Now().Format("2006-01-02"),
		Description: "Sale of services",
		Lines: []ledger.JournalLine{
			{AccountID: 1, DebitAmount: 10000, CreditAmount: 0},
			{AccountID: 2, DebitAmount: 0, CreditAmount: 10000},
		},
	}

	err := ledger.PostEntry(context.Background(), db, entry)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	// Verify it was saved
	var count int
	db.QueryRow("SELECT COUNT(*) FROM journal_lines").Scan(&count)
	if count != 2 {
		t.Errorf("expected 2 journal lines, got %d", count)
	}
}

func TestPostEntry_InvalidLinesCount(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	entry := ledger.JournalEntry{
		Date:        time.Now().Format("2006-01-02"),
		Description: "Invalid entry",
		Lines: []ledger.JournalLine{
			{AccountID: 1, DebitAmount: 10000, CreditAmount: 0},
		},
	}

	err := ledger.PostEntry(context.Background(), db, entry)
	if err != ledger.ErrInvalidLinesCount {
		t.Errorf("expected %v, got %v", ledger.ErrInvalidLinesCount, err)
	}
}

func TestPostEntry_Unbalanced(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	entry := ledger.JournalEntry{
		Date:        time.Now().Format("2006-01-02"),
		Description: "Unbalanced",
		Lines: []ledger.JournalLine{
			{AccountID: 1, DebitAmount: 10000, CreditAmount: 0},
			{AccountID: 2, DebitAmount: 0, CreditAmount: 9000},
		},
	}

	err := ledger.PostEntry(context.Background(), db, entry)
	if err != ledger.ErrUnbalancedEntry {
		t.Errorf("expected %v, got %v", ledger.ErrUnbalancedEntry, err)
	}
}

func TestPostEntry_InvalidAmounts(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	entry := ledger.JournalEntry{
		Date:        time.Now().Format("2006-01-02"),
		Description: "Invalid amounts",
		Lines: []ledger.JournalLine{
			{AccountID: 1, DebitAmount: 10000, CreditAmount: 10000}, // Both set
			{AccountID: 2, DebitAmount: 0, CreditAmount: 0},         // Neither set
		},
	}

	err := ledger.PostEntry(context.Background(), db, entry)
	if err != ledger.ErrInvalidAmounts {
		t.Errorf("expected %v, got %v", ledger.ErrInvalidAmounts, err)
	}
}

func TestPostEntry_InconsistentCurrencies(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	entry := ledger.JournalEntry{
		Date:        time.Now().Format("2006-01-02"),
		Description: "Inconsistent currencies",
		Lines: []ledger.JournalLine{
			{AccountID: 1, DebitAmount: 10000, CreditAmount: 0}, // USD
			{AccountID: 3, DebitAmount: 0, CreditAmount: 10000}, // EUR
		},
	}

	err := ledger.PostEntry(context.Background(), db, entry)
	if err != ledger.ErrInvalidCurrency {
		t.Errorf("expected %v, got %v", ledger.ErrInvalidCurrency, err)
	}
}

func TestPostEntry_AccountNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	entry := ledger.JournalEntry{
		Date:        time.Now().Format("2006-01-02"),
		Description: "Missing account",
		Lines: []ledger.JournalLine{
			{AccountID: 999, DebitAmount: 10000, CreditAmount: 0}, // Does not exist
			{AccountID: 2, DebitAmount: 0, CreditAmount: 10000},
		},
	}

	err := ledger.PostEntry(context.Background(), db, entry)
	if err != ledger.ErrAccountNotFound {
		t.Errorf("expected %v, got %v", ledger.ErrAccountNotFound, err)
	}
}
