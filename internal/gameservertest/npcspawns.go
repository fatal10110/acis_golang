package gameservertest

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/maker"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
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

// WithWalkerRoutes gives the WithNpcSpawns population routes as its walker
// route data, walked on the actor queues' clock; by default it has none.
func WithWalkerRoutes(routes route.WalkerRoutes) Option {
	return func(o *options) { o.walkerRoutes = routes }
}

// WithNpcSpawnSkills gives the WithNpcSpawns population defs as the skills
// its NPCs cast, their effects dispatched through the link's NPC cast seam,
// as the game server gives it the skill data; by default its NPCs cast
// nothing.
func WithNpcSpawnSkills(defs actorcast.Definitions) Option {
	return func(o *options) { o.npcSpawnSkills = defs }
}

// WithSpawnData backs the WithNpcSpawns population's database-tracked
// spawns with the spawn_data table, as the game server does: their rows
// are restored at boot, and Shutdown saves them.
func WithSpawnData() Option {
	return func(o *options) { o.spawnData = true }
}

// WithMakers gives the WithNpcSpawns population catalog's makers beside
// the server's own; a type in both runs catalog's.
func WithMakers(catalog script.MakerCatalog) Option {
	return func(o *options) { o.makerCatalog = catalog }
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
	// spawner is the scripts' spawner, bound to the population before the
	// first NPC spawns.
	spawner *script.Spawner
	// routes is the walker route data.
	routes route.WalkerRoutes
	// skills are the skills the NPCs cast, nil for none.
	skills actorcast.Definitions

	// zones is the WithZones index the spawned NPCs enter, as production
	// hands the population its zones; nil when none was supplied.
	zones *zone.Index

	// spawnStore restores and saves the database-tracked spawns' rows; nil
	// keeps them in memory only.
	spawnStore *gamesql.SpawnStore
	// makerCatalog are makers added to the server's own.
	makerCatalog script.MakerCatalog
	// driftRange is the WithMaxDriftRange drift range; 0 keeps the shipped
	// one.
	driftRange int
	// night tells the makers whether it is night in the game world; nil
	// says it is day.
	night func() bool
	// castles and sieges are the castles the makers find by id and their
	// sieges; nil finds none.
	castles *castle.Manager
	sieges  *siege.Engine
	// doors are the doors the makers find by name; nil finds none.
	doors *gamemanager.WorldObjects
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
	now := time.Now
	if deps.queues != nil && deps.queues.inline != nil {
		now = deps.queues.inline.Now
	}
	walker, err := task.NewWalker(deps.routes, task.GeoPath{Geo: Geo{}}, now, deps.state)
	if err != nil {
		t.Fatalf("new walker: %v", err)
	}
	templates := deps.templates
	if templates == nil {
		templates = npc.NewTable(nil)
	}
	castEffects := actorcast.EffectHandlers{}
	if deps.skills != nil {
		castEffects = link.HostileCastEffects()
	}
	var states map[string]*spawn.State
	if deps.spawnStore != nil {
		if states, err = deps.spawnStore.LoadStates(context.Background()); err != nil {
			t.Fatalf("load spawn data: %v", err)
		}
	}
	makers := maker.Catalog()
	maps.Copy(makers, deps.makerCatalog)
	night := deps.night
	if night == nil {
		night = func() bool { return false }
	}
	makerWorld := script.MakerWorld{Night: night, Castles: deps.castles, Sieges: deps.sieges}
	if deps.doors != nil {
		makerWorld.Doors = deps.doors
	}
	aiConfig := npc.DefaultAIConfig()
	if deps.driftRange != 0 {
		aiConfig.DriftRange = deps.driftRange
	}
	npcs, err := gamemanager.NewNpcsWithMaxBuffsAmount(gamemanager.NewSpawns(deps.makers, states), templates, deps.geo, deps.state, deps.ids,
		decay, respawn, ai, deps.positions, deps.items, deps.ground, gamemanager.KillRewardConfig{Settings: gamemanager.NewKillRewardBook(gamemanager.KillRewardSettings{Rates: deps.dropRates, RateXP: 1, RateSP: 1})}, time.Now, deps.log,
		deps.skills, castEffects, walker, link.HostileSinks(deps.state, deps.stance), link.FolkSinks(deps.state, deps.stance),
		20, 0, 0, npc.DefaultRaidMultipliers(), aiConfig, gamemanager.DefaultSpawnEvents(), deps.effects, deps.queues,
		script.NewMakers(makers, maker.Default, makerWorld, deps.log), deps.scripts, deps.zones)
	if err != nil {
		t.Fatalf("new npc spawns: %v", err)
	}
	if deps.spawner != nil {
		deps.spawner.Bind(npcs)
	}
	npcs.SetGroundTaker(link)
	npcs.SetGuardedSieges(link.GuardedSiege)
	npcs.SetSpawnTimes(network.SpawnTimes(link))
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
