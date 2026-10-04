package gameservertest

import (
	"context"
	"database/sql"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/rs/zerolog"
)

// WithCastles loads castles as the castle data; without it the server has
// no castle.
func WithCastles(castles *castledata.Table) Option {
	return func(o *options) { o.castles = castles }
}

// newCastles builds the castles of data over the clans of clans, written
// through worker.
func newCastles(db *sql.DB, data *castledata.Table, clans *clan.Service, worker *persist.Worker, log zerolog.Logger) *castle.Manager {
	return castle.NewManager(data, clans.Table(), gamesql.NewCastleStore(db), worker, log)
}

// restoreCastles restores the stored castle rows and owners, as the boot
// path does once the clans are restored.
func restoreCastles(t *testing.T, db *sql.DB, castles *castle.Manager) {
	t.Helper()
	store := gamesql.NewCastleStore(db)
	rows, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	owners, err := store.LoadOwners(context.Background())
	if err != nil {
		t.Fatalf("load castle owners: %v", err)
	}
	castles.Restore(rows, owners)
}
