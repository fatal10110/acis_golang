package gameservertest

import (
	"context"
	"database/sql"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
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

// WithSieges runs the castle sieges with cfg over the WithCastles castles,
// each turning on the WithZones siege zone of its castle; without it the
// server runs no siege.
func WithSieges(cfg siege.Config) Option {
	return func(o *options) { o.sieges = &cfg }
}

// newSieges builds the castle sieges WithSieges asks for, nil without it.
func newSieges(db *sql.DB, o *options, castles *castle.Manager, clans *clan.Service, worker *persist.Worker) *siege.Engine {
	if o.sieges == nil {
		return nil
	}
	var fields []*zone.Siege
	if o.zones != nil {
		fields = zone.OfKind[*zone.Siege](o.zones)
	}
	return siege.New(*o.sieges, castles, clans.Table(), fields, gamesql.NewSiegeStore(db), worker, o.log)
}

// startSieges restores the sieges' registrations once the castles have
// their owners and runs their calendars on queue, as the boot path does.
func startSieges(t *testing.T, sieges *siege.Engine, queue *sim.Queue, gcl *network.GameClientLink) {
	t.Helper()
	if sieges == nil {
		return
	}
	if err := sieges.Restore(context.Background()); err != nil {
		t.Fatalf("restore sieges: %v", err)
	}
	t.Cleanup(queue.Close)
	sieges.Start(queue, network.SiegeNotifier(gcl))
}

// PlayerSiegeState returns the siege state of the online player objID.
func (s *Server) PlayerSiegeState(tb testing.TB, objID int32) int32 {
	tb.Helper()
	return s.onlineCharacter(tb, objID).SiegeState()
}
