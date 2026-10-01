package npcs

import (
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// folkCorpses is a corpse-decay task on a clock the scenario moves by hand.
// Its effect is the production one: a due actor still in the world decays
// there.
type folkCorpses struct {
	task *task.Decay

	mu    sync.Mutex
	now   time.Time
	state *world.State
}

func newFolkCorpses(t *testing.T) *folkCorpses {
	t.Helper()
	d := &folkCorpses{now: time.Unix(1_700_000_000, 0)}
	decay, err := task.NewDecay(d, d.clock)
	if err != nil {
		t.Fatalf("task.NewDecay: %v", err)
	}
	d.task = decay
	return d
}

func (d *folkCorpses) clock() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.now
}

func (d *folkCorpses) Decay(actor task.DecayActor) {
	d.mu.Lock()
	state := d.state
	d.mu.Unlock()
	obj, ok := state.Object(actor.ObjectID())
	if !ok {
		return
	}
	if corpse, ok := obj.(interface {
		Decay(*world.State, func()) bool
	}); ok {
		corpse.Decay(state, nil)
	}
}

// passAndTick moves the clock on by elapsed and runs one decay sweep.
func (d *folkCorpses) passAndTick(t *testing.T, srv *gameservertest.Server, elapsed time.Duration) {
	t.Helper()
	d.mu.Lock()
	d.now = d.now.Add(elapsed)
	d.state = srv.State
	d.mu.Unlock()
	if err := d.task.Tick(); err != nil {
		t.Fatalf("decay tick: %v", err)
	}
	srv.Settle(t)
}

// frameAt returns the index of the first frame with opcode about object id,
// -1 when none is.
func frameAt(frames [][]byte, opcode byte, id int32) int {
	for i, frame := range frames {
		if objectFrame(frame, opcode, id) {
			return i
		}
	}
	return -1
}

// TestMortalFolkDiesAndItsCorpseDecays pins the death of a civilian NPC
// whose template is not undying (Npc.java:195 setMortal(!isUndying()),
// CreatureStatus.reduceHp: HP floored at 0, doDie under 0.5). The
// player's forced attack empties its HP: the player, its targeter, reads
// the empty health bar, sees it die (Die, no sweep) and leave its attack
// stance (CreatureAI.onEvtDead). Further swings land nothing. The corpse
// stays for the template corpse time (Npc.doDie's DecayTaskManager.add,
// 7 s) and then leaves the world (DeleteObject).
func TestMortalFolkDiesAndItsCorpseDecays(t *testing.T) {
	t.Parallel()
	corpses := newFolkCorpses(t)
	w := bootFolkWorld(t, nil, gameservertest.WithDecay(corpses.task), gameservertest.WithAttackStanceClock(time.Now))
	tmpl := folkTemplate("Folk", 13031)
	tmpl.Undying = false
	tmpl.HPMax = 1
	f := w.spawnFolk(t, tmpl, 40)
	w.selectFolk(t, f)

	w.c.Send(encodeAttackRequest(f.ObjectID(), w.at))
	var frames [][]byte
	w.srv.AdvanceUntil(t, "the civilian NPC dying to the swing", func() bool {
		frames = append(frames, drainFrames(t, w.c)...)
		return f.Dead()
	})
	frames = append(frames, drainFrames(t, w.c)...)

	if f.HP() != 0 || !f.AlikeDead() || !f.NPCInfoSnapshot().AlikeDead {
		t.Fatalf("dead NPC: HP %v, alike dead %v, NpcInfo alike dead %v; want 0 HP and a corpse", f.HP(), f.AlikeDead(), f.NPCInfoSnapshot().AlikeDead)
	}
	status := -1
	for i, frame := range frames {
		if hp, ok := statusHP(frame, f.ObjectID()); ok && hp == 0 {
			status = i
			break
		}
	}
	die := frameAt(frames, serverpackets.OpcodeDie, f.ObjectID())
	stop := frameAt(frames, serverpackets.OpcodeAutoAttackStop, f.ObjectID())
	if status < 0 || die < status || stop < die {
		t.Fatalf("death frames %x: StatusUpdate at 0 HP at %d, Die at %d, AutoAttackStop at %d; want them in that order", opcodes(frames), status, die, stop)
	}
	r := wire.NewReader(frames[die][1:])
	r.ReadInt32()
	r.ReadInt32()
	r.ReadInt32()
	r.ReadInt32()
	r.ReadInt32()
	if sweep := r.ReadInt32(); sweep != 0 {
		t.Fatalf("Die sweepable = %d, want 0 for a civilian NPC", sweep)
	}
	if !f.HasCorpse() {
		t.Fatal("dead NPC reports no corpse awaiting decay")
	}

	if f.TakeDamage(10, nil) || f.HP() != 0 {
		t.Fatalf("a hit on the corpse: HP %v, want nothing taken", f.HP())
	}

	corpses.passAndTick(t, w.srv, 6*time.Second)
	if _, ok := w.srv.State.Object(f.ObjectID()); !ok {
		t.Fatal("corpse left the world before its corpse time")
	}
	if i := frameAt(drainFrames(t, w.c), serverpackets.OpcodeDeleteObject, f.ObjectID()); i >= 0 {
		t.Fatal("corpse deleted before its corpse time")
	}
	corpses.passAndTick(t, w.srv, time.Second)
	if _, ok := w.srv.State.Object(f.ObjectID()); ok {
		t.Fatal("corpse still in the world after its corpse time")
	}
	if i := frameAt(drainFrames(t, w.c), serverpackets.OpcodeDeleteObject, f.ObjectID()); i < 0 {
		t.Fatal("the player saw no DeleteObject for the decayed corpse")
	}
}

