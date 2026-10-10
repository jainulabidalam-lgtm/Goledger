package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
func PostEntry(ctx context.Context, db *sql.DB, entry JournalEntry) error {
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

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
