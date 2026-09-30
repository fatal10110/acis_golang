package combat

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// landHeldEffect applies the named real debuff to target on its own queue,
// lasting until removed, and returns it once its start hook has run.
func landHeldEffect(t *testing.T, target effectHolder, name string) *effect.Effect {
	t.Helper()
	e, err := effect.New(
		effect.Skill{ID: ccStunSkillID, Level: 1, Debuff: true},
		modelskill.EffectTemplate{Name: name, Count: 1, Time: 300},
	)
	if err != nil {
		t.Fatalf("effect.New(%s): %v", name, err)
	}
	e.Effector, e.Effected = target, target
	done := make(chan struct{})
	if !target.Queue().Post(func() { target.EffectList().Add(e); close(done) }) {
		t.Fatalf("post %s: queue closed", name)
	}
	<-done
	return e
}

// removeHeldEffect ends e on target's own queue, running its exit hook.
func removeHeldEffect(t *testing.T, target effectHolder, e *effect.Effect) {
	t.Helper()
	done := make(chan struct{})
	if !target.Queue().Post(func() { target.EffectList().Remove(e); close(done) }) {
		t.Fatal("post effect removal: queue closed")
	}
	<-done
}

// spawnLatchMonster boots a player next to a fast-swinging monster in
// reach, runs the monster's spawn cycle, then has it wander and arrive so
// its last executed desire is a wander.
func spawnLatchMonster(t *testing.T) (*gameservertest.Server, *npc.Hostile, attackable.Combatant) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	objID := srv.SoleObjectID(t)
	victim := livePlayer(t, srv, objID).(attackable.Combatant)

	x, y, z := srv.PlayerPosition(t, objID)
	tmpl := gameservertest.MovingHostileTemplate("Monster")
	tmpl.AtkSpd = latchAtkSpd
	tmpl.PAtk = 0.25
	tmpl.BaseAttackRange = 40
	at := location.Location{X: x + 20, Y: y, Z: z}
	hostile := srv.SpawnMovingHostileNPCTemplate(t, tmpl, at, at)
	drainUntilQuiet(t, c)

	if err := hostile.TickThink(); err != nil {
		t.Fatalf("spawn-cycle TickThink() error: %v", err)
	}
	wanderThenStop(t, srv, hostile)
	drainUntilQuiet(t, c)
	return srv, hostile, victim
}

// assertNoAttackFrame reads c until it goes quiet and fails on any Attack.
func assertNoAttackFrame(t *testing.T, srv *gameservertest.Server, what string) {
	t.Helper()
	for _, frame := range framesUntilQuiet(srv.Client) {
		if frame[0] == serverpackets.OpcodeAttack {
			t.Fatalf("Attack %s, want none", what)
		}
	}
}

// TestStunnedMonsterTakesLatchOnFirstPassAfterStun pins NpcAI.runAI's
// out-of-control gate: a stunned monster given an attack desire selects
// nothing on event or periodic passes, so its wander stays current and its
// last executed desire stays the wander. The first pass after the stun
// therefore latches the attack, which swings once more after its desire
// decays, and only the pass after that idles.
func TestStunnedMonsterTakesLatchOnFirstPassAfterStun(t *testing.T) {
	t.Parallel()
	srv, hostile, victim := spawnLatchMonster(t)
	stun := landHeldEffect(t, hostile, "Stun")
	drainUntilQuiet(t, srv.Client)

	hostile.AddDamageHate(victim, 0, 100)
	hostile.AddAttackDesire(victim, 5)
	for range 2 {
		if err := hostile.RunAI(); err != nil {
			t.Fatalf("stunned RunAI() error: %v", err)
		}
		if err := hostile.TickThink(); err != nil {
			t.Fatalf("stunned TickThink() error: %v", err)
		}
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionWander {
		t.Fatalf("CurrentIntention() while stunned = %v, want %v kept", got, ai.IntentionWander)
	}
	if !hostile.AI().Desires().Has(&ai.Desire{Kind: ai.IntentionAttack, FinalTarget: victim}) {
		t.Fatal("attack desire left the queue while stunned, want it kept")
	}
	assertNoAttackFrame(t, srv, "while stunned")

	removeHeldEffect(t, hostile, stun)
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("TickThink() after the stun error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() on the first pass after the stun = %v, want %v", got, ai.IntentionAttack)
	}
	readUntil(t, srv.Client, serverpackets.OpcodeAttack, "first swing after the stun")
	attackTime := time.Duration(formulas.TimeBetweenAttacks(hostile.AttackSpeed())) * time.Millisecond
	decayAttackDesire(t, hostile)

	srv.Advance(t, attackTime+10*time.Millisecond)
	readAttackWithout(t, srv, serverpackets.OpcodeChangeMoveType, "latched second swing")
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() after the latched runAI = %v, want %v", got, ai.IntentionAttack)
	}

	srv.Advance(t, attackTime)
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() once the latched swing finished = %v, want %v", got, ai.IntentionIdle)
	}
	assertNoSwingFor(t, srv, attackTime)
}

