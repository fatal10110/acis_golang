package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/group"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The route fixture's NPC templates.
const (
	scribeID      = 31357 // Leandro: a walker that walks its route
	porterID      = 31356 // Remy: a walker that runs its route
	strollerID    = 30500 // a civilian no script binds, whose alias names a route
	patrolID      = 21301 // a monster a test behavior sends along its route
	scribeAlias   = "scribe_leandro"
	porterAlias   = "porter_remy"
	strollerAlias = "stroller"
	patrolAlias   = "patrol"
)

// routeWorld is a booted world with the route fixture's templates and
// routes, the character in it and the AI task driven by hand.
type routeWorld struct {
	*folkWorld
	// spawnAt is where each fixture NPC is spawned, its route's two nodes
	// a and b lying 50 and 250 from it on the same line.
	spawnAt, a, b map[int32]location.Location
}

// bootRouteWorld boots the route fixture with list registered from
// catalog, every fixture id with a template of its kind.
func bootRouteWorld(t *testing.T, list []script.Listing, catalog script.Catalog, opts ...gameservertest.Option) *routeWorld {
	t.Helper()
	templates := []*npc.Template{}
	for _, f := range []struct {
		id    int
		alias string
	}{{scribeID, scribeAlias}, {porterID, porterAlias}, {strollerID, strollerAlias}} {
		tmpl := gameservertest.FolkTemplate("Folk", f.id)
		tmpl.Alias, tmpl.CanMove = f.alias, true
		templates = append(templates, tmpl)
	}
	patrol := spawnMonster(patrolID, "Patrol")
	patrol.Alias = patrolAlias
	templates = append(templates, patrol)

	// The fixture is laid out around where the character enters the world.
	at := location.Location{X: 10, Y: 20, Z: gameservertest.SpawnZ}
	w := &routeWorld{spawnAt: map[int32]location.Location{}, a: map[int32]location.Location{}, b: map[int32]location.Location{}}
	routes := route.WalkerRoutes{}
	for i, f := range []struct {
		id    int32
		alias string
	}{{scribeID, scribeAlias}, {porterID, porterAlias}, {strollerID, strollerAlias}, {patrolID, patrolAlias}} {
		// A script spawn stands 20 above the ground the geodata gives.
		y, z := at.Y+150*i, at.Z+20
		w.spawnAt[f.id] = location.Location{X: at.X + 150, Y: y, Z: z}
		w.a[f.id] = location.Location{X: at.X + 100, Y: y, Z: z}
		w.b[f.id] = location.Location{X: at.X + 400, Y: y, Z: z}
		routes[f.alias] = map[string][]route.WalkerLocation{f.alias: {{Location: w.a[f.id]}, {Location: w.b[f.id]}}}
	}
	kinds := map[int32]script.NPCKind{scribeID: script.KindFolk, porterID: script.KindFolk, strollerID: script.KindFolk, patrolID: script.KindHostile}
	w.folkWorld = bootFolkWorld(t, nil, append([]gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithWalkerRoutes(routes),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithAITask(),
	}, opts...)...)
	if w.at != at {
		t.Fatalf("character stands at %v, want %v", w.at, at)
	}
	drainFrames(t, w.c)
	return w
}

// spawn spawns the fixture NPC id at its spawn point, as a script does,
// and returns it with the frames the character is shown about it.
func (w *routeWorld) spawn(t *testing.T, id int32) (attackable.Combatant, []npcFrame) {
	t.Helper()
	at := w.spawnAt[id]
	n, err := w.srv.NpcSpawns.AddSpawn(id, at.X, at.Y, at.Z-20, 0, false, 0)
	if err != nil {
		t.Fatalf("spawn %d: %v", id, err)
	}
	return n, aboutNPC(drainFrames(t, w.c), n.ObjectID())
}

// aboutNPC keeps the frames about the NPC objectID.
func aboutNPC(frames [][]byte, objectID int32) []npcFrame {
	var kept []npcFrame
	for _, frame := range frames {
		if len(frame) >= 5 && objectFrame(frame, frame[0], objectID) {
			kept = append(kept, npcFrame{opcode: frame[0], raw: frame})
		}
	}
	return kept
}

// legs returns the destination and origin of every walk among frames.
func legs(frames []npcFrame) (dests, origins []location.Location) {
	for _, fr := range frames {
		if fr.opcode == serverpackets.OpcodeMoveToLocation {
			dest, origin := moveLeg(fr)
			dests, origins = append(dests, dest), append(origins, origin)
		}
	}
	return dests, origins
}

func walkersList() ([]script.Listing, script.Catalog) {
	return []script.Listing{{Path: "script.ai.group.Walkers"}}, script.Catalog{"script.ai.group.Walkers": group.Walkers}
}

