// Package sql contains login server database access.
package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/loginserver/model"
)

// ErrAccountNotFound is returned when no accounts row matches the given login.
var ErrAccountNotFound = errors.New("account not found")

// AccountStore reads and writes the accounts table.
type AccountStore struct {
	db *sql.DB
}

// NewAccountStore returns an AccountStore backed by db.
func NewAccountStore(db *sql.DB) *AccountStore {
	return &AccountStore{db: db}
}

// Account returns the account registered under login, or ErrAccountNotFound
// if no such row exists.
func (s *AccountStore) Account(ctx context.Context, login string) (model.Account, error) {
	var password string
	var accessLevel, lastServer int

	err := s.db.QueryRowContext(ctx,
		"SELECT password, access_level, last_server FROM accounts WHERE login = ?",
		login,
	).Scan(&password, &accessLevel, &lastServer)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Account{}, ErrAccountNotFound
	}
	if err != nil {
		return model.Account{}, fmt.Errorf("query account %q: %w", login, err)
	}
	return model.NewAccount(login, password, accessLevel, lastServer), nil
}

// CreateAccount inserts a new account row with the given pre-hashed password
// and creation time, and returns the resulting Account. New accounts start at
// access level 0 and last server 1, matching the table's column defaults.
func (s *AccountStore) CreateAccount(ctx context.Context, login, hashedPassword string, createdAt time.Time) (model.Account, error) {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO accounts (login, password, last_active) VALUES (?, ?, ?)",
		login, hashedPassword, createdAt.UnixMilli(),
	)
	if err != nil {
		return model.Account{}, fmt.Errorf("create account %q: %w", login, err)
	}
	return model.NewAccount(login, hashedPassword, 0, 1), nil
}

// SetLastActive updates the account's last-active timestamp.
func (s *AccountStore) SetLastActive(ctx context.Context, login string, at time.Time) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE accounts SET last_active = ? WHERE login = ?", at.UnixMilli(), login); err != nil {
		return fmt.Errorf("set last active for %q: %w", login, err)
	}
	return nil
}

// SetAccessLevel updates the account's access level.
func (s *AccountStore) SetAccessLevel(ctx context.Context, login string, level int) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE accounts SET access_level = ? WHERE login = ?", level, login); err != nil {
		return fmt.Errorf("set access level for %q: %w", login, err)
	}
	return nil
}

// SetLastServer updates the account's last-server id.
func (s *AccountStore) SetLastServer(ctx context.Context, login string, serverID int) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE accounts SET last_server = ? WHERE login = ?", serverID, login); err != nil {
		return fmt.Errorf("set last server for %q: %w", login, err)
	}
	return nil
}

// UpsertAccount creates the account if login is new, or updates its password
// and access level if it already exists. It reports whether the row was
// created or changed.
func (s *AccountStore) UpsertAccount(ctx context.Context, login, hashedPassword string, accessLevel int) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO accounts (login, password, access_level) VALUES (?, ?, ?) "+
			"ON DUPLICATE KEY UPDATE password = VALUES(password), access_level = VALUES(access_level)",
		login, hashedPassword, accessLevel,
	)
	if err != nil {
		return false, fmt.Errorf("upsert account %q: %w", login, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("upsert account %q: %w", login, err)
	}
	return n > 0, nil
}

// ChangeAccessLevel updates an existing account's access level. It reports
// whether a row was changed.
func (s *AccountStore) ChangeAccessLevel(ctx context.Context, login string, level int) (bool, error) {
	res, err := s.db.ExecContext(ctx, "UPDATE accounts SET access_level = ? WHERE login = ?", level, login)
	if err != nil {
		return false, fmt.Errorf("change access level for %q: %w", login, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("change access level for %q: %w", login, err)
	}
	return n > 0, nil
}

// DeleteAccount removes the account row for login. It reports whether a row
// was deleted.
func (s *AccountStore) DeleteAccount(ctx context.Context, login string) (bool, error) {
	res, err := s.db.ExecContext(ctx, "DELETE FROM accounts WHERE login = ?", login)
	if err != nil {
		return false, fmt.Errorf("delete account %q: %w", login, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete account %q: %w", login, err)
	}
	return n > 0, nil
}

// AccountFilter selects which accounts ListAccounts returns, by access
// level.
type AccountFilter int

const (
	AllAccounts AccountFilter = iota
	BannedAccounts
	PrivilegedAccounts
	RegularAccounts
)

// AccountSummary is one row of a login/access-level listing.
type AccountSummary struct {
	Login       string
	AccessLevel int
}

// ListAccounts returns login/access-level pairs ordered by login, narrowed
// by filter.
func (s *AccountStore) ListAccounts(ctx context.Context, filter AccountFilter) ([]AccountSummary, error) {
	query := "SELECT login, access_level FROM accounts"
	switch filter {
	case BannedAccounts:
		query += " WHERE access_level < 0"
	case PrivilegedAccounts:
		query += " WHERE access_level > 0"
	case RegularAccounts:
		query += " WHERE access_level = 0"
	}
	query += " ORDER BY login ASC"

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()

	var out []AccountSummary
	for rows.Next() {
		var a AccountSummary
		if err := rows.Scan(&a.Login, &a.AccessLevel); err != nil {
			return nil, fmt.Errorf("list accounts: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	return out, nil
}
