package manager

import (
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"

	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// respawnInto respawns the slots of the population it is pointed at.
type respawnInto struct{ npcs *Npcs }

func (r *respawnInto) Respawn(key string) { r.npcs.Respawn(key) }

// scriptRespawnWorld is a population on a clock the test moves, whose
// respawn task respawns into it, with a standalone master of template 1
// that declares one private of template 2, respawning after 240 s.
type scriptRespawnWorld struct {
	npcs    *Npcs
	state   *world.State
	respawn *task.Respawn
	now     time.Time
	master  *npc.Hostile
}

func newScriptRespawnWorld(t *testing.T) *scriptRespawnWorld {
	t.Helper()
	w := &scriptRespawnWorld{state: world.New(), now: time.Unix(1000, 0)}
	clock := func() time.Time { return w.now }
	into := &respawnInto{}
	decay, _ := task.NewDecay(nopDecayEffects{}, clock)
	w.respawn, _ = task.NewRespawn(into, clock)
	walker, _ := task.NewWalker(nil, noRouteWalkerPath{}, clock, w.state)
	npcs, err := NewNpcs(NewSpawns(&spawn.Table{}, nil), npc.NewTable([]*npc.Template{
		{ID: 1, TemplateID: 1, Type: "Monster", HPMax: 100, RunSpeed: 100, Privates: []npc.PrivateEntry{{NpcID: 2, Weight: 3, RespawnDelay: 240 * time.Second}}},
		{ID: 2, TemplateID: 2, Type: "Monster", HPMax: 100, RunSpeed: 100},
	}), fakeGeo{}, w.state, &sequentialIDs{}, decay, w.respawn, task.NewAI(w.state, zerolog.Nop()), task.NewPositionUpdates(w.state), item.NewTable(nil),
		&recordingGround{}, KillRewardConfig{}, clock, zerolog.Nop(), nil, actorcast.EffectHandlers{}, walker, nil, effect.Env{Activity: task.NewEffects()}, npcQueues(), testMakers())
	if err != nil {
		t.Fatalf("NewNpcs() error: %v", err)
	}
	w.npcs, into.npcs = npcs, npcs
	live, err := npcs.AddSpawn(1, 0, 0, 0, 0, false, 0)
	if err != nil {
		t.Fatalf("AddSpawn() error: %v", err)
	}
	w.master = live.(*npc.Hostile)
	if err := npcs.CreatePrivates(w.master); err != nil {
		t.Fatalf("CreatePrivates() error: %v", err)
	}
	return w
}

// private returns the master's one live private.
func (w *scriptRespawnWorld) private(t *testing.T) *npc.Hostile {
	t.Helper()
	minions := w.master.Minions()
	if len(minions) != 1 {
		t.Fatalf("master's privates = %d, want 1", len(minions))
	}
	return minions[0].(*npc.Hostile)
}

// livePrivates counts the NPCs of template 2 in the world.
func (w *scriptRespawnWorld) livePrivates() int {
	n := 0
	for _, obj := range w.state.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && h.NpcID() == 2 {
			n++
		}
	}
	return n
}

// decay decays h as its corpse would, the respawn its spawn arms
// included.
func (w *scriptRespawnWorld) decay(t *testing.T, h *npc.Hostile) {
	t.Helper()
	if !h.Decay(w.state, w.npcs.RespawnHook(h.ObjectID())) {
		t.Fatal("Decay() = false, want true")
	}
}

// pass moves the clock by d and runs the respawn task.
func (w *scriptRespawnWorld) pass(d time.Duration) {
	w.now = w.now.Add(d)
	w.respawn.Tick()
}

