// Package sql contains login server database access.
package sql

import (
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
func (s *AccountStore) Account(login string) (model.Account, error) {
	var password string
	var accessLevel, lastServer int

	err := s.db.QueryRow(
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
func (s *AccountStore) CreateAccount(login, hashedPassword string, createdAt time.Time) (model.Account, error) {
	_, err := s.db.Exec(
		"INSERT INTO accounts (login, password, last_active) VALUES (?, ?, ?)",
		login, hashedPassword, createdAt.UnixMilli(),
	)
	if err != nil {
		return model.Account{}, fmt.Errorf("create account %q: %w", login, err)
	}
	return model.NewAccount(login, hashedPassword, 0, 1), nil
}

// SetLastActive updates the account's last-active timestamp.
func (s *AccountStore) SetLastActive(login string, at time.Time) error {
	if _, err := s.db.Exec("UPDATE accounts SET last_active = ? WHERE login = ?", at.UnixMilli(), login); err != nil {
		return fmt.Errorf("set last active for %q: %w", login, err)
	}
	return nil
}

// SetAccessLevel updates the account's access level.
func (s *AccountStore) SetAccessLevel(login string, level int) error {
	if _, err := s.db.Exec("UPDATE accounts SET access_level = ? WHERE login = ?", level, login); err != nil {
		return fmt.Errorf("set access level for %q: %w", login, err)
	}
	return nil
}

// SetLastServer updates the account's last-server id.
func (s *AccountStore) SetLastServer(login string, serverID int) error {
	if _, err := s.db.Exec("UPDATE accounts SET last_server = ? WHERE login = ?", serverID, login); err != nil {
		return fmt.Errorf("set last server for %q: %w", login, err)
	}
	return nil
}
