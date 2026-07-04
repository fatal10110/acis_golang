//go:build integration

package sql

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go/modules/mariadb"
)

// accountsSchema mirrors aCis_datapack/sql/accounts.sql verbatim.
const accountsSchema = "CREATE TABLE IF NOT EXISTS `accounts` (\n" +
	"	`login` VARCHAR(45) NOT NULL DEFAULT '',\n" +
	"	`password` VARCHAR(60) NOT NULL DEFAULT '',\n" +
	"	`last_active` BIGINT NOT NULL DEFAULT 0,\n" +
	"	`access_level` INT(3) NOT NULL DEFAULT 0,\n" +
	"	`last_server` INT(4) NOT NULL DEFAULT 1,\n" +
	"	PRIMARY KEY (`login`)\n" +
	")"

// newIntegrationStore starts a real MariaDB container, creates the accounts
// table, and returns an AccountStore backed by it.
func newIntegrationStore(t *testing.T) *AccountStore {
	t.Helper()
	ctx := context.Background()

	container, err := mariadb.Run(ctx, "mariadb:11")
	if err != nil {
		t.Fatalf("start mariadb container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("terminate mariadb container: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.ExecContext(ctx, accountsSchema); err != nil {
		t.Fatalf("create accounts table: %v", err)
	}
	return NewAccountStore(db)
}

func TestAccountStore_Account_NotFound(t *testing.T) {
	store := newIntegrationStore(t)

	_, err := store.Account("ghost")
	if !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("Account() error = %v, want ErrAccountNotFound", err)
	}
}

func TestAccountStore_CreateAndReadBack(t *testing.T) {
	store := newIntegrationStore(t)

	createdAt := time.UnixMilli(1_700_000_000_000)
	created, err := store.CreateAccount("player1", "hashedpw", createdAt)
	if err != nil {
		t.Fatalf("CreateAccount() unexpected error: %v", err)
	}
	if created.Login != "player1" || created.Password != "hashedpw" || created.AccessLevel != 0 || created.LastServer != 1 {
		t.Fatalf("CreateAccount() = %+v, want login=player1 password=hashedpw accessLevel=0 lastServer=1", created)
	}

	got, err := store.Account("player1")
	if err != nil {
		t.Fatalf("Account() unexpected error: %v", err)
	}
	if got != created {
		t.Fatalf("Account() after create = %+v, want %+v", got, created)
	}
}

func TestAccountStore_CreateAccount_DuplicateLoginFails(t *testing.T) {
	store := newIntegrationStore(t)

	createdAt := time.UnixMilli(1_700_000_000_000)
	if _, err := store.CreateAccount("dupe", "hash1", createdAt); err != nil {
		t.Fatalf("first CreateAccount() unexpected error: %v", err)
	}
	if _, err := store.CreateAccount("dupe", "hash2", createdAt); err == nil {
		t.Fatal("second CreateAccount() with same login: want error (primary key violation), got nil")
	}
}

func TestAccountStore_SetLastActive(t *testing.T) {
	store := newIntegrationStore(t)
	createdAt := time.UnixMilli(1_700_000_000_000)
	if _, err := store.CreateAccount("player1", "hash", createdAt); err != nil {
		t.Fatalf("CreateAccount() unexpected error: %v", err)
	}

	updatedAt := time.UnixMilli(1_800_000_000_000)
	if err := store.SetLastActive("player1", updatedAt); err != nil {
		t.Fatalf("SetLastActive() unexpected error: %v", err)
	}

	var lastActive int64
	if err := store.db.QueryRow("SELECT last_active FROM accounts WHERE login = ?", "player1").Scan(&lastActive); err != nil {
		t.Fatalf("verify last_active: %v", err)
	}
	if lastActive != updatedAt.UnixMilli() {
		t.Errorf("last_active = %d, want %d", lastActive, updatedAt.UnixMilli())
	}
}

func TestAccountStore_SetAccessLevel(t *testing.T) {
	store := newIntegrationStore(t)
	createdAt := time.UnixMilli(1_700_000_000_000)
	if _, err := store.CreateAccount("player1", "hash", createdAt); err != nil {
		t.Fatalf("CreateAccount() unexpected error: %v", err)
	}

	if err := store.SetAccessLevel("player1", -1); err != nil {
		t.Fatalf("SetAccessLevel() unexpected error: %v", err)
	}

	got, err := store.Account("player1")
	if err != nil {
		t.Fatalf("Account() unexpected error: %v", err)
	}
	if got.AccessLevel != -1 {
		t.Errorf("AccessLevel = %d, want -1", got.AccessLevel)
	}
}

func TestAccountStore_SetLastServer(t *testing.T) {
	store := newIntegrationStore(t)
	createdAt := time.UnixMilli(1_700_000_000_000)
	if _, err := store.CreateAccount("player1", "hash", createdAt); err != nil {
		t.Fatalf("CreateAccount() unexpected error: %v", err)
	}

	if err := store.SetLastServer("player1", 3); err != nil {
		t.Fatalf("SetLastServer() unexpected error: %v", err)
	}

	got, err := store.Account("player1")
	if err != nil {
		t.Fatalf("Account() unexpected error: %v", err)
	}
	if got.LastServer != 3 {
		t.Errorf("LastServer = %d, want 3", got.LastServer)
	}
}
