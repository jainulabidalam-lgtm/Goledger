package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

type AuditLog struct {
	ActorID    int
	Action     string
	EntityType string
	EntityID   int64
	Details    string
}

// Log writes an append-only audit record to the database.
// This function expects a *sql.Tx or *sql.DB and executes within it.
func Log(ctx context.Context, db interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}, actorID int, action, entityType string, entityID int64, details interface{}) error {
	var detailsStr string
	if details != nil {
		b, err := json.Marshal(details)
		if err != nil {
			return fmt.Errorf("failed to marshal audit details: %w", err)
		}
		detailsStr = string(b)
	}

	_, err := db.ExecContext(ctx,
		"INSERT INTO audit_logs (actor_id, action, entity_type, entity_id, details) VALUES (?, ?, ?, ?, ?)",
		actorID, action, entityType, entityID, detailsStr,
	)
	if err != nil {
		return fmt.Errorf("failed to insert audit log: %w", err)
	}

	return nil
}
