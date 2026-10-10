package http

import (
	"net/http"
	"strings"
)

type Account struct {
	ID       int64  `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Currency string `json:"currency"`
	Status   string `json:"status"`
}

type CreateAccountRequest struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Currency string `json:"currency"`
}

func (s *Server) HandleAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listAccounts(w, r)
	case http.MethodPost:
		s.createAccount(w, r)
	default:
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), "SELECT id, code, name, type, currency, status FROM accounts ORDER BY code ASC")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer rows.Close()

	var accounts []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.Code, &a.Name, &a.Type, &a.Currency, &a.Status); err != nil {
			respondError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		accounts = append(accounts, a)
	}

	if accounts == nil {
		accounts = []Account{} // return empty array instead of null
	}
	respondJSON(w, http.StatusOK, accounts)
}

func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var req CreateAccountRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		return
	}

	req.Code = strings.TrimSpace(req.Code)
	req.Name = strings.TrimSpace(req.Name)
	req.Type = strings.ToUpper(strings.TrimSpace(req.Type))
	req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))

	if req.Code == "" || req.Name == "" || req.Type == "" || req.Currency == "" {
		respondError(w, http.StatusBadRequest, "missing required fields")
		return
	}

	validTypes := map[string]bool{"ASSET": true, "LIABILITY": true, "EQUITY": true, "INCOME": true, "EXPENSE": true}
	if !validTypes[req.Type] {
		respondError(w, http.StatusBadRequest, "invalid account type")
		return
	}

	validCurrencies := map[string]bool{"INR": true, "USD": true, "EUR": true}
	if !validCurrencies[req.Currency] {
		respondError(w, http.StatusBadRequest, "invalid currency")
		return
	}

	// Insert into DB
	res, err := s.DB.ExecContext(r.Context(),
		"INSERT INTO accounts (code, name, type, currency, status) VALUES (?, ?, ?, ?, ?)",
		req.Code, req.Name, req.Type, req.Currency, "ACTIVE",
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			respondError(w, http.StatusConflict, "account code already exists")
			return
		}
		respondError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	id, _ := res.LastInsertId()

	acc := Account{
		ID:       id,
		Code:     req.Code,
		Name:     req.Name,
		Type:     req.Type,
		Currency: req.Currency,
		Status:   "ACTIVE",
	}
	respondJSON(w, http.StatusCreated, acc)
}
