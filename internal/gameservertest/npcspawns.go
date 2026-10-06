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
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/maker"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
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

// WithNpcDropRates is WithNpcSpawns (no makers unless that option names
// some) whose kills, and the admin drop pages, roll drop categories with
// rates.
func WithNpcDropRates(rates item.Rates) Option {
	return func(o *options) {
		if o.npcSpawns == nil {
			o.npcSpawns = &spawn.Table{}
		}
		o.npcDropRates = rates
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
	dropRates item.Rates
	log       zerolog.Logger
	// sevenSigns is the state the Seven Signs groups follow, and whose
	// period changes swap them.
	sevenSigns *sevensigns.State
	// scripts raises the spawned NPCs' script hooks.
	scripts *script.Registry
}

// WithDataReloads gives the link the //reload and //respawnall hooks; by
// default it has none.
func WithDataReloads(reloads network.DataReloads) Option {
	return func(o *options) { o.dataReloads = reloads }
}

// bootNpcSpawns builds the WithNpcSpawns population over deps and hands it
// to link. It returns the population and its respawn timers.
func bootNpcSpawns(t *testing.T, link *network.GameClientLink, deps npcSpawnDeps) (*gamemanager.Npcs, *task.Respawn) {
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
		decay, respawn, ai, deps.positions, deps.items, deps.ground, gamemanager.KillRewardConfig{Rates: deps.dropRates, RateXP: 1, RateSP: 1}, time.Now, deps.log,
		nil, actorcast.EffectHandlers{}, walker, network.HostileSinks(deps.state, deps.stance), link.FolkSinks(deps.state, deps.stance),
		20, 0, 0, npc.DefaultRaidMultipliers(), npc.DefaultAIConfig(), gamemanager.DefaultSpawnEvents(), deps.effects, deps.queues,
		script.NewMakers(maker.Catalog(), maker.Default, deps.log), deps.scripts)
	if err != nil {
		t.Fatalf("new npc spawns: %v", err)
	}
	link.SetNpcSpawns(npcs)
	npcs.SpawnOnStart()
	if deps.sevenSigns != nil {
		deps.sevenSigns.SetSpawns(npcs)
		npcs.StartSevenSigns(deps.sevenSigns)
	}
	return npcs, respawn
}

type noDecay struct{}

func (noDecay) Decay(task.DecayActor) {}

type noRespawn struct{}

func (noRespawn) Respawn(string) {}
