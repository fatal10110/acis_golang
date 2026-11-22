package main

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/fatal10110/acis_golang/internal/loginserver/data/sql"
)

// TestAccountCollationEqualNameWorksOnStoredLogin drives every
// single-account option with a spelling the accounts table's
// accent-insensitive collation matches to player1 (#5046): each acts on
// the stored row and reports it under its stored login, and no second row
// appears.
func TestAccountCollationEqualNameWorksOnStoredLogin(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	const variant = "plàyer1"

	runScript(t, store, "1 player1 s3cret 0 5")

	out := runScript(t, store, "1 "+variant+" n3w 3 5")
	if !strings.Contains(out, "Account player1 has been created or updated") {
		t.Fatalf("update via %q not reported under the stored login:\n%s", variant, out)
	}
	accounts, err := store.ListAccounts(ctx, sql.AllAccounts)
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(accounts) != 1 || accounts[0].Login != "player1" {
		t.Fatalf("accounts after update = %+v, want only player1", accounts)
	}
	acc, err := store.Account(ctx, "player1")
	if err != nil {
		t.Fatalf("Account(player1): %v", err)
	}
	if acc.AccessLevel != 3 {
		t.Errorf("AccessLevel = %d, want 3", acc.AccessLevel)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(acc.Password), []byte("n3w")); err != nil {
		t.Errorf("stored password is not the updated one: %v", err)
	}

	out = runScript(t, store, "2 "+variant+" 1 5")
	if !strings.Contains(out, "Account player1 has been updated") {
		t.Fatalf("level change via %q not reported under the stored login:\n%s", variant, out)
	}

	out = runScript(t, store, "3 "+variant+" y 5")
	if !strings.Contains(out, "Account player1 has been deleted") {
		t.Fatalf("delete via %q not reported under the stored login:\n%s", variant, out)
	}
	if _, err := store.Account(ctx, "player1"); err == nil {
		t.Fatal("player1 still stored after delete")
	}
}
