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
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
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
	table, err := xml.LoadSpawnlist(dir, zerolog.Nop(), 1)
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

func TestNpcMovementCapsAtWaterSurface(t *testing.T) {
	dir := walkerTestSpawnFixture(t)
	table, err := xml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("LoadSpawnlist() error: %v", err)
	}
	form, err := zone.NewCuboid(0, 1_000, 0, 1_000, -1_000, 150)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewWater(1, form))
	state := world.New()
	decay, err := task.NewDecay(nopDecayEffects{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	respawn, err := task.NewRespawn(nopRespawnEffects{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	walker, err := task.NewWalker(nil, alwaysOpenPath{}, time.Now, state)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewNpcs(NewSpawns(table, nil), walkerTestTemplate(), fakeGeo{}, state, &sequentialIDs{}, decay, respawn, task.NewAI(state, zerolog.Nop()), task.NewPositionUpdates(state), item.NewTable(nil), &recordingGround{}, KillRewardConfig{}, time.Now, zerolog.Nop(), nil, actorcast.EffectHandlers{}, walker, zones)
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
	if _, err := hostile.Move().MoveToLocation(location.Location{X: 300, Y: 200, Z: 200}); err != nil {
		t.Fatalf("MoveToLocation() error: %v", err)
	}
	hostile.Move().UpdatePosition(10 * time.Second)
	if got := hostile.Move().Position(); got.Z != 150 {
		t.Fatalf("Position().Z = %d, want water surface 150", got.Z)
	}
}

// TestNpcDespawnStopsWalkerRoute pins issue #1940's despawn side: once an
// NPC leaves state (RespawnHook), its route registration must not survive —
// a later Arrived call for its (possibly since-reused) object id must be a
// no-op, not a stale route continuation.
func TestNpcDespawnStopsWalkerRoute(t *testing.T) {
	dir := walkerTestSpawnFixture(t)
	table, err := xml.LoadSpawnlist(dir, zerolog.Nop(), 1)
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

// TestNpcLeashReturnDoesNotHijackWalkerRoute pins a review finding on #1940:
// the shared moveCtl.SetArrived hook fires for every kind of movement this
// Hostile makes, not only route moves — offensive-follow chase and leash
// return-home (Hostile.ReturnHome -> MoveHome) go through the very same
// hook. aCis NpcAI.onEvtArrived only continues route-node logic when the
// current AI intention is MOVE_ROUTE (NpcAI.java onEvtArrived: bails with
// `_isOnARoute = false; return;` otherwise); Go has no such intention to
// check, so without a gate a leash-return arrival would be misread as a
// route arrival and the walker would immediately re-issue a move back
// toward the patrol node, hijacking the NPC away from where it just
// leashed to.
func TestNpcLeashReturnDoesNotHijackWalkerRoute(t *testing.T) {
	dir := t.TempDir()
	writeSpawnFixture(t, filepath.Join(dir, "chase.xml"), `
<list>
	<territory name="field" minZ="-10" maxZ="10">
		<node x="0" y="0"/>
		<node x="2000" y="0"/>
		<node x="2000" y="2000"/>
		<node x="0" y="2000"/>
	</territory>
	<npcmaker name="maker" territory="field" maximumNpcs="1">
		<npc id="502" total="1" pos="100;200;0;12345"/>
	</npcmaker>
</list>`)
	table, err := xml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("LoadSpawnlist() error: %v", err)
	}
	templates := npc.NewTable([]*npc.Template{{
		ID:          502,
		TemplateID:  502,
		Type:        "Monster",
		Alias:       "chasetest",
		HPMax:       100,
		RunSpeed:    7000, // fast enough that both route and leash moves below finish in ~1 position tick
		AIParams:    commons.NewStatSet(),
		NoSleepMode: true,
	}})
	spawns := NewSpawns(table, nil)

	home := location.Location{X: 100, Y: 200, Z: 0}
	// Inside the default 200-unit drift range, so the general AI's own
	// leash-return (hostile.Think -> ReturnHome, fired automatically on
	// every arrival, including this route arrival) leaves the route alone
	// once the NPC settles here.
	routeNode := location.Location{X: 100, Y: 350, Z: 0}
	// Outside the drift range from home, so a later ReturnHome() call
	// actually triggers a leash move.
	strayPoint := location.Location{X: 100, Y: 800, Z: 0}
	routes := route.WalkerRoutes{
		"chasetest": {"chasetest": []route.WalkerLocation{{Location: routeNode}}},
	}

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
	walker, err := task.NewWalker(routes, alwaysOpenPath{}, time.Now, state)
	if err != nil {
		t.Fatalf("NewWalker() error: %v", err)
	}

	if _, err := NewNpcs(spawns, templates, fakeGeo{}, state, ids, decay, respawnTask, ai, positions, items,
		&recordingGround{}, KillRewardConfig{}, time.Now, zerolog.Nop(), nil, actorcast.EffectHandlers{}, walker); err != nil {
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

	// Wait for the route's initial move to settle the NPC at routeNode.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && (hostile.Move().Moving() || hostile.Move().Position() != routeNode) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := hostile.Move().Position(); got != routeNode {
		t.Fatalf("precondition: Position() = %+v, want routeNode %+v", got, routeNode)
	}

	// Simulate the NPC having wandered off (e.g. chasing a target) to a
	// point outside the drift range, without going through any move.Event —
	// SetXYZ reseeds position directly, so it fires no arrival hook and
	// leaves the route's own state untouched, isolating what happens next.
	hostile.SetXYZ(strayPoint.X, strayPoint.Y, strayPoint.Z)

	// Simulate the AI loop deciding this NPC must leash home — the same
	// call hostile.Think() makes via returnHomeOutsideDriftRange.
	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false, want true (strayPoint is outside the default drift range)")
	}

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && hostile.Move().Moving() {
		time.Sleep(10 * time.Millisecond)
	}
	// Give a hijacked route re-move (the bug) time to actually start and be
	// observed, rather than racing a check against the exact instant the
	// leash arrival's callback runs.
	time.Sleep(50 * time.Millisecond)

	if got := hostile.Move().Position(); got != home {
		t.Fatalf("Position() after leash return = %+v, want home %+v (walker route hijacked the leash move)", got, home)
	}
	if hostile.Move().Moving() {
		t.Fatal("Move().Moving() = true after leash return settled, want false (walker route re-issued a move)")
	}
}

// TestWalkerWalkModeNPCsMoveAtWalkSpeed pins issue #2028: aCis Walkers.java
// onCreated forces a specific WALKING_NPCS id subset into walk stance
// (setWalkOrRun(false)) instead of every other NPC's default run stance, so
// those ids must move at their template's WalkSpeed, not RunSpeed, while on
// their route (and at all times — the reference never toggles an NPC back).
func TestWalkerWalkModeNPCsMoveAtWalkSpeed(t *testing.T) {
	dir := t.TempDir()
	writeSpawnFixture(t, filepath.Join(dir, "walkmode.xml"), `
<list>
	<territory name="field" minZ="-10" maxZ="10">
		<node x="0" y="0"/>
		<node x="1000" y="0"/>
		<node x="1000" y="1000"/>
		<node x="0" y="1000"/>
	</territory>
	<npcmaker name="maker" territory="field" maximumNpcs="1">
		<npc id="31357" total="1" pos="100;200;0;0"/>
	</npcmaker>
</list>`)
	table, err := xml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("LoadSpawnlist() error: %v", err)
	}
	templates := npc.NewTable([]*npc.Template{{
		ID:         31357,
		TemplateID: 31357,
		Type:       "Monster",
		HPMax:      100,
		RunSpeed:   200,
		WalkSpeed:  50,
		AIParams:   commons.NewStatSet(),
	}})
	spawns := NewSpawns(table, nil)

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
	walker, err := task.NewWalker(nil, alwaysOpenPath{}, time.Now, state)
	if err != nil {
		t.Fatalf("NewWalker() error: %v", err)
	}

	if _, err := NewNpcs(spawns, templates, fakeGeo{}, state, ids, decay, respawnTask, ai, positions, items,
		&recordingGround{}, KillRewardConfig{}, time.Now, zerolog.Nop(), nil, actorcast.EffectHandlers{}, walker); err != nil {
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

	event, err := hostile.Move().MoveToLocation(location.Location{X: 900, Y: 200, Z: 0})
	if err != nil {
		t.Fatalf("MoveToLocation() error: %v", err)
	}
	if got, want := event.Speed, 50.0; got != want {
		t.Fatalf("MoveToLocation() Speed = %v, want WalkSpeed %v (RunSpeed leaked through for a WALKING_NPCS id)", got, want)
	}
}
