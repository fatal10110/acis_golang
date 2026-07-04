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

func TestAccountStore_UpsertAccount(t *testing.T) {
	store := newIntegrationStore(t)

	changed, err := store.UpsertAccount("player1", "hash1", 2)
	if err != nil {
		t.Fatalf("UpsertAccount() create unexpected error: %v", err)
	}
	if !changed {
		t.Error("UpsertAccount() create changed = false, want true")
	}
	got, err := store.Account("player1")
	if err != nil {
		t.Fatalf("Account() after create: %v", err)
	}
	if got.Password != "hash1" || got.AccessLevel != 2 {
		t.Fatalf("Account() after create = %+v, want password=hash1 accessLevel=2", got)
	}

	changed, err = store.UpsertAccount("player1", "hash2", 3)
	if err != nil {
		t.Fatalf("UpsertAccount() update unexpected error: %v", err)
	}
	if !changed {
		t.Error("UpsertAccount() update changed = false, want true")
	}
	got, err = store.Account("player1")
	if err != nil {
		t.Fatalf("Account() after update: %v", err)
	}
	if got.Password != "hash2" || got.AccessLevel != 3 {
		t.Fatalf("Account() after update = %+v, want password=hash2 accessLevel=3", got)
	}
}

func TestAccountStore_ChangeAccessLevel(t *testing.T) {
	store := newIntegrationStore(t)
	createdAt := time.UnixMilli(1_700_000_000_000)
	if _, err := store.CreateAccount("player1", "hash", createdAt); err != nil {
		t.Fatalf("CreateAccount() unexpected error: %v", err)
	}

	changed, err := store.ChangeAccessLevel("player1", 4)
	if err != nil {
		t.Fatalf("ChangeAccessLevel() unexpected error: %v", err)
	}
	if !changed {
		t.Error("ChangeAccessLevel() on existing account changed = false, want true")
	}

	changed, err = store.ChangeAccessLevel("ghost", 4)
	if err != nil {
		t.Fatalf("ChangeAccessLevel() on missing account unexpected error: %v", err)
	}
	if changed {
		t.Error("ChangeAccessLevel() on missing account changed = true, want false")
	}
}

func TestAccountStore_DeleteAccount(t *testing.T) {
	store := newIntegrationStore(t)
	createdAt := time.UnixMilli(1_700_000_000_000)
	if _, err := store.CreateAccount("player1", "hash", createdAt); err != nil {
		t.Fatalf("CreateAccount() unexpected error: %v", err)
	}

	deleted, err := store.DeleteAccount("player1")
	if err != nil {
		t.Fatalf("DeleteAccount() unexpected error: %v", err)
	}
	if !deleted {
		t.Error("DeleteAccount() on existing account deleted = false, want true")
	}
	if _, err := store.Account("player1"); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("Account() after delete: got err %v, want ErrAccountNotFound", err)
	}

	deleted, err = store.DeleteAccount("player1")
	if err != nil {
		t.Fatalf("DeleteAccount() second call unexpected error: %v", err)
	}
	if deleted {
		t.Error("DeleteAccount() on missing account deleted = true, want false")
	}
}

func TestAccountStore_ListAccounts(t *testing.T) {
	store := newIntegrationStore(t)
	createdAt := time.UnixMilli(1_700_000_000_000)
	for login, level := range map[string]int{"banned1": -1, "regular1": 0, "gm1": 1} {
		if _, err := store.CreateAccount(login, "hash", createdAt); err != nil {
			t.Fatalf("CreateAccount(%s) unexpected error: %v", login, err)
		}
		if _, err := store.ChangeAccessLevel(login, level); err != nil {
			t.Fatalf("ChangeAccessLevel(%s) unexpected error: %v", login, err)
		}
	}

	tests := []struct {
		filter AccountFilter
		want   []string
	}{
		{AllAccounts, []string{"banned1", "gm1", "regular1"}},
		{BannedAccounts, []string{"banned1"}},
		{PrivilegedAccounts, []string{"gm1"}},
		{RegularAccounts, []string{"regular1"}},
	}
	for _, tt := range tests {
		got, err := store.ListAccounts(tt.filter)
		if err != nil {
			t.Fatalf("ListAccounts(%v) unexpected error: %v", tt.filter, err)
		}
		if len(got) != len(tt.want) {
			t.Fatalf("ListAccounts(%v) = %v, want logins %v", tt.filter, got, tt.want)
		}
		for i, login := range tt.want {
			if got[i].Login != login {
				t.Errorf("ListAccounts(%v)[%d].Login = %q, want %q", tt.filter, i, got[i].Login, login)
			}
		}
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