// TestScriptRespawnBringsPrivateBackAfterDelay pins the script respawn
// request (Npc.scheduleRespawn on a private, MinionSpawn.doRespawn): it reads
// the private's declared delay, brings the private back around its live
// master once the delay asked for has passed and not before, and asks
// nothing for a delay that is not positive or of an NPC that is no private.
func TestScriptRespawnBringsPrivateBackAfterDelay(t *testing.T) {
	w := newScriptRespawnWorld(t)
	private := w.private(t)
	if got := w.npcs.RespawnDelay(private); got != 240*time.Second {
		t.Fatalf("private's respawn delay = %v, want 240s", got)
	}
	if got := w.npcs.RespawnDelay(w.master); got != 0 {
		t.Fatalf("standalone master's respawn delay = %v, want 0", got)
	}
	if err := w.npcs.ScheduleRespawn(w.master, time.Second); !errors.Is(err, ErrNotPrivate) {
		t.Fatalf("respawn request for the master = %v, want ErrNotPrivate", err)
	}

	if err := w.npcs.ScheduleRespawn(private, 0); err != nil {
		t.Fatalf("respawn request of 0 = %v, want nil", err)
	}
	w.decay(t, private)
	w.pass(time.Hour)
	if got := w.livePrivates(); got != 0 {
		t.Fatalf("privates after a respawn request of 0 = %d, want none", got)
	}

	if err := w.npcs.ScheduleRespawn(private, 240*time.Second); err != nil {
		t.Fatalf("respawn request after decay = %v", err)
	}
	w.pass(239 * time.Second)
	if got := w.livePrivates(); got != 0 {
		t.Fatalf("privates 239 s after the request = %d, want none", got)
	}
	w.pass(time.Second)
	back := w.private(t)
	if back.ObjectID() == private.ObjectID() || w.livePrivates() != 1 {
		t.Fatalf("privates 240 s after the request = %d (id %d, was %d), want the private back", w.livePrivates(), back.ObjectID(), private.ObjectID())
	}
	if back.Master() != w.master {
		t.Fatal("the respawned private has lost its master")
	}
	if got := back.Scratch().Int(npc.IntWeightPoint); got != 3 {
		t.Fatalf("respawned private's weight point = %d, want its declaration's 3", got)
	}
}

// TestScriptRespawnWaitsForCorpseAndLiveMaster pins doRespawn's two gates: a
// request that comes due while the private's corpse is still in the world
// brings nothing back, and neither does one that comes due once its master
// has died.
func TestScriptRespawnWaitsForCorpseAndLiveMaster(t *testing.T) {
	w := newScriptRespawnWorld(t)
	private := w.private(t)
	if err := w.npcs.ScheduleRespawn(private, time.Second); err != nil {
		t.Fatalf("respawn request = %v", err)
	}
	w.pass(time.Second)
	if got := w.private(t); got != private {
		t.Fatal("a request that came due with the private in the world replaced it")
	}

	if err := w.npcs.ScheduleRespawn(private, time.Second); err != nil {
		t.Fatalf("respawn request = %v", err)
	}
	w.decay(t, private)
	w.master.Kill(nil)
	if !w.master.Dead() {
		t.Fatal("the master did not die")
	}
	w.pass(time.Second)
	if got := w.livePrivates(); got != 0 {
		t.Fatalf("privates after the master died = %d, want none", got)
	}
}

// TestDecayMeHidesWithoutRespawn pins decayMe: the private leaves the world
// with nothing armed to bring it back, its script-created slot goes with it
// and a later respawn request asks nothing, as the reference's doRespawn
// leaves an NPC that is hidden, not decayed, alone.
func TestDecayMeHidesWithoutRespawn(t *testing.T) {
	w := newScriptRespawnWorld(t)
	private := w.private(t)
	before := w.npcs.LiveCount()
	w.npcs.DecayMe(private)
	if _, ok := w.state.Object(private.ObjectID()); ok {
		t.Fatal("the hidden private is still in the world")
	}
	if !private.Decayed() {
		t.Fatal("the hidden private takes more work")
	}
	if got := w.npcs.LiveCount(); got != before-1 {
		t.Fatalf("LiveCount() = %d, want %d", got, before-1)
	}
	if err := w.npcs.ScheduleRespawn(private, time.Second); err != nil {
		t.Fatalf("respawn request for a hidden private = %v, want nil: a private whose slot is gone asks nothing", err)
	}
	w.pass(time.Hour)
	if got := w.livePrivates(); got != 0 {
		t.Fatalf("privates after the hide = %d, want none", got)
	}
	w.npcs.DecayMe(private)
}
