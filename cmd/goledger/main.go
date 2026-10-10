package main

import (
	"encoding/json"
	"log"
	"net/http"

	"goledger/internal/database"
	api "goledger/internal/http"
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func main() {
	log.Println("Initializing database...")
	db, err := database.InitDB("goledger.db")
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()
	log.Println("Database initialized successfully.")

	srv := &api.Server{DB: db}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	srv.RegisterRoutes(mux)

	log.Println("Starting GoLedger on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
