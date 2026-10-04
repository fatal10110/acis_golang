package gameservertest

import (
	"context"
	"database/sql"
	"testing"

	"github.com/rs/zerolog"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/derby"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// derbyOptions are the race track WithDerbyTrack wires.
type derbyOptions struct {
	runners []derby.Template
	rnd     derby.Rand
}

// WithDerbyTrack wires a monster race track running runners, drawn with
// rnd, its records and stakes in the test database's mdt_history and
// mdt_bets (default: no track). Its countdown is not started; a test drives
// it through Server.Derby.Tick. It announces to the players inside the
// derby track zones of WithZones.
func WithDerbyTrack(runners []derby.Template, rnd derby.Rand) Option {
	return func(o *options) { o.derby = &derbyOptions{runners: runners, rnd: rnd} }
}

func bootDerbyTrack(t *testing.T, o *derbyOptions, db *sql.DB, ids *sequentialIDs, state *world.State, zones *zone.Index) *derby.Track {
	t.Helper()
	if o == nil {
		return nil
	}
	track, err := derby.New(context.Background(), gamesql.NewDerbyStore(db), o.runners, ids, network.DerbySink(state, zones), o.rnd, zerolog.Nop())
	if err != nil {
		t.Fatalf("derby track: %v", err)
	}
	return track
}
