package idfactory_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/db"
	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
)

// Round-trip persistence tests against a real MariaDB, covering the
// acceptance criteria that pure unit tests can't: reading ids already used
// across the schema's tables, and reclaiming ids on restart/reload once
// their row is gone. Skipped unless ACIS_TEST_DATABASE_URL is set (see
// CONTRIBUTING/CI: no database service runs on the default `go test ./...`
// path), e.g.:
//
//	ACIS_TEST_DATABASE_URL="jdbc:mariadb://localhost:33061/acis_test" \
//	ACIS_TEST_DATABASE_LOGIN=root ACIS_TEST_DATABASE_PASSWORD=test \
//	go test ./internal/commons/idfactory/...
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("ACIS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ACIS_TEST_DATABASE_URL not set; skipping DB integration test")
	}

	pool, err := db.Open(db.Config{
		URL:      url,
		Login:    os.Getenv("ACIS_TEST_DATABASE_LOGIN"),
		Password: os.Getenv("ACIS_TEST_DATABASE_PASSWORD"),
	})
	if err != nil {
		t.Fatalf("db.Open() error: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for _, stmt := range []string{
		"DELETE FROM characters", "DELETE FROM items", "DELETE FROM clan_data",
		"DELETE FROM items_on_ground", "DELETE FROM mods_wedding", "DELETE FROM petition",
	} {
		if _, err := pool.Exec(stmt); err != nil {
			t.Fatalf("cleanup %q: %v", stmt, err)
		}
	}
	return pool
}

func TestNew_SkipsIDsAlreadyUsedAcrossAllTables(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()

	inserts := []struct {
		stmt string
		id   int32
	}{
		{"INSERT INTO characters (obj_Id, char_name) VALUES (?, 'x')", idfactory.FirstObjectID},
		{"INSERT INTO items (object_id, item_id) VALUES (?, 1)", idfactory.FirstObjectID + 1},
		{"INSERT INTO clan_data (clan_id) VALUES (?)", idfactory.FirstObjectID + 2},
		{"INSERT INTO items_on_ground (object_id) VALUES (?)", idfactory.FirstObjectID + 3},
		{"INSERT INTO mods_wedding (id) VALUES (?)", idfactory.FirstObjectID + 4},
		{"INSERT INTO petition (oid, type, content, state, rate, feedback, responders) VALUES (?, 't', 'c', 's', 'r', 'f', '')", idfactory.FirstObjectID + 5},
	}
	for _, ins := range inserts {
		if _, err := pool.ExecContext(ctx, ins.stmt, ins.id); err != nil {
			t.Fatalf("seed %q: %v", ins.stmt, err)
		}
	}

	alloc, err := idfactory.New(ctx, pool, nil)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	got, err := alloc.NextID()
	if err != nil {
		t.Fatalf("NextID() unexpected error: %v", err)
	}
	want := int32(idfactory.FirstObjectID + 6)
	if got != want {
		t.Fatalf("NextID() = %d, want %d (ids 0-5 already used across characters/items/clan_data/items_on_ground/mods_wedding/petition)", got, want)
	}
}

func TestNew_ReclaimsIDOnRestartAfterRowDeleted(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()

	id := int32(idfactory.FirstObjectID + 10)
	if _, err := pool.ExecContext(ctx, "INSERT INTO characters (obj_Id, char_name) VALUES (?, 'x')", id); err != nil {
		t.Fatalf("seed: %v", err)
	}

	first, err := idfactory.New(ctx, pool, nil)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	for i := 0; i < 11; i++ {
		got, err := first.NextID()
		if err != nil {
			t.Fatalf("NextID() unexpected error: %v", err)
		}
		if got == id {
			t.Fatalf("NextID() returned %d while its row still exists", id)
		}
	}

	if _, err := pool.ExecContext(ctx, "DELETE FROM characters WHERE obj_Id = ?", id); err != nil {
		t.Fatalf("delete row: %v", err)
	}

	reloaded, err := idfactory.New(ctx, pool, nil)
	if err != nil {
		t.Fatalf("New() on reload error: %v", err)
	}
	for i := 0; i < 11; i++ {
		got, err := reloaded.NextID()
		if err != nil {
			t.Fatalf("NextID() unexpected error: %v", err)
		}
		if got == id {
			return // id reclaimed after its row was deleted and the factory reloaded
		}
	}
	t.Fatalf("id %d was never reclaimed after its row was deleted and New() reloaded", id)
}