// The walker script sends its walkers along the routes listed under their
// aliases (Walkers.onCreated): created running, a walking walker is
// switched to its walk stance at once, shown to the players around it as
// ChangeMoveType then NpcInfo (Npc.setWalkOrRun), and neither moves until
// its AI takes up the route desire on its second tick (NpcAI.runAI's first
// tick acts on nothing); each then heads for its nearest node and, there,
// walks on to the next. A civilian no script binds keeps the alias rule:
// it walks its route from the moment it spawns.
func TestWalkersScriptSendsItsWalkersAlongTheirRoutes(t *testing.T) {
	t.Parallel()
	executors(t, testWalkersScript)
}

func testWalkersScript(t *testing.T, opts ...gameservertest.Option) {
	list, catalog := walkersList()
	w := bootRouteWorld(t, list, catalog, opts...)

	scribe, frames := w.spawn(t, scribeID)
	want := []byte{serverpackets.OpcodeNPCInfo, serverpackets.OpcodeChangeMoveType, serverpackets.OpcodeNPCInfo}
	if got := npcOpcodes(frames); string(got) != string(want) {
		t.Fatalf("scribe spawn frames = %#x, want NpcInfo, ChangeMoveType, NpcInfo", got)
	}
	if !npcInfoRunning(frames[0]) || moveTypeRunning(frames[1]) || npcInfoRunning(frames[2]) {
		t.Fatalf("scribe shown running %v, then stance running %v, then running %v; want run, walk, walk",
			npcInfoRunning(frames[0]), moveTypeRunning(frames[1]), npcInfoRunning(frames[2]))
	}
	porter, frames := w.spawn(t, porterID)
	if got := npcOpcodes(frames); len(got) != 1 || got[0] != serverpackets.OpcodeNPCInfo || !npcInfoRunning(frames[0]) {
		t.Fatalf("porter spawn frames = %#x, want one NpcInfo showing it running", got)
	}
	stroller, frames := w.spawn(t, strollerID)
	if dests, _ := legs(frames); len(dests) != 1 || dests[0] != w.a[strollerID] {
		t.Fatalf("unbound stroller legs at spawn = %v, want one to its nearest node %v", dests, w.a[strollerID])
	}

	s, p := scribe.(*npc.Folk), porter.(*npc.Folk)
	if s.Running() || !p.Running() {
		t.Fatalf("scribe running %v, porter running %v; want the scribe walking, the porter running", s.Running(), p.Running())
	}
	if s.IsMoving() || p.IsMoving() {
		t.Fatal("a scripted walker moves as it spawns, want it waiting for its AI")
	}

	frames1 := w.tickAI(t)
	for _, n := range []attackable.Combatant{scribe, porter} {
		if dests, _ := legs(aboutNPC(frames1, n.ObjectID())); len(dests) != 0 {
			t.Fatalf("npc %d walked %v on its first AI tick, want nothing", n.ObjectID(), dests)
		}
	}
	frames2 := w.tickAI(t)
	for _, c := range []struct {
		n  attackable.Combatant
		id int32
	}{{scribe, scribeID}, {porter, porterID}} {
		dests, origins := legs(aboutNPC(frames2, c.n.ObjectID()))
		if len(dests) != 1 || dests[0] != w.a[c.id] || origins[0] != w.spawnAt[c.id] {
			t.Fatalf("npc %d legs on its second AI tick = %v from %v, want one to %v from %v", c.id, dests, origins, w.a[c.id], w.spawnAt[c.id])
		}
	}

	w.srv.AdvanceUntil(t, "scribe reaches its first node", func() bool { return folkAt(s) == w.a[scribeID] })
	dests, origins := legs(aboutNPC(drainFrames(t, w.c), scribe.ObjectID()))
	if len(dests) != 1 || dests[0] != w.b[scribeID] || origins[0] != w.a[scribeID] {
		t.Fatalf("scribe legs at its first node = %v from %v, want one to %v", dests, origins, w.b[scribeID])
	}
	if !stroller.(*npc.Folk).IsMoving() && folkAt(stroller.(*npc.Folk)) == w.spawnAt[strollerID] {
		t.Fatal("unbound stroller never left its spawn point")
	}
}

// routeBehavior is a test behavior that asks the patrol monster, as it is
// created, to walk its route with weight.
func routeBehavior(weight float64) func() script.Script {
	return func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{patrolID}, Hooks: script.Hooks{
			OnCreated: func(_ *script.Script, e script.Created) { e.NPC.AddMoveRouteDesire(patrolAlias, weight) },
		}}
	}
}

