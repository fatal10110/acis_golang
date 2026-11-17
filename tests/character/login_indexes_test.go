package character

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/dbtest"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/link"
)

// The shipped schema keys items and characters by object id only, so the
// per-login reads by owner and by account scan the whole table. The server
// adds an index on each at boot, to a fresh database as to a deployed one.

// leadingIndexes counts the indexes of table that column leads.
func leadingIndexes(t *testing.T, db *sql.DB, table, column string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM information_schema.statistics
		 WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ? AND seq_in_index = 1`,
		table, column).Scan(&n); err != nil {
		t.Fatalf("read indexes of %s: %v", table, err)
	}
	return n
}

// explainAccess returns the access type and key EXPLAIN reports for the
// first table of query.
func explainAccess(t *testing.T, db *sql.DB, query string) (access, key string) {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), "EXPLAIN "+query)
	if err != nil {
		t.Fatalf("explain %q: %v", query, err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("explain columns: %v", err)
	}
	if !rows.Next() {
		t.Fatalf("explain %q: no plan rows", query)
	}
	vals := make([]sql.NullString, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		t.Fatalf("scan explain: %v", err)
	}
	for i, c := range cols {
		switch c {
		case "type":
			access = vals[i].String
		case "key":
			key = vals[i].String
		}
	}
	return access, key
}

// TestBootIndexesOwnerAndAccount boots a server and checks the login reads
// by owner and by account use their index.
func TestBootIndexesOwnerAndAccount(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t)
	for _, ix := range []struct{ table, column string }{{"items", "owner_id"}, {"characters", "account_name"}} {
		if n := leadingIndexes(t, srv.DB, ix.table, ix.column); n != 1 {
			t.Fatalf("%s(%s) leads %d indexes after boot, want 1", ix.table, ix.column, n)
		}
	}
	owner := srv.SeedCharacterFor(t, "indexed", "Indexed", 1, 0).ID
	for _, q := range []struct{ query, key string }{
		{fmt.Sprintf("SELECT i.object_id FROM items i LEFT JOIN augmentations a ON a.item_oid = i.object_id WHERE i.owner_id = %d ORDER BY i.loc_data", owner), "owner_id"},
		{fmt.Sprintf("SELECT obj_Id FROM characters WHERE account_name = '%s' ORDER BY obj_Id ASC", "indexed"), "account_name"},
	} {
		if access, key := explainAccess(t, srv.DB, q.query); access != "ref" || key != q.key {
			t.Fatalf("EXPLAIN %s = type %q key %q, want ref on %s", q.query, access, key, q.key)
		}
	}
}

// TestEnsureIndexesOnDeployedSchema starts from a database carrying the
// shipped schema, with neither index, and adds them; a second run, as at the
// next boot, changes nothing.
func TestEnsureIndexesOnDeployedSchema(t *testing.T) {
	t.Parallel()
	db := sqltest.NewDB(t)
	for _, ix := range []struct{ table, column string }{{"items", "owner_id"}, {"characters", "account_name"}} {
		if n := leadingIndexes(t, db, ix.table, ix.column); n != 0 {
			t.Fatalf("shipped schema: %s(%s) leads %d indexes, want none", ix.table, ix.column, n)
		}
	}
	for run := range 2 {
		if err := gamesql.EnsureIndexes(context.Background(), db); err != nil {
			t.Fatalf("run %d: %v", run+1, err)
		}
		for _, ix := range []struct{ table, column string }{{"items", "owner_id"}, {"characters", "account_name"}} {
			if n := leadingIndexes(t, db, ix.table, ix.column); n != 1 {
				t.Fatalf("run %d: %s(%s) leads %d indexes, want 1", run+1, ix.table, ix.column, n)
			}
		}
	}
}

// TestSelectionReusesCharacterList pins that entering the world reads the
// account's characters once, at the character list: the selection takes the
// account's other characters from that list. The first account list read
// after the selection is made to fail; it is the restart's, which goes out
// empty.
func TestSelectionReusesCharacterList(t *testing.T) {
	t.Parallel()
	const account = "onelist"
	fault := &dbtest.Fault{}
	srv := gameservertest.Boot(t, gameservertest.WithDBFault(fault))
	srv.SeedCharacterFor(t, account, "Picked", 1, 0)
	srv.SeedCharacterFor(t, account, "Other", 1, 0)
	key := link.SessionKey{LoginKey1: 1, LoginKey2: 2, PlayKey1: 3, PlayKey2: 4}
	srv.IssueLoginSession(account, key)
	c := srv.DialAuthLogin(t, account, key)
	if slots := readCharSelectSlots(t, c, "login"); len(slots) != 2 {
		t.Fatalf("login list = %+v, want 2 slots", slots)
	}

	fault.FailNth("FROM characters WHERE account_name = ? ORDER BY obj_Id", 1)
	c.Send(encodeRequestGameStart(0))
	readUntil(t, c, serverpackets.OpcodeCharSelected)
	c.Send(encodeEnterWorld())
	readUntil(t, c, serverpackets.OpcodeActionFailed)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntil(t, c, serverpackets.OpcodeRestartResponse)
	if slots := readCharSelectSlots(t, c, "restart"); len(slots) != 0 {
		t.Fatalf("restart list = %+v, want it empty: the selection read the account list again", slots)
	}
}
