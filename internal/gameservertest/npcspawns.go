package gameservertest

import (
	"testing"
	"time"

	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// WithNpcSpawns gives the server a live NPC population wired into the link
// the way the game server wires its spawn table's, so the admin spawn
// commands can place NPCs. It is Server.NpcSpawns. At boot it spawns
// makers' on-start NPCs (none when makers is nil), built from the WithNPCs
// templates; none of its tasks tick.
func WithNpcSpawns(makers *spawn.Table) Option {
	return func(o *options) {
		if makers == nil {
			makers = &spawn.Table{}
		}
		o.npcSpawns = makers
	}
}

// npcSpawnDeps are the server parts a live NPC population is built over.
type npcSpawnDeps struct {
	state     *world.State
	templates *npc.Table
	geo       move.Geo
	ids       *sequentialIDs
	decay     *task.Decay
	ai        *task.AI
	positions *task.PositionUpdates
	items     *item.Table
	ground    *task.GroundItems
	effects   effect.Env
	queues    *queues
	stance    network.AttackStanceTracker
	makers    *spawn.Table
	log       zerolog.Logger
}

// bootNpcSpawns builds the WithNpcSpawns population over deps and hands it
// to link.
func bootNpcSpawns(t *testing.T, link *network.GameClientLink, deps npcSpawnDeps) *gamemanager.Npcs {
	t.Helper()
	decay := deps.decay
	if decay == nil {
		var err error
		if decay, err = task.NewDecay(noDecay{}, time.Now); err != nil {
			t.Fatalf("new decay: %v", err)
		}
	}
	respawn, err := task.NewRespawn(noRespawn{}, time.Now)
	if err != nil {
		t.Fatalf("new respawn: %v", err)
	}
	ai := deps.ai
	if ai == nil {
		ai = task.NewAI(deps.state, deps.log)
	}
	walker, err := task.NewWalker(nil, task.GeoPath{Geo: Geo{}}, time.Now, deps.state)
	if err != nil {
		t.Fatalf("new walker: %v", err)
	}
	templates := deps.templates
	if templates == nil {
		templates = npc.NewTable(nil)
	}
	npcs, err := gamemanager.NewNpcsWithMaxBuffsAmount(gamemanager.NewSpawns(deps.makers, nil), templates, deps.geo, deps.state, deps.ids,
		decay, respawn, ai, deps.positions, deps.items, deps.ground, gamemanager.KillRewardConfig{}, time.Now, deps.log,
		nil, actorcast.EffectHandlers{}, walker, network.HostileSinks(deps.state, deps.stance), network.FolkSinks(deps.state, deps.stance),
		20, 0, 0, npc.DefaultRaidMultipliers(), npc.DefaultAIConfig(), deps.effects, deps.queues)
	if err != nil {
		t.Fatalf("new npc spawns: %v", err)
	}
	link.SetNpcSpawns(npcs)
	return npcs
}

type noDecay struct{}

func (noDecay) Decay(task.DecayActor) {}

type noRespawn struct{}

func (noRespawn) Respawn(string) {}
