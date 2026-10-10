package http

import (
	"net/http"
	"time"

	"goledger/internal/ledger"
)

type CreateJournalRequest struct {
	Date        string               `json:"date"`
	Description string               `json:"description"`
	Lines       []ledger.JournalLine `json:"lines"`
}

type JournalResponse struct {
	ID          int64                 `json:"id"`
	Date        string                `json:"date"`
	Description string                `json:"description"`
	Status      string                `json:"status"`
	Lines       []JournalLineResponse `json:"lines"`
}

type JournalLineResponse struct {
	ID           int64 `json:"id"`
	AccountID    int64 `json:"account_id"`
	DebitAmount  int64 `json:"debit_amount"`
	CreditAmount int64 `json:"credit_amount"`
}

func (s *Server) HandleJournals(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listJournals(w, r)
	case http.MethodPost:
		s.createJournal(w, r)
	default:
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) createJournal(w http.ResponseWriter, r *http.Request) {
	var req CreateJournalRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		return
	}

	if req.Date == "" {
		req.Date = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", req.Date); err != nil {
		respondError(w, http.StatusBadRequest, "invalid date format, expected YYYY-MM-DD")
		return
	}

	entry := ledger.JournalEntry{
		Date:        req.Date,
		Description: req.Description,
		Lines:       req.Lines,
	}

	// The PostEntry function does all the heavy validation and transaction processing.
	if err := ledger.PostEntry(r.Context(), s.DB, entry); err != nil {
		// Map domain errors to HTTP statuses
		switch err {
		case ledger.ErrInvalidLinesCount, ledger.ErrUnbalancedEntry, ledger.ErrInvalidAmounts, ledger.ErrInvalidCurrency:
			respondError(w, http.StatusBadRequest, err.Error())
		case ledger.ErrAccountNotFound:
			respondError(w, http.StatusNotFound, err.Error())
		default:
			respondError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	respondJSON(w, http.StatusCreated, map[string]string{"status": "posted"})
}

func (s *Server) listJournals(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), "SELECT id, date, description, status FROM journal_entries ORDER BY date DESC, id DESC")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer rows.Close()

	var entries []JournalResponse
	for rows.Next() {
		var j JournalResponse
		if err := rows.Scan(&j.ID, &j.Date, &j.Description, &j.Status); err != nil {
			respondError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		entries = append(entries, j)
	}

	if entries == nil {
		entries = []JournalResponse{}
	}
	respondJSON(w, http.StatusOK, entries)
}

func (s *Server) HandleGetJournal(w http.ResponseWriter, r *http.Request, idStr string) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var j JournalResponse
	err := s.DB.QueryRowContext(r.Context(), "SELECT id, date, description, status FROM journal_entries WHERE id = ?", idStr).
		Scan(&j.ID, &j.Date, &j.Description, &j.Status)
	if err != nil {
		respondError(w, http.StatusNotFound, "journal entry not found")
		return
	}

	rows, err := s.DB.QueryContext(r.Context(), "SELECT id, account_id, debit_amount, credit_amount FROM journal_lines WHERE journal_entry_id = ?", j.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var l JournalLineResponse
		if err := rows.Scan(&l.ID, &l.AccountID, &l.DebitAmount, &l.CreditAmount); err != nil {
			respondError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		j.Lines = append(j.Lines, l)
	}

	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if j.Lines == nil {
		j.Lines = []JournalLineResponse{}
	}
	respondJSON(w, http.StatusOK, j)
}
