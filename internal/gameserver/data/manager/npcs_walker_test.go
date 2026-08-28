package manager

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// alwaysOpenPath is a task.WalkerPath test double that never blocks
// movement — this test exercises spawn/arrival wiring, not geodata pathing.
type alwaysOpenPath struct{}

func (alwaysOpenPath) CanMove(origin, target location.Location) bool { return true }
func (alwaysOpenPath) HasPath(origin, target location.Location) bool { return true }

const (
	walkerTestSpawnHeading = 12345
	walkerTestAlias        = "briartest"
)

// walkerTestSpawnFixture writes a spawnlist with one fresh (non-db) entry
// fixed at (100, 200, 0), heading 12345 — matching walkerTestRoutes' single
// node, so the NPC's very first route move is a same-cell arrival back at
// its own spawn point.
func walkerTestSpawnFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeSpawnFixture(t, filepath.Join(dir, "walker.xml"), `
<list>
	<territory name="field" minZ="-10" maxZ="10">
		<node x="0" y="0"/>
		<node x="1000" y="0"/>
		<node x="1000" y="1000"/>
		<node x="0" y="1000"/>
	</territory>
	<npcmaker name="maker" territory="field" maximumNpcs="1">
		<npc id="501" total="1" pos="100;200;0;12345"/>
	</npcmaker>
</list>`)
	return dir
}

func walkerTestTemplate() *npc.Table {
	return npc.NewTable([]*npc.Template{{
		ID:         501,
		TemplateID: 501,
		Type:       "Monster",
		Alias:      walkerTestAlias,
		HPMax:      100,
		RunSpeed:   100,
		AIParams:   commons.NewStatSet(),
		// Route-walking NPCs must keep patrolling with nobody around to see
		// them, the same way the reference's Walkers-registered ids do —
		// without it task.AI's own region-inactivity gate (unrelated to
		// this issue) would leave this fixture NPC parked forever with no
		// player in the test's empty world.
		NoSleepMode: true,
	}})
}

func walkerTestRoutes(spawnPoint location.Location) route.WalkerRoutes {
	return route.WalkerRoutes{
		walkerTestAlias: {walkerTestAlias: []route.WalkerLocation{{Location: spawnPoint}}},
	}
}

// TestNpcSpawnRegistersWalkerRouteAndRestoresSpawnHeading pins issue #1940:
// a spawned NPC whose template alias resolves in walkerRoutes.xml is
// registered with task.Walker at spawn (previously PR #418's Walker task
// had no production caller at all — StartRoute/Arrived/MoveToNextPoint were
// unreachable outside tests), and an arrival that lands exactly back at the
// spawn point restores the spawn heading (aCis NpcAI.onEvtArrived,
// NpcAI.java:295-296), even after the NPC's heading changed in between.
func TestNpcSpawnRegistersWalkerRouteAndRestoresSpawnHeading(t *testing.T) {
	dir := walkerTestSpawnFixture(t)
	table, err := xml.LoadSpawnlist(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSpawnlist() error: %v", err)
	}
	templates := walkerTestTemplate()
	spawns := NewSpawns(table, nil)

	spawnPoint := location.Location{X: 100, Y: 200, Z: 0}
	state := world.New()
	ids := &sequentialIDs{}
	decay, err := task.NewDecay(nopDecayEffects{}, time.Now)
	if err != nil {
		t.Fatalf("NewDecay() error: %v", err)
	}
	respawnTask, err := task.NewRespawn(nopRespawnEffects{}, time.Now)
	if err != nil {
		t.Fatalf("NewRespawn() error: %v", err)
	}
	ai := task.NewAI(state, zerolog.Nop())
	positions := task.NewPositionUpdates(state)
	items := item.NewTable(nil)
	walker, err := task.NewWalker(walkerTestRoutes(spawnPoint), alwaysOpenPath{}, time.Now, state)
	if err != nil {
		t.Fatalf("NewWalker() error: %v", err)
	}

	npcs, err := NewNpcs(spawns, templates, fakeGeo{}, state, ids, decay, respawnTask, ai, positions, items,
		&recordingGround{}, KillRewardConfig{}, time.Now, zerolog.Nop(), nil, actorcast.EffectHandlers{}, walker)
	if err != nil {
		t.Fatalf("NewNpcs() error: %v", err)
	}
	if got, want := npcs.LiveCount(), 1; got != want {
		t.Fatalf("LiveCount() = %d, want %d", got, want)
	}

	obj, ok := state.Object(1)
	if !ok {
		t.Fatal("spawned npc object id 1 not found")
	}
	hostile, ok := obj.(*npc.Hostile)
	if !ok {
		t.Fatalf("object id 1 is %T, want *npc.Hostile", obj)
	}

	// Prove the eventual match to walkerTestSpawnHeading below comes from
	// the arrival's own reset, not coincidence: knock the heading away from
	// it first. StartRoute's same-cell move has not yet arrived at this
	// point — its arrival timer fires PositionUpdateInterval (100ms) after
	// spawn — so this always lands before the reset.
	hostile.SetHeading(999)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && hostile.Heading() != walkerTestSpawnHeading {
		time.Sleep(10 * time.Millisecond)
	}
	if got := hostile.Heading(); got != walkerTestSpawnHeading {
		t.Fatalf("Heading() after route arrival = %d, want spawn heading %d", got, walkerTestSpawnHeading)
	}
}

// TestNpcDespawnStopsWalkerRoute pins issue #1940's despawn side: once an
// NPC leaves state (RespawnHook), its route registration must not survive —
// a later Arrived call for its (possibly since-reused) object id must be a
// no-op, not a stale route continuation.
func TestNpcDespawnStopsWalkerRoute(t *testing.T) {
	dir := walkerTestSpawnFixture(t)
	table, err := xml.LoadSpawnlist(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSpawnlist() error: %v", err)
	}
	templates := walkerTestTemplate()
	spawns := NewSpawns(table, nil)

	spawnPoint := location.Location{X: 100, Y: 200, Z: 0}
	state := world.New()
	ids := &sequentialIDs{}
	decay, err := task.NewDecay(nopDecayEffects{}, time.Now)
	if err != nil {
		t.Fatalf("NewDecay() error: %v", err)
	}
	respawnTask, err := task.NewRespawn(nopRespawnEffects{}, time.Now)
	if err != nil {
		t.Fatalf("NewRespawn() error: %v", err)
	}
	ai := task.NewAI(state, zerolog.Nop())
	positions := task.NewPositionUpdates(state)
	items := item.NewTable(nil)
	walker, err := task.NewWalker(walkerTestRoutes(spawnPoint), alwaysOpenPath{}, time.Now, state)
	if err != nil {
		t.Fatalf("NewWalker() error: %v", err)
	}

	npcs, err := NewNpcs(spawns, templates, fakeGeo{}, state, ids, decay, respawnTask, ai, positions, items,
		&recordingGround{}, KillRewardConfig{}, time.Now, zerolog.Nop(), nil, actorcast.EffectHandlers{}, walker)
	if err != nil {
		t.Fatalf("NewNpcs() error: %v", err)
	}

	obj, ok := state.Object(1)
	if !ok {
		t.Fatal("spawned npc object id 1 not found")
	}
	hostile, ok := obj.(*npc.Hostile)
	if !ok {
		t.Fatalf("object id 1 is %T, want *npc.Hostile", obj)
	}

	npcs.RespawnHook(1)

	if err := walker.MoveToNextPoint(&walkerActorRef{Hostile: hostile}); err == nil {
		t.Fatal("MoveToNextPoint() error = nil after despawn, want a not-registered error")
	}
}
