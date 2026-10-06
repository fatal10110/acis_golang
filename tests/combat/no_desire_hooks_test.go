package combat

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The template ids of the idle and arrival scenarios: two hostile and one
// civilian id a test behavior is bound to, and one of each kind it is not.
const (
	idleStander   = int32(20030)
	idleWanderer  = int32(20031)
	idleFolk      = int32(30100)
	idleUnbound   = int32(20050)
	idleFolkFree  = int32(30101)
	idleBoundPriv = int32(20032)
	idleFreePriv  = int32(20051)
)

// idleLog records, one line per call, the no-desire, move-finished and
// out-of-territory hooks of a test behavior. Hooks run on each NPC's own
// queue, so every field is guarded by mu.
type idleLog struct {
	mu sync.Mutex
	// names maps an object id to the name the lines use.
	names map[int32]string
	// running reads, per object id, whether the NPC is in its run stance.
	running map[int32]func() bool
	// intention reads, per object id, a hostile NPC's current intention.
	intention map[int32]func() ai.Intention
	lines     []string
}

func newIdleLog() *idleLog {
	return &idleLog{names: map[int32]string{}, running: map[int32]func() bool{}, intention: map[int32]func() ai.Intention{}}
}

func (l *idleLog) trackHostile(name string, h *npc.Hostile) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.names[h.ObjectID()] = name
	l.running[h.ObjectID()] = h.Running
	l.intention[h.ObjectID()] = h.AI().CurrentIntention
}

func (l *idleLog) trackFolk(name string, f *npc.Folk) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.names[f.ObjectID()] = name
	l.running[f.ObjectID()] = f.Running
}

func (l *idleLog) add(format string, args ...any) {
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

// take returns the lines since the last call, sorted: NPCs on different
// queues record in no fixed order.
func (l *idleLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	lines := l.lines
	l.lines = nil
	slices.Sort(lines)
	return lines
}

// noDesire records a no-desire hook call; that of wanderer also queues a
// wander of timer 7 and weight 9.
func (l *idleLog) noDesire(wanderer int32) func(*script.Script, script.NoDesire) {
	return func(_ *script.Script, e script.NoDesire) {
		l.mu.Lock()
		id := e.NPC.ObjectID()
		line := fmt.Sprintf("NO_DESIRE %s running=%v", l.names[id], l.running[id]())
		if intention := l.intention[id]; intention != nil {
			line += fmt.Sprintf(" intention=%v", intention())
		}
		l.lines = append(l.lines, line)
		l.mu.Unlock()
		if e.NPC.NpcID() == wanderer {
			e.NPC.AddWanderDesire(7, 9)
		}
	}
}

// hostileBehavior returns the catalog entry of a behavior bound to the
// hostile ids that records its idle and arrival hooks.
func (l *idleLog) hostileBehavior(wanderer int32, ids ...int32) func() script.Script {
	return func() script.Script {
		return script.Script{Behavior: true, NPCs: ids, Hooks: script.Hooks{
			OnNoDesire: l.noDesire(wanderer),
			OnMoveToFinished: func(_ *script.Script, e script.MoveToFinished) {
				l.mu.Lock()
				defer l.mu.Unlock()
				l.add("MOVE_TO_FINISHED %s at=%d,%d,%d", l.names[e.NPC.ObjectID()], e.X, e.Y, e.Z)
			},
			OnOutOfTerritory: func(_ *script.Script, e script.OutOfTerritory) {
				l.mu.Lock()
				defer l.mu.Unlock()
				l.add("OUT_OF_TERRITORY %s", l.names[e.NPC.ObjectID()])
			},
		}}
	}
}

// folkBehavior returns the catalog entry of a behavior bound to the
// civilian ids that records its no-desire hook: a civilian NPC keeps no
// territory, so a behavior bound to one that waits on leaving it would be
// refused. Its walks and move-finished hook are covered in tests/ai.
func (l *idleLog) folkBehavior(ids ...int32) func() script.Script {
	return func() script.Script {
		return script.Script{Behavior: true, NPCs: ids, Hooks: script.Hooks{OnNoDesire: l.noDesire(0)}}
	}
}

// bootIdleHooks boots one character in the world, with the AI task, and
// the recording behavior bound to the bound ids.
func bootIdleHooks(t *testing.T, log *idleLog, opts ...gameservertest.Option) *gameservertest.Server {
	t.Helper()
	kinds := map[int32]script.NPCKind{
		idleStander: script.KindHostile, idleWanderer: script.KindHostile, idleBoundPriv: script.KindHostile,
		idleUnbound: script.KindHostile, idleFreePriv: script.KindHostile,
		idleFolk: script.KindFolk, idleFolkFree: script.KindFolk,
	}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAITask(),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: "ai.IdleRecorder"}, {Path: "ai.FolkIdleRecorder"}}, script.Catalog{
			"ai.IdleRecorder":     log.hostileBehavior(idleWanderer, idleStander, idleWanderer, idleBoundPriv),
			"ai.FolkIdleRecorder": log.folkBehavior(idleFolk),
		}),
	}, opts...)...)
	startInWorld(t, srv.Client)
	return srv
}

