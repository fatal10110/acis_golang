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
