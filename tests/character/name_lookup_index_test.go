package character

import (
	"context"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Character names and account names are looked up through their column's
// own case-folding comparison, which the index the server adds at boot
// serves, rather than through LOWER(), which no index serves.

// TestNameLookupsUseIndexOnLargeTable fills a database carrying the shipped
// schema and the boot indexes with 100,000 characters, then checks the
// lookups by name and by account read one index entry instead of the table.
func TestNameLookupsUseIndexOnLargeTable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := sqltest.NewDB(t)
	if n := leadingIndexes(t, db, "characters", "char_name"); n != 0 {
		t.Fatalf("shipped schema: characters(char_name) leads %d indexes, want none", n)
	}
	for run := range 2 {
		if err := gamesql.EnsureIndexes(ctx, db); err != nil {
			t.Fatalf("run %d: %v", run+1, err)
		}
		if n := leadingIndexes(t, db, "characters", "char_name"); n != 1 {
			t.Fatalf("run %d: characters(char_name) leads %d indexes, want 1", run+1, n)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO characters (account_name, obj_Id, char_name)
		SELECT CONCAT('acct', seq DIV 7), 268435456 + seq, CONCAT('Bulk', seq) FROM seq_1_to_100000`); err != nil {
		t.Fatalf("fill characters: %v", err)
	}
	if _, err := db.ExecContext(ctx, "ANALYZE TABLE characters"); err != nil {
		t.Fatalf("analyze characters: %v", err)
	}
	for _, q := range []struct{ query, key string }{
		{"SELECT COUNT(*) FROM characters WHERE char_name = 'bULK77777'", "char_name"},
		{"SELECT obj_Id, char_name, COALESCE(accesslevel,0) FROM characters WHERE char_name = 'bULK77777' ORDER BY obj_Id", "char_name"},
		{"SELECT COUNT(*) FROM characters WHERE account_name = 'ACCT777'", "account_name"},
	} {
		if access, key := explainAccess(t, db, q.query); (access != "ref" && access != "const") || key != q.key {
			t.Fatalf("EXPLAIN %s = type %q key %q, want ref on %s", q.query, access, key, q.key)
		}
	}

	store := gamesql.NewCharacterStore(db)
	if taken, err := store.NameTaken(ctx, "bULK77777"); err != nil || !taken {
		t.Fatalf("NameTaken(bULK77777) = %v, %v; want taken by Bulk77777", taken, err)
	}
	if ref, ok, err := store.FindByName(ctx, "bULK77777"); err != nil || !ok || ref.Name != "Bulk77777" {
		t.Fatalf("FindByName(bULK77777) = %+v, %v, %v; want Bulk77777", ref, ok, err)
	}
	if n, err := store.CountByAccount(ctx, "ACCT777"); err != nil || n != 7 {
		t.Fatalf("CountByAccount(ACCT777) = %d, %v; want 7", n, err)
	}
}

// TestCreateRefusesNameDifferingOnlyInCase has a second account create the
// name a stored character carries in other letter cases: the creation is
// refused as a name in use, and only the stored character holds it.
func TestCreateRefusesNameDifferingOnlyInCase(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t)
	srv.SeedCharacterFor(t, "holder", "Casefold", 1, 0)
	c := srv.DialClient(t, "newcomer", 0)
	for _, name := range []string{"casefold", "CASEFOLD", "cASEfOLD"} {
		c.Send(encodeRequestCharacterCreate(name, 0, 0, 0, 0, 0, 0))
		if got := readCreateReply(t, c, name); got != int32(serverpackets.CharCreateFailReasonNameAlreadyExists) {
			t.Fatalf("create %s: reply = %d, want name in use", name, got)
		}
	}
	if n := nameRows(t, srv, "Casefold"); n != 1 {
		t.Fatalf("%d rows hold the name, want 1", n)
	}
}
