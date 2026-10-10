package http

import (
	"net/http"
)

type LedgerResponse struct {
	Account Account       `json:"account"`
	Entries []LedgerEntry `json:"entries"`
	Balance int64         `json:"balance"` // Current running balance
}

type LedgerEntry struct {
	JournalLineID  int64  `json:"journal_line_id"`
	JournalEntryID int64  `json:"journal_entry_id"`
	Date           string `json:"date"`
	Description    string `json:"description"`
	DebitAmount    int64  `json:"debit_amount"`
	CreditAmount   int64  `json:"credit_amount"`
	RunningBalance int64  `json:"running_balance"`
}

func (s *Server) HandleAccountLedger(w http.ResponseWriter, r *http.Request, idStr string) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var acc Account
	err := s.DB.QueryRowContext(r.Context(), "SELECT id, code, name, type, currency, status FROM accounts WHERE id = ?", idStr).
		Scan(&acc.ID, &acc.Code, &acc.Name, &acc.Type, &acc.Currency, &acc.Status)
	if err != nil {
		respondError(w, http.StatusNotFound, "account not found")
		return
	}

	rows, err := s.DB.QueryContext(r.Context(), `
		SELECT l.id, e.id, e.date, e.description, l.debit_amount, l.credit_amount 
		FROM journal_lines l
		JOIN journal_entries e ON l.journal_entry_id = e.id
		WHERE l.account_id = ?
		ORDER BY e.date ASC, e.id ASC, l.id ASC
	`, acc.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer rows.Close()

	var entries []LedgerEntry
	var currentBalance int64 = 0

	// Determine normal balance based on account type
	// ASSET, EXPENSE -> Debit Normal (Balance = sum(Debits) - sum(Credits))
	// LIABILITY, EQUITY, INCOME -> Credit Normal (Balance = sum(Credits) - sum(Debits))
	isDebitNormal := acc.Type == "ASSET" || acc.Type == "EXPENSE"

	for rows.Next() {
		var entry LedgerEntry
		if err := rows.Scan(&entry.JournalLineID, &entry.JournalEntryID, &entry.Date, &entry.Description, &entry.DebitAmount, &entry.CreditAmount); err != nil {
			respondError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		if isDebitNormal {
			currentBalance += entry.DebitAmount - entry.CreditAmount
		} else {
			currentBalance += entry.CreditAmount - entry.DebitAmount
		}

		entry.RunningBalance = currentBalance
		entries = append(entries, entry)
	}

	if entries == nil {
		entries = []LedgerEntry{}
	}

	res := LedgerResponse{
		Account: acc,
		Entries: entries,
		Balance: currentBalance,
	}

	respondJSON(w, http.StatusOK, res)
}
