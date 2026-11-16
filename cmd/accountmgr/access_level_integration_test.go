package main

import (
	"context"
	"strings"
	"testing"
)

// Re-applying an account's current access level still reports the account
// as updated: the database counts the row the update matched.
func TestChangeAccessLevelToSameLevelReportsUpdated(t *testing.T) {
	store := newIntegrationStore(t)
	runScript(t, store, "1 player1 s3cret 5 5")

	for range 2 {
		out := runScript(t, store, "2 player1 5 5")
		if !strings.Contains(out, "Account player1 has been updated") {
			t.Fatalf("same-level change not reported as updated:\n%s", out)
		}
	}
	out := runScript(t, store, "2 ghost 1 5")
	if !strings.Contains(out, "Account ghost doesn't exist") {
		t.Fatalf("change level on unknown account not reported:\n%s", out)
	}

	// Re-creating with identical values is reported the same way.
	out = runScript(t, store, "1 player1 s3cret 5 5")
	if !strings.Contains(out, "Account player1 has been created or updated") {
		t.Fatalf("re-create not reported as created or updated:\n%s", out)
	}
}

// A non-numeric access level reaches the database, whose strict sql_mode
// rejects it: the tool prints the database error and leaves the row as is.
func TestNonNumericAccessLevelPrintsDatabaseError(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	runScript(t, store, "1 player1 s3cret 5 5")

	out := runScript(t, store, "2 player1 abc 5")
	if !strings.Contains(out, "There was error while updating account:\n") || !strings.Contains(out, "Incorrect integer value") {
		t.Fatalf("non-numeric change level did not print the database error:\n%s", out)
	}
	acc, err := store.Account(ctx, "player1")
	if err != nil {
		t.Fatalf("Account(player1): %v", err)
	}
	if acc.AccessLevel != 5 {
		t.Fatalf("AccessLevel after rejected change = %d, want 5", acc.AccessLevel)
	}

	out = runScript(t, store, "1 p2 pw abc 5")
	if !strings.Contains(out, "There was error while adding/updating account:\n") || !strings.Contains(out, "Incorrect integer value") {
		t.Fatalf("non-numeric create did not print the database error:\n%s", out)
	}
	if _, err := store.Account(ctx, "p2"); err == nil {
		t.Fatal("account p2 created despite the rejected access level")
	}
}
