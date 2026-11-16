package sql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// serverCrashMemo is the server_memo variable that is "true" while a server
// session runs and "false" once it shut down cleanly.
const serverCrashMemo = "server_crash"

// ServerCrashStore keeps the server_memo flag that tells a booting server
// whether the previous session ended without a clean shutdown.
type ServerCrashStore struct {
	db *sql.DB
}

// NewServerCrashStore returns a ServerCrashStore backed by db.
func NewServerCrashStore(db *sql.DB) *ServerCrashStore {
	return &ServerCrashStore{db: db}
}

// MarkRunning reports whether the previous session crashed, its flag still
// "true" (case-insensitive), and otherwise sets the flag for this session. A
// missing flag reads as no crash. After a crash the flag is already set and
// is left as it is.
func (s *ServerCrashStore) MarkRunning(ctx context.Context) (crashed bool, err error) {
	var value string
	switch err := s.db.QueryRowContext(ctx, "SELECT value FROM server_memo WHERE var = ?", serverCrashMemo).Scan(&value); {
	case err == sql.ErrNoRows:
	case err != nil:
		return false, fmt.Errorf("load server crash flag: %w", err)
	}
	if strings.EqualFold(value, "true") {
		return true, nil
	}
	return false, s.set(ctx, true)
}

// MarkStopped clears the flag, recording that this session shut down
// cleanly.
func (s *ServerCrashStore) MarkStopped(ctx context.Context) error {
	return s.set(ctx, false)
}

func (s *ServerCrashStore) set(ctx context.Context, crashed bool) error {
	value := "false"
	if crashed {
		value = "true"
	}
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO server_memo (var, value) VALUES (?, ?) ON DUPLICATE KEY UPDATE value = VALUES(value)",
		serverCrashMemo, value,
	); err != nil {
		return fmt.Errorf("save server crash flag: %w", err)
	}
	return nil
}