// TestConfusedMonsterSelectsNoHeavierDesire pins confusion as part of
// runAI's out-of-control gate: the attack its confusion start queues is
// taken up at once, as the confusion does not count until its start has
// run, but a heavier walk queued while it stays confused is not selected
// by event or periodic passes.
func TestConfusedMonsterSelectsNoHeavierDesire(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	startInWorld(t, c)
	px, py, pz := srv.PlayerPosition(t, srv.SoleObjectID(t))
	at := location.Location{X: px + 20, Y: py, Z: pz}
	hostile := srv.SpawnMovingHostileNPCTemplate(t, gameservertest.MovingHostileTemplate("Monster"), at, at)
	drainUntilQuiet(t, c)
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("spawn-cycle TickThink() error: %v", err)
	}

	landHeldEffect(t, hostile, "Confusion")
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() after confusion = %v, want %v", got, ai.IntentionAttack)
	}
	x, y, z := hostile.Position()
	if !hostile.AI().AddMoveToDesire(location.Location{X: x + 200, Y: y, Z: z}, math.MaxFloat64) {
		t.Fatal("AddMoveToDesire() = false, want the walk queued")
	}
	if err := hostile.RunAI(); err != nil {
		t.Fatalf("confused RunAI() error: %v", err)
	}
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("confused TickThink() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() while confused = %v, want %v kept", got, ai.IntentionAttack)
	}

	drainUntilQuiet(t, c)
}

// countAttackFrames lets n swing periods pass and counts the Attack
// broadcasts in them.
func countAttackFrames(t *testing.T, srv *gameservertest.Server, period time.Duration, n int) int {
	t.Helper()
	swings := 0
	for range n {
		srv.Advance(t, period)
		for _, frame := range framesUntilQuiet(srv.Client) {
			if frame[0] == serverpackets.OpcodeAttack {
				swings++
			}
		}
	}
	return swings
}

// TestConfusedMonsterKeepsSwinging pins the finished swing's think past
// runAI's out-of-control gate: runAI selects nothing for a confused
// monster, but the finished attack's think still steps its current attack,
// so it keeps swinging at its target for the whole confusion.
func TestConfusedMonsterKeepsSwinging(t *testing.T) {
	t.Parallel()
	srv, hostile, _, attackTime := startHostileAttack(t, 300, 0)
	landHeldEffect(t, hostile, "Confusion")
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() after confusion = %v, want %v", got, ai.IntentionAttack)
	}
	drainUntilQuiet(t, srv.Client)

	// Few enough periods that the low-level player outlives the swings.
	const periods = 3
	if got := countAttackFrames(t, srv, attackTime+10*time.Millisecond, periods); got < periods {
		t.Fatalf("Attack broadcasts over %d swing periods while confused = %d, want at least %d", periods, got, periods)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() after swinging while confused = %v, want %v", got, ai.IntentionAttack)
	}
}

// TestStunnedMonsterStopsSwinging is the stun side of the finished swing's
// think: a stunned monster's attack step does nothing, so no swing follows
// while the stun holds, and the attack stays its intention.
func TestStunnedMonsterStopsSwinging(t *testing.T) {
	t.Parallel()
	srv, hostile, _, attackTime := startHostileAttack(t, 300, 0)
	landHeldEffect(t, hostile, "Stun")
	drainUntilQuiet(t, srv.Client)

	if got := countAttackFrames(t, srv, attackTime+10*time.Millisecond, 5); got != 0 {
		t.Fatalf("Attack broadcasts while stunned = %d, want 0", got)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() while stunned = %v, want %v kept", got, ai.IntentionAttack)
	}
}

// TestStunnedMonsterIdlesPastLatchAndSwingsItAfter pins the periodic idle
// of an out-of-control monster with a latched attack: the idle runs at
// once instead of waiting behind the latch, and the latch survives it, as
// NpcAI.runAI's _nextDesire survives thinkIdle. The first pass after the
// stun swings once at the latched target and the monster then stops.
func TestStunnedMonsterIdlesPastLatchAndSwingsItAfter(t *testing.T) {
	t.Parallel()
	srv, hostile, _, attackTime := startHostileAttackAfter(t, latchAtkSpd, 0, wanderThenStop)
	stun := landHeldEffect(t, hostile, "Stun")
	decayAttackDesire(t, hostile)
	// Running, so the idle's walk stance shows on the wire.
	hostile.ForceRunStance()
	drainUntilQuiet(t, srv.Client)

	if err := hostile.TickThink(); err != nil {
		t.Fatalf("stunned TickThink() error: %v", err)
	}
	var walk []byte
	for _, frame := range framesUntilQuiet(srv.Client) {
		switch frame[0] {
		case serverpackets.OpcodeAttack:
			t.Fatal("Attack on the stunned periodic idle, want none")
		case serverpackets.OpcodeChangeMoveType:
			walk = frame
		}
	}
	if walk == nil {
		t.Fatal("no ChangeMoveType on the stunned periodic idle, want the walk stance")
	}
	assertChangeMoveType(t, walk, hostile.ObjectID(), false)
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() after the stunned periodic idle = %v, want %v", got, ai.IntentionIdle)
	}

	removeHeldEffect(t, hostile, stun)
	drainUntilQuiet(t, srv.Client)
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("TickThink() after the stun error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() on the first pass after the stun = %v, want the latched %v", got, ai.IntentionAttack)
	}
	readUntil(t, srv.Client, serverpackets.OpcodeAttack, "latched swing after the stun")

	srv.Advance(t, attackTime+10*time.Millisecond)
	if got := hostile.AI().CurrentIntention(); got == ai.IntentionAttack {
		t.Fatalf("CurrentIntention() once the latched swing finished = %v, want the latch spent", got)
	}
	assertNoSwingFor(t, srv, attackTime)
}
