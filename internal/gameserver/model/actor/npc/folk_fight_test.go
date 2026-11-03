package npc

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// scriptedFolkAttack is an attack controller that records the swings it
// starts and answers the attack gates from its fields.
type scriptedFolkAttack struct {
	attacking bool
	hitAnim   bool
	refuse    bool
	swings    []int32
}

func (a *scriptedFolkAttack) BowCoolingDown() bool                { return false }
func (a *scriptedFolkAttack) AttackingNow() bool                  { return a.attacking }
func (a *scriptedFolkAttack) InHitAnimation() bool                { return a.hitAnim }
func (a *scriptedFolkAttack) CanAttack(attackable.Combatant) bool { return !a.refuse && !a.attacking }

func (a *scriptedFolkAttack) Stop() { a.attacking, a.hitAnim = false, false }

func (a *scriptedFolkAttack) DoAttack(target attackable.Combatant) {
	a.swings = append(a.swings, target.ObjectID())
	a.attacking, a.hitAnim = true, true
}

// folkFightRig is a civilian NPC at (100, 0, 0) with a reach of 100, wired
// to a scripted attack controller, with two player-kind targets it knows.
type folkFightRig struct {
	f      *Folk
	in     *sim.Inline
	attack *scriptedFolkAttack
	events *recordingSink
	near   *hostileTarget
	far    *hostileTarget
}

// newFolkFightRig builds the rig with near 50 and far 80 units east of the
// NPC, before the NPC's first AI tick.
func newFolkFightRig(t *testing.T) *folkFightRig {
	t.Helper()
	inst, err := NewInstance(1, &Template{ID: 31677, TemplateID: 31677, Type: "Folk", Level: 80, HPMax: 2880, RunSpeed: 120, WalkSpeed: 50, BaseAttackRange: 100})
	if err != nil {
		t.Fatal(err)
	}
	f, err := NewFolk(inst)
	if err != nil {
		t.Fatal(err)
	}
	r := &folkFightRig{
		f: f, in: sim.NewInline(time.Unix(0, 0)), attack: &scriptedFolkAttack{}, events: &recordingSink{},
		near: &hostileTarget{id: 2, playable: true}, far: &hostileTarget{id: 3, playable: true},
	}
	w := world.New()
	if err := f.Attach(FolkRuntime{World: w, Queue: r.in.NewQueue("folk"), Sink: r.events, AI: &recordingFolkAI{}}); err != nil {
		t.Fatal(err)
	}
	f.SetAttacker(r.attack)
	w.Spawn(f, 100, 0, 0, 0)
	w.Spawn(r.near, 150, 0, 0, 0)
	w.Spawn(r.far, 180, 0, 0, 0)
	return r
}

func (r *folkFightRig) tick(t *testing.T) {
	t.Helper()
	if err := r.f.TickThink(); err != nil {
		t.Fatalf("TickThink() = %v", err)
	}
	r.in.Run()
}

// finishSwing ends the swing in flight the way the controller does: the
// hit animation closes, then the swing finishes.
func (r *folkFightRig) finishSwing() {
	r.attack.attacking, r.attack.hitAnim = false, false
	r.f.AttackEvents().Emit(event.AttackFinished{})
	r.in.Run()
}

// TestFolkAttackDesireRunsAtOnceBeforeFirstTick follows NpcAI.addAttackDesire
// 713-728 and runAI 451, 508: an NPC that hates no one runs desire
// selection as the attack desire is queued, and an attack desire opens the
// selection before the NPC's first AI tick, which a do-nothing does not.
func TestFolkAttackDesireRunsAtOnceBeforeFirstTick(t *testing.T) {
	r := newFolkFightRig(t)
	r.f.AddDoNothingDesire(40, 1e6)
	r.f.runAI(false)
	if got := r.f.CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("intention = %v before the first tick, want idle: only an attack opens the selection", got)
	}
	r.f.ClearDesires()

	r.f.AddAttackDesire(r.near, 2000)
	r.in.Run()
	if len(r.attack.swings) != 1 || r.attack.swings[0] != r.near.ObjectID() {
		t.Fatalf("swings = %v, want one at %d at once", r.attack.swings, r.near.ObjectID())
	}
	if got := r.f.CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("intention = %v, want attack", got)
	}
	if c, ok := r.f.MostHated(); !ok || c.ObjectID() != r.near.ObjectID() {
		t.Fatalf("most hated = %v, %v, want the attack's target", c, ok)
	}

	// A second attack desire on a hated NPC waits for the next selection.
	r.f.AddAttackDesire(r.far, 5000)
	r.in.Run()
	if len(r.attack.swings) != 1 {
		t.Fatalf("swings = %v, want no selection while the NPC hates someone", r.attack.swings)
	}
}