// idleTemplate is a movable monster template with id and AI params.
func idleTemplate(id int32, params npc.AIParams) *npc.Template {
	tmpl := gameservertest.MovingHostileTemplate("Monster")
	tmpl.ID, tmpl.TemplateID, tmpl.AIParams = int(id), int(id), params
	return tmpl
}

// TestNoDesireRunsBoundBehaviorsInsteadOfIdleStandIns pins the no-desire
// dispatch per NPC id on the periodic AI cycle. An NPC with nothing to do,
// hostile or civilian, runs its bound behavior's no-desire hook once per
// cycle from its second one on, already in its walk stance and idle; what
// the hook queues is what the NPC does next. A bound NPC queues no built-in
// idle wander or escort follow; an unbound one still does. Both executors
// run it: the hooks run on each NPC's own queue.
func TestNoDesireRunsBoundBehaviorsInsteadOfIdleStandIns(t *testing.T) {
	t.Parallel()
	for _, exec := range []struct {
		name string
		opts []gameservertest.Option
	}{
		{"inline", nil},
		{"pool", []gameservertest.Option{gameservertest.WithRealPool()}},
	} {
		t.Run(exec.name, func(t *testing.T) {
			t.Parallel()
			noDesireScenario(t, exec.opts...)
		})
	}
}

