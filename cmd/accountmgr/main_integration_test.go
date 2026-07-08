//go:build integration

package main

import (
	"bytes"
	"context"
	dbsql "database/sql"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go/modules/mariadb"

	"golang.org/x/crypto/bcrypt"

	"github.com/fatal10110/acis_golang/internal/loginserver/data/sql"
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

func newIntegrationStore(t *testing.T) *sql.AccountStore {
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

	db, err := dbsql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.ExecContext(ctx, accountsSchema); err != nil {
		t.Fatalf("create accounts table: %v", err)
	}
	return sql.NewAccountStore(db)
}

func runScript(t *testing.T, store *sql.AccountStore, input string) string {
	t.Helper()
	var out bytes.Buffer
	if err := run(context.Background(), strings.NewReader(input), &out, store); err != nil {
		t.Fatalf("run() unexpected error: %v\noutput:\n%s", err, out.String())
	}
	return out.String()
}

func TestAccountLifecycle(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()

	// Create: account row persisted with a bcrypt-hashed password.
	out := runScript(t, store, "1 Player1 s3cret 5 5")
	if !strings.Contains(out, "Account player1 has been created or updated") {
		t.Fatalf("create output missing confirmation:\n%s", out)
	}
	acc, err := store.Account(ctx, "player1")
	if err != nil {
		t.Fatalf("Account(player1) after create: %v", err)
	}
	if acc.AccessLevel != 5 {
		t.Errorf("AccessLevel after create = %d, want 5", acc.AccessLevel)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(acc.Password), []byte("s3cret")); err != nil {
		t.Errorf("stored password does not match: %v", err)
	}

	// Change access level.
	out = runScript(t, store, "2 player1 -1 5")
	if !strings.Contains(out, "Account player1 has been updated") {
		t.Fatalf("change level output missing confirmation:\n%s", out)
	}
	out = runScript(t, store, "2 ghost 1 5")
	if !strings.Contains(out, "Account ghost doesn't exist") {
		t.Fatalf("change level on unknown account not reported:\n%s", out)
	}

	// List: banned mode should show player1 (access level -1) after the change above.
	out = runScript(t, store, "4 1 5")
	if !strings.Contains(out, "player1 -> -1") {
		t.Errorf("banned listing missing player1:\n%s", out)
	}
	if !strings.Contains(out, "Displayed accounts: 1") {
		t.Errorf("banned listing count wrong:\n%s", out)
	}

	// Delete: declined then confirmed.
	out = runScript(t, store, "3 player1 n 5")
	if !strings.Contains(out, "Deletion cancelled.") {
		t.Fatalf("declined delete missing message:\n%s", out)
	}
	out = runScript(t, store, "3 player1 y 5")
	if !strings.Contains(out, "Account player1 has been deleted") {
		t.Fatalf("delete output missing confirmation:\n%s", out)
	}
	out = runScript(t, store, "3 player1 y 5")
	if !strings.Contains(out, "Account player1 doesn't exist") {
		t.Fatalf("delete of missing account not reported:\n%s", out)
	}
}