// TestFolkFinishedSwingGoesOnWithTheAttack follows CreatureAI.thinkAttack
// 108-112 and onEvtFinishedAttack 23-29: a step meeting a swing in flight
// puts the attack aside, and the swing's end takes it up again.
func TestFolkFinishedSwingGoesOnWithTheAttack(t *testing.T) {
	r := newFolkFightRig(t)
	r.f.AddAttackDesire(r.near, 2000)
	r.in.Run()
	r.attack.hitAnim = false
	r.tick(t)
	if len(r.attack.swings) != 1 || r.f.fight.next == nil {
		t.Fatalf("swings = %v, next = %v; want the attack put aside while the swing is in flight", r.attack.swings, r.f.fight.next)
	}
	r.finishSwing()
	if len(r.attack.swings) != 2 {
		t.Fatalf("swings = %v after the swing finished, want a second one", r.attack.swings)
	}
}

// TestFolkLatchedAttackOutlivesItsDesire follows NpcAI.runAI 512-536: an
// attack taken up right after the NPC idled is latched, so the next
// selection takes it up again although its desire has left the queue; the
// one after idles.
func TestFolkLatchedAttackOutlivesItsDesire(t *testing.T) {
	r := newFolkFightRig(t)
	r.tick(t)
	r.f.AddAttackDesire(r.near, 2000)
	r.in.Run()
	r.attack.attacking, r.attack.hitAnim = false, false
	r.f.cast.desires.RemoveKind(ai.IntentionAttack)

	r.tick(t)
	if len(r.attack.swings) != 2 {
		t.Fatalf("swings = %v, want the latched attack taken up again", r.attack.swings)
	}
	r.attack.attacking, r.attack.hitAnim = false, false
	r.tick(t)
	if len(r.attack.swings) != 2 {
		t.Fatalf("swings = %v, want no swing once the latch is spent", r.attack.swings)
	}
	if got := r.f.CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("intention = %v, want idle on the empty queue", got)
	}
}

// TestFolkHitAnimationHoldsSelection follows NpcAI.runAI 517-531: during
// the hit animation no desire is taken up, however heavy; once it ends the
// latched attack goes first, then the heaviest desire.
func TestFolkHitAnimationHoldsSelection(t *testing.T) {
	r := newFolkFightRig(t)
	r.f.AddAttackDesire(r.near, 2000)
	r.in.Run()
	r.attack.attacking = false
	r.f.AddAttackDesire(r.far, 1e6)
	r.tick(t)
	r.tick(t)
	if len(r.attack.swings) != 1 {
		t.Fatalf("swings = %v during the hit animation, want none taken up", r.attack.swings)
	}
	r.attack.hitAnim = false
	r.tick(t)
	r.attack.attacking, r.attack.hitAnim = false, false
	r.tick(t)
	want := []int32{r.near.ObjectID(), r.near.ObjectID(), r.far.ObjectID()}
	if len(r.attack.swings) != len(want) || r.attack.swings[1] != want[1] || r.attack.swings[2] != want[2] {
		t.Fatalf("swings = %v once the animation ended, want %v: the latched attack, then the heavier one", r.attack.swings, want)
	}
}

