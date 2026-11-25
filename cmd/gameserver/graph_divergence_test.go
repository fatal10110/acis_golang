package main

import (
	"maps"
	"slices"
	"testing"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// bootOverrides are the types the test harness supplies in place of the
// production provider: its seams.
var bootOverrides = map[string]string{
	"*sql.DB": "the pooled test database, behind any WithDBFault wrap",
}

// notShared are the types the production graph still provides from
// cmd/gameserver. Each slice of #4709 moves some into internal/gameserver/app
// and deletes them here.
var notShared = []string{
	"*announcement.Registry",
	"*boat.Fleet",
	"*cache.Crests",
	"*cache.HTML",
	"*castle.Manager",
	"*castlemanor.Manager",
	"*clan.Service",
	"*clanhall.Functions",
	"*clanhall.Halls",
	"*config.Properties",
	"*cursedweapon.Manager",
	"*derby.Track",
	"*festival.Manager",
	"*fishchamp.Championship",
	"*hero.Manager",
	"*idfactory.Allocator",
	"*lottery.Lottery",
	"*main.clientBans",
	"*main.doorTimerEffects",
	"*main.gameData",
	"*main.loginLinkState",
	"*main.npcRespawnEffects",
	"*main.serverSession",
	"*main.worldDecayEffects",
	"*manager.Npcs",
	"*manager.Roster",
	"*manager.Spawns",
	"*manager.WorldObjects",
	"*merchant.Service",
	"*merchant.Stock",
	"*network.GameClientLink",
	"*network.LoginStatus",
	"*network.SessionValidator",
	"*network.TaskEffects",
	"*olympiad.Olympiad",
	"*persist.Order",
	"*persist.Worker",
	"*petition.Manager",
	"*raidpoint.Points",
	"*relation.Manager",
	"*schemebuffer.Manager",
	"*script.Env",
	"*script.EventBook",
	"*script.Makers",
	"*script.Quests",
	"*script.RateBook",
	"*script.Registry",
	"*script.Spawner",
	"*sevensigns.State",
	"*shutdown.Countdown",
	"*siege.Engine",
	"*sim.Pool",
	"*skill.Persistence",
	"*sql.CoupleStore",
	"*sql.GroundItemStore",
	"*sql.PetitionStore",
	"*sql.RelationStore",
	"*sql.SpawnStore",
	"*task.AI",
	"*task.AttackStance",
	"*task.Autosave",
	"*task.Decay",
	"*task.Door",
	"*task.DoorRegen",
	"*task.Effects",
	"*task.GameClock",
	"*task.GroundItems",
	"*task.InventoryUpdates",
	"*task.ItemInstances",
	"*task.NPCRegen",
	"*task.PlayerClock",
	"*task.PositionUpdates",
	"*task.PvPFlags",
	"*task.Respawn",
	"*task.ShadowItems",
	"*task.Walker",
	"*task.Water",
	"*wedding.Manager",
	"*world.State",
	"clan.Config",
	"effect.Env",
	"main.bootContext",
	"main.chatLogger",
	"main.communityBoard",
	"main.gameServerConfig",
	"main.gameServerPaths",
	"main.gameplayConfig",
	"main.gmAuditLogger",
	"main.hexIDProperties",
	"main.itemLogger",
	"main.sevenSignsQueue",
	"manager.KillRewardConfig",
	"network.DataReloads",
	"pet.Config",
	"signspriest.Config",
	"skill.BookPolicy",
	"task.GroundItemOptions",
	"task.PvPFlagOptions",
	"zerolog.Logger",
}

// TestBootBuildsFromTheProductionGraph fails when the graph the test harness
// boots from and the production graph diverge (#4709): every type Boot's
// graph provides comes from the production constructor, unless it is a
// listed seam, and every type production provides is in Boot's graph,
// unless it is listed as not shared yet.
func TestBootBuildsFromTheProductionGraph(t *testing.T) {
	prod := providedTypes(t, gameServerProviders(gameServerPaths{})...)
	boot := providedTypes(t, gameservertest.BootGraph(nil))

	for _, typ := range sortedKeys(boot) {
		prodCtor, inProd := prod[typ]
		_, seam := bootOverrides[typ]
		switch {
		case !inProd:
			t.Errorf("Boot's graph provides %s, which the production graph does not", typ)
		case seam:
		case boot[typ] != prodCtor:
			t.Errorf("Boot's graph provides %s through %s, production through %s; add an override to bootOverrides only for a test seam", typ, boot[typ], prodCtor)
		}
	}
	for _, typ := range sortedKeys(prod) {
		if _, ok := boot[typ]; !ok && !slices.Contains(notShared, typ) {
			t.Errorf("production provides %s, which Boot's graph lacks; provide it from internal/gameserver/app", typ)
		}
	}
	for _, typ := range notShared {
		if _, ok := prod[typ]; !ok {
			t.Errorf("notShared lists %s, which production no longer provides", typ)
		}
		if _, ok := boot[typ]; ok {
			t.Errorf("notShared lists %s, which Boot's graph provides now", typ)
		}
	}
	for typ := range bootOverrides {
		if _, ok := boot[typ]; !ok {
			t.Errorf("bootOverrides lists %s, which Boot's graph does not provide", typ)
		}
	}
}

// providedTypes returns, for each type the provider options opts provide,
// the name of its constructor. opts hold no invokes, so no constructor runs.
func providedTypes(t *testing.T, opts ...fx.Option) map[string]string {
	t.Helper()
	log := provideLog{}
	opts = append(slices.Clip(opts), fx.WithLogger(func() fxevent.Logger { return log }))
	if err := fx.New(opts...).Err(); err != nil {
		t.Fatalf("build graph: %v", err)
	}
	return log
}

// provideLog records the constructor of each provided type, by type name.
type provideLog map[string]string

func (l provideLog) LogEvent(e fxevent.Event) {
	switch e := e.(type) {
	case *fxevent.Provided:
		for _, name := range e.OutputTypeNames {
			l[name] = e.ConstructorName
		}
	case *fxevent.Supplied:
		l[e.TypeName] = "fx.Supply"
	}
}

func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}
