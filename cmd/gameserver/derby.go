package main

import (
	"database/sql"
	"math/rand/v2"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/derby"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// provideDerbyTrack builds the monster race track: its race records and
// stakes restored from mdt_history and mdt_bets, and its runners the loaded
// templates of npcs 31003 to 31026.
func provideDerbyTrack(ctx bootContext, db *sql.DB, data *gameData, ids *idfactory.Allocator, state *world.State, log zerolog.Logger) (*derby.Track, error) {
	return derby.New(ctx, gamesql.NewDerbyStore(db), derbyRunnerTemplates(data.NPCs), ids, network.DerbySink(state, data.Zones), rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())), log)
}

// derbyRunnerTemplates returns the loaded runner templates in npc id
// order; an id with no template is left out.
func derbyRunnerTemplates(npcs *npc.Table) []derby.Template {
	var out []derby.Template
	for id := derby.FirstRunnerID; id <= derby.LastRunnerID; id++ {
		tpl, ok := npcs.Get(id)
		if !ok {
			continue
		}
		out = append(out, derby.Template{NpcID: tpl.ID, Name: tpl.Name, CollisionHeight: tpl.CollisionHeight, CollisionRadius: tpl.CollisionRadius})
	}
	return out
}

// startDerbyTrack runs the race countdown while the server runs.
func startDerbyTrack(lc fx.Lifecycle, track *derby.Track, log zerolog.Logger) {
	startTicker(lc, log, track.Start)
}