// TestFolkAggroListTracksHitsAndDecays follows Npc.reduceCurrentHp 395 and
// NpcAI.runAI 558-569 with AggroList.refresh and reduceAllHate: a hit adds
// its damage at no hate; every third tick hate and attack desires lose 6.6,
// and once the NPC hates no one its list empties and its attack desires go.
func TestFolkAggroListTracksHitsAndDecays(t *testing.T) {
	r := newFolkFightRig(t)
	r.f.TakeDamage(30, r.near)
	if _, ok := r.f.MostHated(); ok {
		t.Fatal("a hit raised hate; it adds damage only")
	}
	if got, _ := r.f.fight.aggro.Get(r.near); got.Damage != 30 {
		t.Fatalf("aggro damage = %v, want 30", got.Damage)
	}
	r.f.AddAttackDesireHold(r.far, 10)
	r.in.Run()
	r.attack.attacking, r.attack.hitAnim = false, false
	for range 3 {
		r.tick(t)
		r.attack.attacking, r.attack.hitAnim = false, false
	}
	want := 10.0
	want -= folkAttackHateDecay
	if got, _ := r.f.fight.aggro.Get(r.far); got.Hate != want {
		t.Fatalf("hate after three ticks = %v, want %v", got.Hate, want)
	}
	for range 3 {
		r.tick(t)
		r.attack.attacking, r.attack.hitAnim = false, false
	}
	if !r.f.fight.aggro.IsEmpty() {
		t.Fatal("aggro list kept once the NPC hates no one")
	}
	if r.f.cast.desires.HasKind(ai.IntentionAttack) {
		t.Fatal("attack desire kept once the NPC hates no one")
	}
}

// TestFolkStopHateDropsTheAttack follows AggroList.stopHate 85-101: the
// hate stops, the NPC goes back to peace when it hates no one, and its
// attack desire on the creature goes.
func TestFolkStopHateDropsTheAttack(t *testing.T) {
	r := newFolkFightRig(t)
	r.f.AddAttackDesire(r.near, 2000)
	r.f.AddAttackDesire(r.far, 1000)
	r.in.Run()
	r.f.StopHate(r.near)
	if c, ok := r.f.MostHated(); !ok || c.ObjectID() != r.far.ObjectID() {
		t.Fatalf("most hated = %v, %v, want the other target", c, ok)
	}
	if r.f.cast.desires.Has(&ai.Desire{Kind: ai.IntentionAttack, FinalTarget: r.near}) {
		t.Fatal("attack desire on the stopped target kept")
	}
	r.f.StopHate(r.far)
	if !r.f.fight.aggro.IsEmpty() {
		t.Fatal("aggro list kept once the NPC hates no one")
	}
}

// TestFolkAttackOnFarTargetIsDropped follows NpcAI.runAI 505: an attack
// desire on a creature more than 1500 away leaves the queue.
func TestFolkAttackOnFarTargetIsDropped(t *testing.T) {
	r := newFolkFightRig(t)
	away := &hostileTarget{id: 4, playable: true}
	r.f.world.Spawn(away, 100+1501, 0, 0, 0)
	r.f.AddAttackDesireHold(away, 2000)
	r.in.Run()
	if r.f.cast.desires.HasKind(ai.IntentionAttack) || len(r.attack.swings) != 0 {
		t.Fatalf("attack on a creature 1501 away kept (swings %v)", r.attack.swings)
	}
}

// TestFolkAttackWaitsOutOfReach follows CreatureMove.maybeStartOffensiveFollow
// 393-418 for an NPC that cannot move: out of reach it waits, the desire
// queued.
func TestFolkAttackWaitsOutOfReach(t *testing.T) {
	r := newFolkFightRig(t)
	away := &hostileTarget{id: 4, playable: true}
	r.f.world.Spawn(away, 100+300, 0, 0, 0)
	r.f.AddAttackDesire(away, 2000)
	r.in.Run()
	if len(r.attack.swings) != 0 || !r.f.cast.desires.HasKind(ai.IntentionAttack) {
		t.Fatalf("swings = %v, want none and the desire kept", r.attack.swings)
	}
}

// TestFolkDeathStopsTheSwing: a dying NPC's swing in flight stops.
func TestFolkDeathStopsTheSwing(t *testing.T) {
	r := newFolkFightRig(t)
	r.f.AddAttackDesire(r.near, 2000)
	r.in.Run()
	r.f.Kill(r.near)
	r.in.Run()
	if r.attack.attacking {
		t.Fatal("swing kept in flight through death")
	}
	if r.f.cast.desires.Len() != 0 {
		t.Fatal("desires kept through death")
	}
}