// TestUndyingFolkSurvivesLethalHit pins the undying floor
// (CreatureStatus.reduceHp: Math.max(hp - value, 1)): the same lethal
// swing leaves a service NPC at 1 HP, alive, with no Die shown and no
// corpse registered.
func TestUndyingFolkSurvivesLethalHit(t *testing.T) {
	t.Parallel()
	corpses := newFolkCorpses(t)
	w := bootFolkWorld(t, nil, gameservertest.WithDecay(corpses.task))
	tmpl := folkTemplate("Merchant", merchantID)
	tmpl.HPMax = 2
	f := w.spawnFolk(t, tmpl, 40)
	w.selectFolk(t, f)

	w.c.Send(encodeAttackRequest(f.ObjectID(), w.at))
	var frames [][]byte
	w.srv.AdvanceUntil(t, "a swing landing on the merchant", func() bool {
		frames = append(frames, drainFrames(t, w.c)...)
		return f.HP() < 2
	})
	w.srv.Advance(t, 3*time.Second)
	frames = append(frames, drainFrames(t, w.c)...)

	if f.Dead() || f.HP() != 1 {
		t.Fatalf("undying merchant: dead %v, HP %v; want alive at 1 HP", f.Dead(), f.HP())
	}
	if i := frameAt(frames, serverpackets.OpcodeDie, f.ObjectID()); i >= 0 {
		t.Fatalf("undying merchant shown dying: %x", opcodes(frames))
	}
	if corpses.task.Tracked(f) {
		t.Fatal("undying merchant registered for corpse decay")
	}
}

// statusHP reads the current HP a StatusUpdate frame reports for id.
func statusHP(frame []byte, id int32) (int32, bool) {
	if frame[0] != serverpackets.OpcodeStatusUpdate {
		return 0, false
	}
	r := wire.NewReader(frame[1:])
	if r.ReadInt32() != id {
		return 0, false
	}
	n := r.ReadInt32()
	for range n {
		attr, value := r.ReadInt32(), r.ReadInt32()
		if serverpackets.StatusType(attr) == serverpackets.StatusCurrentHP {
			return value, true
		}
	}
	return 0, false
}

// TestMortalWalkerFolkStopsWhenItDies pins Creature.doDie's abortAll and
// CreatureAI.onEvtDead's idle intention for a civilian NPC walking its
// route: observers see it stop (StopMove) before it falls (Die), and it
// walks no further leg.
func TestMortalWalkerFolkStopsWhenItDies(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	at := location.Location{X: w.at.X + 150, Y: w.at.Y, Z: w.at.Z}
	a := location.Location{X: w.at.X + 100, Y: w.at.Y, Z: w.at.Z}
	b := location.Location{X: w.at.X + 400, Y: w.at.Y, Z: w.at.Z}
	tmpl := walkerTemplate()
	tmpl.Undying = false
	f, walker := w.srv.SpawnRouteFolkNPCAt(t, tmpl, at, walkerRoutes(a, b), true)
	drainFrames(t, w.c)
	if !f.IsMoving() {
		t.Fatal("walker is not walking its route")
	}

	if !f.TakeDamage(1_000_000, nil) {
		t.Fatal("a lethal hit did not kill the mortal walker")
	}
	frames := drainFrames(t, w.c)
	stop := frameAt(frames, serverpackets.OpcodeStopMove, f.ObjectID())
	die := frameAt(frames, serverpackets.OpcodeDie, f.ObjectID())
	if stop < 0 || die < stop {
		t.Fatalf("death frames %x: StopMove at %d, Die at %d; want the stop then the death", opcodes(frames), stop, die)
	}
	if f.IsMoving() {
		t.Fatal("dead walker still walking")
	}

	w.srv.Advance(t, 5*time.Second)
	walker.Tick()
	if dests, _ := folkMoves(drainFrames(t, w.c), f); len(dests) != 0 {
		t.Fatalf("dead walker walked on to %v", dests)
	}
}
