package character

import (
	"context"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
)

// TestClanLinkClearsUseIndex starts from the shipped schema, which indexes
// neither characters(apprentice) nor characters(sponsor), and adds both at
// boot; a second run changes nothing. A departing clan member's links are
// then cleared through the index, locking only the rows that name it
// rather than every character row (#5504).
func TestClanLinkClearsUseIndex(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := sqltest.NewDB(t)
	columns := []string{"apprentice", "sponsor"}
	for _, col := range columns {
		if n := leadingIndexes(t, db, "characters", col); n != 0 {
			t.Fatalf("shipped schema: characters(%s) leads %d indexes, want none", col, n)
		}
	}
	for run := range 2 {
		if err := gamesql.EnsureIndexes(ctx, db); err != nil {
			t.Fatalf("run %d: %v", run+1, err)
		}
		for _, col := range columns {
			if n := leadingIndexes(t, db, "characters", col); n != 1 {
				t.Fatalf("run %d: characters(%s) leads %d indexes, want 1", run+1, col, n)
			}
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO characters (account_name, obj_Id, char_name)
		SELECT CONCAT('acct', seq), 268435456 + seq, CONCAT('Link', seq) FROM seq_1_to_2000`); err != nil {
		t.Fatalf("fill characters: %v", err)
	}
	if _, err := db.ExecContext(ctx, "ANALYZE TABLE characters"); err != nil {
		t.Fatalf("analyze characters: %v", err)
	}
	for _, col := range columns {
		query := "UPDATE characters SET " + col + "=0 WHERE " + col + "=268436000"
		if access, key := explainAccess(t, db, query); (access != "range" && access != "ref") || key != col {
			t.Fatalf("EXPLAIN %s = type %q key %q, want a lookup on %s", query, access, key, col)
		}
	}
}