// bootPatrol boots the route fixture with the patrol monster's route
// desire at weight and spawns the monster, walking its route once its AI
// takes the desire up.
func bootPatrol(t *testing.T, weight float64, opts ...gameservertest.Option) (*routeWorld, *npc.Hostile) {
	t.Helper()
	w := bootRouteWorld(t, []script.Listing{{Path: "ai.Patrol"}}, script.Catalog{"ai.Patrol": routeBehavior(weight)}, opts...)
	n, frames := w.spawn(t, patrolID)
	if dests, _ := legs(frames); len(dests) != 0 {
		t.Fatalf("patrol legs at spawn = %v, want none: a behavior is bound to it", dests)
	}
	h := n.(*npc.Hostile)
	w.tickAI(t)
	dests, _ := legs(aboutNPC(w.tickAI(t), h.ObjectID()))
	if len(dests) != 1 || dests[0] != w.a[patrolID] {
		t.Fatalf("patrol legs on its second AI tick = %v, want one to %v", dests, w.a[patrolID])
	}
	if got := h.AI().CurrentIntention(); got != ai.IntentionMoveRoute {
		t.Fatalf("patrol intention = %v, want move_route", got)
	}
	return w, h
}

// attack queues, on the patrol monster's queue, a desire to attack the
// character with weight, as a script's attacked hook does.
func (w *routeWorld) attack(t *testing.T, h *npc.Hostile, weight float64) {
	t.Helper()
	obj, ok := w.srv.State.Object(w.player)
	if !ok {
		t.Fatal("character is not in the world")
	}
	onQueueOf(t, h, func() { script.NPCOf(h).AddAttackDesire(script.PlayerOf(obj.(attackable.Combatant)), weight) })
}

// A route desire is ranked by its weight like any other (NpcAI.runAI): a
// monster whose route weighs 10, as Gordon's does, leaves it for a heavier
// attack desire, and once that desire has lost weight below the route's
// (6.6 every third AI tick) goes back to the route from the node nearest
// to where it stands.
func TestRouteDesireYieldsToAHeavierAttack(t *testing.T) {
	t.Parallel()
	executors(t, testRouteDesireYields)
}

func testRouteDesireYields(t *testing.T, opts ...gameservertest.Option) {
	w, h := bootPatrol(t, 10, opts...)

	w.attack(t, h, 15)
	if got := h.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("intention after a heavier attack desire = %v, want attack", got)
	}
	drainFrames(t, w.c)

	var resumed []location.Location
	for i := 0; i < 6 && h.AI().CurrentIntention() != ai.IntentionMoveRoute; i++ {
		resumed, _ = legs(aboutNPC(w.tickAI(t), h.ObjectID()))
	}
	if got := h.AI().CurrentIntention(); got != ai.IntentionMoveRoute {
		t.Fatalf("intention once the attack desire lost weight = %v, want move_route", got)
	}
	x, y, z := h.Position()
	at := location.Location{X: x, Y: y, Z: z}
	nearest := w.a[patrolID]
	if at.Distance3D(w.b[patrolID]) < at.Distance3D(nearest) {
		nearest = w.b[patrolID]
	}
	if len(resumed) != 1 || resumed[0] != nearest {
		t.Fatalf("legs as the route is taken up again = %v from %v, want one to the nearest node %v", resumed, at, nearest)
	}
}

// A route desire heavier than the attack desire keeps the monster on its
// route: a walker at weight 50, as the primeval walkers queue theirs, does
// not take up an attack desire of 15, and walks on from node to node.
func TestHeavierRouteDesireKeepsTheMonsterOnItsRoute(t *testing.T) {
	t.Parallel()
	w, h := bootPatrol(t, 50)

	w.attack(t, h, 15)
	var shown []npcFrame
	for range 3 {
		shown = append(shown, aboutNPC(w.tickAI(t), h.ObjectID())...)
		if got := h.AI().CurrentIntention(); got != ai.IntentionMoveRoute {
			t.Fatalf("intention with a lighter attack desire queued = %v, want move_route", got)
		}
	}
	w.srv.AdvanceUntil(t, "patrol walks on past its first node", func() bool {
		x, _, _ := h.Position()
		return x > w.a[patrolID].X
	})
	shown = append(shown, aboutNPC(drainFrames(t, w.c), h.ObjectID())...)
	dests, origins := legs(shown)
	if len(dests) == 0 || dests[0] != w.b[patrolID] || origins[0] != w.a[patrolID] {
		t.Fatalf("patrol legs with the attack desire queued = %v from %v, want the first from %v on to %v", dests, origins, w.a[patrolID], w.b[patrolID])
	}
	for _, dest := range dests {
		if dest != w.a[patrolID] && dest != w.b[patrolID] {
			t.Fatalf("patrol legs = %v, want route nodes only", dests)
		}
	}
	for _, fr := range shown {
		if fr.opcode == serverpackets.OpcodeMoveToPawn || fr.opcode == serverpackets.OpcodeAttack {
			t.Fatalf("patrol shown %#x with a lighter attack desire, want it walking its route only", fr.opcode)
		}
	}
}
