package scenario

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// shippedCastleTable is the shipped castle data, loaded once per test binary.
var shippedCastleTable struct {
	once  sync.Once
	table *castledata.Table
	err   error
}

// siegeOptions are the boot options a siege line asks for: the shipped
// castles and their sieges.
func siegeOptions(t *testing.T) []gameservertest.Option {
	t.Helper()
	path := datapack.Path(t, "data", "xml", "castles.xml")
	shippedCastleTable.once.Do(func() { shippedCastleTable.table, shippedCastleTable.err = gamexml.LoadCastles(path) })
	if shippedCastleTable.err != nil {
		t.Fatalf("load shipped castles: %v", shippedCastleTable.err)
	}
	return []gameservertest.Option{
		gameservertest.WithCastles(shippedCastleTable.table),
		gameservertest.WithSieges(siege.DefaultConfig()),
	}
}

// seedSiege stores the siege line's registration of the clan line's clan,
// before the castles and sieges are restored: a siege_clans row on its
// side, or the castle's ownership.
func seedSiege(t *testing.T, db *sql.DB, s *Siege) {
	t.Helper()
	ctx := context.Background()
	var err error
	if s.Side == "owner" {
		_, err = db.ExecContext(ctx, "UPDATE clan_data SET hasCastle = ? WHERE clan_id = ?", s.Castle, scenarioClan)
	} else {
		_, err = db.ExecContext(ctx, "INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (?, ?, ?)",
			s.Castle, scenarioClan, strings.ToUpper(s.Side))
	}
	if err != nil {
		t.Fatalf("seed siege: %v", err)
	}
}

// checkSiege requires the booted server to have restored the siege line's
// registration as written.
func (r *run) checkSiege(t *testing.T) {
	t.Helper()
	s := r.sc.Siege
	if s == nil {
		return
	}
	sg, ok := r.srv.Sieges.Get(s.Castle)
	if !ok {
		t.Fatalf("%s: siege line: no shipped castle %d", r.sc.Name, s.Castle)
	}
	if got := sg.Side(scenarioClan); !strings.EqualFold(got.String(), s.Side) {
		t.Fatalf("%s: siege line: the clan is restored on side %q of castle %d, want %s", r.sc.Name, got, s.Castle, s.Side)
	}
}