func noDesireScenario(t *testing.T, opts ...gameservertest.Option) {
	log := newIdleLog()
	srv := bootIdleHooks(t, log, opts...)
	x, y, z := srv.PlayerPosition(t, srv.SoleObjectID(t))
	at := func(i int) location.Location { return location.Location{X: x + 200 + 40*i, Y: y, Z: z} }
	escort := npc.AIParams{"Party_Type": "1"}

	hostiles := map[string]*npc.Hostile{}
	for i, spec := range []struct {
		name   string
		id     int32
		params npc.AIParams
	}{
		{"stander", idleStander, nil},
		{"wanderer", idleWanderer, nil},
		{"unbound", idleUnbound, nil},
		{"master", idleUnbound, nil},
		{"boundPriv", idleBoundPriv, escort},
		{"freePriv", idleFreePriv, escort},
	} {
		h := srv.SpawnMovingHostileNPCTemplate(t, idleTemplate(spec.id, spec.params), at(i), at(i))
		hostiles[spec.name] = h
		log.trackHostile(spec.name, h)
	}
	for _, priv := range []*npc.Hostile{hostiles["boundPriv"], hostiles["freePriv"]} {
		priv.SetMaster(hostiles["master"])
		hostiles["master"].AddMinion(priv)
	}
	for _, h := range hostiles {
		srv.AI.Add(h)
	}
	folk := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", int(idleFolk)), at(7))
	log.trackFolk("folk", folk)
	free := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", int(idleFolkFree)), at(8))
	log.trackFolk("freeFolk", free)
	drainUntilQuiet(t, srv.Client)

	runAICycle(t, srv)
	if got := log.take(); len(got) != 0 {
		t.Fatalf("hooks on the first cycle = %q, want none: nothing idles before its first cycle", got)
	}

	runAICycle(t, srv)
	want := []string{
		"NO_DESIRE boundPriv running=false intention=idle",
		"NO_DESIRE folk running=false",
		"NO_DESIRE stander running=false intention=idle",
		"NO_DESIRE wanderer running=false intention=idle",
	}
	if got := log.take(); !slices.Equal(got, want) {
		t.Fatalf("hooks on the second cycle =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	for name, wantDesire := range map[string]*ai.Desire{
		"stander":   nil,
		"boundPriv": nil,
		"wanderer":  {Kind: ai.IntentionWander, Timer: 7, Weight: 9},
		"unbound":   {Kind: ai.IntentionWander, Timer: 5, Weight: 5},
		"freePriv":  {Kind: ai.IntentionFollow, FinalTarget: hostiles["master"]},
	} {
		d, ok := hostiles[name].AI().Desires().Peek()
		switch {
		case wantDesire == nil && ok:
			t.Errorf("%s queued %+v after its idle, want nothing: the behavior queued nothing", name, d)
		case wantDesire != nil && !ok:
			t.Errorf("%s queued nothing after its idle, want %+v", name, wantDesire)
		case wantDesire != nil && (d.Kind != wantDesire.Kind || d.Timer != wantDesire.Timer ||
			(wantDesire.Kind != ai.IntentionFollow && d.Weight != wantDesire.Weight) ||
			(wantDesire.FinalTarget != nil && (d.FinalTarget == nil || d.FinalTarget.ObjectID() != wantDesire.FinalTarget.ObjectID()))):
			t.Errorf("%s queued %+v after its idle, want %+v", name, d, wantDesire)
		}
	}

	// Still with nothing to do, the bound standers idle again next cycle.
	runAICycle(t, srv)
	got := slices.DeleteFunc(log.take(), func(l string) bool { return strings.HasPrefix(l, "NO_DESIRE wanderer") })
	want = []string{
		"NO_DESIRE boundPriv running=false intention=idle",
		"NO_DESIRE folk running=false",
		"NO_DESIRE stander running=false intention=idle",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("hooks on the third cycle =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// walkUntilArrived ticks h's movement until its walk ends.
func walkUntilArrived(t *testing.T, srv *gameservertest.Server, h *npc.Hostile) {
	t.Helper()
	for i := 0; h.Move().Moving(); i++ {
		if i >= int(20*time.Second/move.PositionUpdateInterval) {
			t.Fatal("walk never arrived")
		}
		srv.TickPositions()
	}
	srv.Settle(t)
}

// TestMoveFinishedAndOutOfTerritoryHooks pins the arrival hooks of a bound
// monster's walks to a point: each arrival runs the move-finished hook with
// the position it reached, and the first arrival outside its territory then
// runs the out-of-territory hook, again only after an arrival back inside.
// The cycle after a walk idles the monster, still on its finished walk, and
// as its behavior queues nothing the no-desire hook runs twice.
func TestMoveFinishedAndOutOfTerritoryHooks(t *testing.T) {
	t.Parallel()
	log := newIdleLog()
	srv := bootIdleHooks(t, log)
	x, y, z := srv.PlayerPosition(t, srv.SoleObjectID(t))
	home := location.Location{X: x + 400, Y: y, Z: z}
	h := srv.SpawnMovingHostileNPCTemplate(t, idleTemplate(idleStander, nil), home, home)
	log.trackHostile("stander", h)
	srv.AI.Add(h)
	drainUntilQuiet(t, srv.Client)
	runAICycle(t, srv)

	walkTo := func(dest location.Location, want ...string) {
		t.Helper()
		if !h.AI().AddMoveToDesire(dest, 1_000) {
			t.Fatalf("AddMoveToDesire(%+v) = false, want the walk queued", dest)
		}
		runAICycle(t, srv)
		if got := log.take(); len(got) != 0 {
			t.Fatalf("hooks on the cycle that starts the walk to %+v = %q, want none", dest, got)
		}
		walkUntilArrived(t, srv, h)
		if hx, hy, hz := h.Position(); (location.Location{X: hx, Y: hy, Z: hz}) != dest {
			t.Fatalf("position after the walk = %d,%d,%d, want %+v", hx, hy, hz, dest)
		}
		if got := log.take(); !slices.Equal(got, want) {
			t.Fatalf("hooks on the arrival at %+v = %q, want %q", dest, got, want)
		}
		runAICycle(t, srv)
		idle := []string{"NO_DESIRE stander running=false intention=idle", "NO_DESIRE stander running=false intention=idle"}
		if got := log.take(); !slices.Equal(got, idle) {
			t.Fatalf("hooks on the cycle after the walk to %+v = %q, want %q", dest, got, idle)
		}
	}
	finished := func(l location.Location) string {
		return fmt.Sprintf("MOVE_TO_FINISHED stander at=%d,%d,%d", l.X, l.Y, l.Z)
	}

	out := location.Location{X: home.X, Y: home.Y + 300, Z: home.Z}
	walkTo(out, finished(out), "OUT_OF_TERRITORY stander")
	further := location.Location{X: home.X, Y: home.Y + 400, Z: home.Z}
	walkTo(further, finished(further))
	walkTo(home, finished(home))
	walkTo(out, finished(out), "OUT_OF_TERRITORY stander")
}

// TestMoveFinishedHookOnFleeArrival pins the move-finished hook on the end
// of a flight, with the position the bound monster fled to.
func TestMoveFinishedHookOnFleeArrival(t *testing.T) {
	t.Parallel()
	log := newIdleLog()
	srv := bootIdleHooks(t, log)
	x, y, z := srv.PlayerPosition(t, srv.SoleObjectID(t))
	// Off the player's row: a flight from a creature on the same row has no
	// run-off direction and goes nowhere.
	home := location.Location{X: x + 100, Y: y + 100, Z: z}
	h := srv.SpawnMovingHostileNPCTemplate(t, idleTemplate(idleStander, nil), home, home)
	log.trackHostile("stander", h)
	srv.AI.Add(h)
	drainUntilQuiet(t, srv.Client)
	runAICycle(t, srv)

	h.AI().AddFleeDesire(liveCombatant(t, srv), 150, 1_000)
	runAICycle(t, srv)
	if got := h.AI().CurrentIntention(); got != ai.IntentionFlee || !h.Move().Moving() {
		t.Fatalf("after the flee cycle: intention %v, moving %v; want a flight under way", got, h.Move().Moving())
	}
	if !h.Running() {
		t.Fatal("the monster flees at a walk; want its run stance")
	}
	walkUntilArrived(t, srv, h)
	hx, hy, hz := h.Position()
	want := []string{fmt.Sprintf("MOVE_TO_FINISHED stander at=%d,%d,%d", hx, hy, hz)}
	if got := log.take(); !slices.Equal(got, want) {
		t.Fatalf("hooks on the flight's end = %q, want %q", got, want)
	}
	if (location.Location{X: hx, Y: hy, Z: hz}) == home {
		t.Fatal("the monster never left its spawn")
	}

	// The idle after the flight switches back to the walk stance before the
	// no-desire hook runs.
	runAICycle(t, srv)
	got := log.take()
	if len(got) == 0 {
		t.Fatal("no no-desire hook on the cycle after the flight")
	}
	for _, l := range got {
		if !strings.HasPrefix(l, "NO_DESIRE stander running=false") {
			t.Fatalf("hooks on the cycle after the flight = %q, want only no-desire calls in the walk stance", got)
		}
	}
}
