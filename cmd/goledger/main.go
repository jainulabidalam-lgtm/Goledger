package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"

	"goledger/internal/auth"
	"goledger/internal/config"
	"goledger/internal/database"
	api "goledger/internal/http"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	logger.Info("Initializing database...", "path", cfg.DBPath)
	db, err := database.InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()
	logger.Info("Database initialized successfully.")

	// Admin Provisioning
	adminUser := os.Getenv("PROVISION_ADMIN_USER")
	adminPass := os.Getenv("PROVISION_ADMIN_PASS")
	if adminUser != "" && adminPass != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(adminPass), 14)
		if err != nil {
			logger.Error("Failed to hash admin password", "error", err)
		} else {
			_, err = db.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)", adminUser, string(hash), auth.RoleAdmin)
			if err != nil {
				if strings.Contains(err.Error(), "UNIQUE constraint failed") {
					logger.Warn("Admin user already exists. Not overwriting credentials.", "username", adminUser)
				} else {
					logger.Error("Failed to provision admin", "error", err)
				}
			} else {
				logger.Info("Admin provisioned successfully", "username", adminUser)
			}
		}
		// Clear environment variables so they aren't exposed in subprocesses or memory dumps easily
		os.Setenv("PROVISION_ADMIN_USER", "")
		os.Setenv("PROVISION_ADMIN_PASS", "")
	}

	srv := &api.Server{
		DB:           db,
		TrustedProxy: cfg.TrustedProxy,
	}
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Wrap with logging and security middleware
	handler := api.LoggingMiddleware(logger)(api.SecureHeadersMiddleware(mux))

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Channel to listen for errors coming from the listener.
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("Starting GoLedger", "addr", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	// Channel to listen for signals.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		log.Fatalf("Server failed to start: %v", err)
	case sig := <-shutdown:
		logger.Info("Starting shutdown...", "signal", sig)

		// Create a context with a timeout for the graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			logger.Error("Graceful shutdown did not complete in time", "error", err)
			if err := server.Close(); err != nil {
				logger.Error("Could not stop http server", "error", err)
			}
		}
	}
	logger.Info("Server stopped gracefully")
}
