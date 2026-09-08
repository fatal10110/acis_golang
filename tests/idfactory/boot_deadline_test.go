package idfactory

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/rs/zerolog"
)

// TestBootCleanupStopsOnExpiredContextInsteadOfMaskingIt drives New with an
// already-expired context. Before this test, cleanup ran every remaining
// orphan-cleanup/repair statement anyway (each one failing independently
// with its own "couldn't cleanup"/"couldn't repair" warn), burying the real
// cause under one line per statement and leaving loadUsedIDs's own
// context-deadline error as the only visible symptom of a boot-timeout hit.
// cleanup must instead stop at the first stage boundary it checks and log
// once that the boot deadline, not a bad statement, is why nothing ran.
func TestBootCleanupStopsOnExpiredContextInsteadOfMaskingIt(t *testing.T) {
	db := sqltest.SharedDB(t)

	for _, stmt := range idScanTables {
		seedRow(t, db, stmt)
	}
	clearIDScanRows(t, db)

	// Orphaned by an item row that no longer exists; would be deleted by
	// orphanCleanupStatements if the pass ever reached that statement.
	seedRow(t, db,
		"INSERT INTO augmentations (item_oid, attributes, skill_id, skill_level) VALUES (57391, 1, 1, 1)")

	var buf bytes.Buffer
	log := zerolog.New(&buf)

	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	<-ctx.Done()

	if _, err := idfactory.New(ctx, db, log); err == nil {
		t.Fatal("New() error = nil, want context deadline exceeded")
	}

	logged := buf.String()
	if strings.Count(logged, "couldn't cleanup database row orphans") > 0 {
		t.Fatalf("cleanup ran orphan statements against an already-expired context:\n%s", logged)
	}
	if strings.Count(logged, "boot deadline hit") != 1 {
		t.Fatalf("want exactly one boot-deadline warn, got log:\n%s", logged)
	}

	var augmentations int
	if err := db.QueryRow("SELECT COUNT(*) FROM augmentations WHERE item_oid NOT IN (SELECT object_id FROM items)").Scan(&augmentations); err != nil {
		t.Fatalf("count orphan augmentations: %v", err)
	}
	if augmentations != 1 {
		t.Fatalf("orphan augmentation row was cleaned despite the expired context: count = %d, want 1", augmentations)
	}
}
