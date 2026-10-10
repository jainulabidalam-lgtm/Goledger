package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"goledger/internal/audit"
)

var (
	ErrInvalidLinesCount = errors.New("journal entry must have at least two lines")
	ErrUnbalancedEntry   = errors.New("total debits must equal total credits")
	ErrInvalidAmounts    = errors.New("each line must have a positive debit OR credit, not both")
	ErrInvalidCurrency   = errors.New("inconsistent currencies across accounts")
	ErrAccountNotFound   = errors.New("account not found")
)

type JournalEntry struct {
	Date        string
	Description string
	Lines       []JournalLine
}

type JournalLine struct {
	AccountID    int64 `json:"account_id"`
	DebitAmount  int64 `json:"debit_amount"`
	CreditAmount int64 `json:"credit_amount"`
}

// PostEntry validates and atomically persists a journal entry.
func PostEntry(ctx context.Context, db *sql.DB, entry JournalEntry, actorID int) error {
	if len(entry.Lines) < 2 {
		return ErrInvalidLinesCount
	}

	var totalDebit, totalCredit int64
	currencyMap := make(map[string]bool)

	for _, line := range entry.Lines {
		if (line.DebitAmount > 0 && line.CreditAmount > 0) || (line.DebitAmount <= 0 && line.CreditAmount <= 0) {
			return ErrInvalidAmounts
		}
		totalDebit += line.DebitAmount
		totalCredit += line.CreditAmount

		var currency string
		err := db.QueryRowContext(ctx, "SELECT currency FROM accounts WHERE id = ?", line.AccountID).Scan(&currency)
		if err != nil {
			if err == sql.ErrNoRows {
				return ErrAccountNotFound
			}
			return fmt.Errorf("failed to lookup account currency: %w", err)
		}
		currencyMap[currency] = true
	}

	if totalDebit != totalCredit {
		return ErrUnbalancedEntry
	}

	if len(currencyMap) > 1 {
		return ErrInvalidCurrency
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Safe to call even if committed

	res, err := tx.ExecContext(ctx,
		"INSERT INTO journal_entries (date, description, status) VALUES (?, ?, ?)",
		entry.Date, entry.Description, "POSTED",
	)
	if err != nil {
		return fmt.Errorf("failed to insert journal entry: %w", err)
	}

	entryID, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get journal entry ID: %w", err)
	}

	for _, line := range entry.Lines {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO journal_lines (journal_entry_id, account_id, debit_amount, credit_amount) VALUES (?, ?, ?, ?)",
			entryID, line.AccountID, line.DebitAmount, line.CreditAmount,
		)
		if err != nil {
			return fmt.Errorf("failed to insert journal line: %w", err)
		}
	}

	if err := audit.Log(ctx, tx, actorID, "CREATE", "JOURNAL_ENTRY", entryID, entry); err != nil {
		return fmt.Errorf("failed to audit log journal posting: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// ReverseEntry creates a new journal entry that reverses the given entry.
func ReverseEntry(ctx context.Context, db *sql.DB, originalEntryID int64, actorID int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var date string
	var description string
	var status string
	err = tx.QueryRowContext(ctx, "SELECT date, description, status FROM journal_entries WHERE id = ?", originalEntryID).
		Scan(&date, &description, &status)
	if err != nil {
		if err == sql.ErrNoRows {
			return errors.New("original journal entry not found")
		}
		return fmt.Errorf("failed to lookup original journal entry: %w", err)
	}

	if status != "POSTED" {
		return errors.New("cannot reverse an entry that is not POSTED")
	}

	// Read original lines
	rows, err := tx.QueryContext(ctx, "SELECT account_id, debit_amount, credit_amount FROM journal_lines WHERE journal_entry_id = ?", originalEntryID)
	if err != nil {
		return fmt.Errorf("failed to read journal lines: %w", err)
	}
	defer rows.Close()

	var lines []JournalLine
	for rows.Next() {
		var l JournalLine
		if err := rows.Scan(&l.AccountID, &l.DebitAmount, &l.CreditAmount); err != nil {
			return fmt.Errorf("failed to scan journal line: %w", err)
		}
		// Swap debits and credits
		lines = append(lines, JournalLine{
			AccountID:    l.AccountID,
			DebitAmount:  l.CreditAmount,
			CreditAmount: l.DebitAmount,
		})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows error: %w", err)
	}

	if len(lines) < 2 {
		return errors.New("original entry has invalid lines count")
	}

	newDescription := fmt.Sprintf("%s (Reversal of #%d)", description, originalEntryID)

	res, err := tx.ExecContext(ctx,
		"INSERT INTO journal_entries (date, description, status) VALUES (DATE('now'), ?, ?)",
		newDescription, "POSTED",
	)
	if err != nil {
		return fmt.Errorf("failed to insert reversing journal entry: %w", err)
	}

	newEntryID, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get reversing journal entry ID: %w", err)
	}

	for _, line := range lines {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO journal_lines (journal_entry_id, account_id, debit_amount, credit_amount) VALUES (?, ?, ?, ?)",
			newEntryID, line.AccountID, line.DebitAmount, line.CreditAmount,
		)
		if err != nil {
			return fmt.Errorf("failed to insert reversing journal line: %w", err)
		}
	}

	// Mark original as reversed
	_, err = tx.ExecContext(ctx, "UPDATE journal_entries SET status = 'REVERSED' WHERE id = ?", originalEntryID)
	if err != nil {
		return fmt.Errorf("failed to update original journal entry status: %w", err)
	}

	// Audit Log
	auditDetails := map[string]interface{}{
		"original_id": originalEntryID,
		"reversal_id": newEntryID,
		"description": newDescription,
	}
	if err := audit.Log(ctx, tx, actorID, "REVERSE", "JOURNAL_ENTRY", originalEntryID, auditDetails); err != nil {
		return fmt.Errorf("failed to audit log reversal: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit reversal transaction: %w", err)
	}

	return nil
}
