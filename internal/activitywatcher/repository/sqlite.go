package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver; no CGO required

	"github.com/simple-container-com/api/internal/activitywatcher/model"
)

const schema = `
CREATE TABLE IF NOT EXISTS events (
    id          TEXT     PRIMARY KEY,
    user_id     TEXT     NOT NULL,
    event_type  TEXT     NOT NULL,
    occurred_at DATETIME NOT NULL,
    metadata    TEXT     NOT NULL DEFAULT '{}',
    created_at  DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_events_user_occurred
    ON events (user_id, occurred_at DESC);
PRAGMA journal_mode=WAL;
`

// SQLiteRepository is an EventRepository backed by SQLite.
type SQLiteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository opens (or creates) a SQLite database at dbPath and
// applies the schema migration.
func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Single writer; WAL allows concurrent readers.
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(schema); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &SQLiteRepository{db: db}, nil
}

// Close releases the underlying database connection.
func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

// Create persists e and fills e.CreatedAt from the DB default.
func (r *SQLiteRepository) Create(ctx context.Context, e *model.Event) error {
	meta, err := json.Marshal(e.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO events (id, user_id, event_type, occurred_at, metadata)
		 VALUES (?, ?, ?, ?, ?)`,
		e.ID,
		e.UserID,
		e.EventType,
		e.OccurredAt.UTC().Format(time.RFC3339Nano),
		string(meta),
	)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	row := r.db.QueryRowContext(ctx, `SELECT created_at FROM events WHERE id = ?`, e.ID)
	var createdAt string
	if err = row.Scan(&createdAt); err != nil {
		return fmt.Errorf("fetch created_at: %w", err)
	}
	e.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		// Fallback: SQLite default format may differ; try without nano.
		e.CreatedAt, err = time.Parse("2006-01-02T15:04:05Z", createdAt)
		if err != nil {
			e.CreatedAt = time.Now().UTC()
		}
	}
	return nil
}

// ListByUser returns all events for userID, newest first.
func (r *SQLiteRepository) ListByUser(ctx context.Context, userID string) ([]*model.Event, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, event_type, occurred_at, metadata, created_at
		 FROM events WHERE user_id = ? ORDER BY occurred_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []*model.Event
	for rows.Next() {
		var (
			e                        model.Event
			occurredAtStr, createdAt string
			metaStr                  string
		)
		if err = rows.Scan(&e.ID, &e.UserID, &e.EventType, &occurredAtStr, &metaStr, &createdAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		e.OccurredAt, _ = time.Parse(time.RFC3339Nano, occurredAtStr)
		if e.OccurredAt.IsZero() {
			e.OccurredAt, _ = time.Parse("2006-01-02T15:04:05Z", occurredAtStr)
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		if e.CreatedAt.IsZero() {
			e.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", createdAt)
		}
		e.Metadata = json.RawMessage(metaStr)
		events = append(events, &e)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return events, nil
}
