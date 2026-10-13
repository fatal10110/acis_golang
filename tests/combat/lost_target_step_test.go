package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// TestAttackStepOnDeadVictimKeepsAttackAndHate pins the attack step's
// lost-target check (CreatureAI.thinkAttack -> AbstractAI.isTargetLost: no
// target, not in the world, or not known): a victim that died where the
// monster still sees it is not lost. A continue step (a control effect
// ending) keeps the attack, the hate and the attack desire, and the attack
// gate refuses the corpse. Only the periodic hate refresh stops the dead
// victim's hate and drops its desire.
func TestAttackStepOnDeadVictimKeepsAttackAndHate(t *testing.T) {
	t.Parallel()
	srv, hostile, victim, _ := startHostileAttack(t, 300, 0)
	hate := hostile.AggroHate(victim)
	if hate <= 0 {
		t.Fatalf("setup: hate on the victim = %v, want positive", hate)
	}
	onPlayerQueue(t, srv, victim.ObjectID(), func(pc *player.Character) { pc.Kill(nil) })
	srv.Settle(t)
	if !victim.Dead() || !hostile.Knows(victim) {
		t.Fatalf("setup: victim dead %v, known %v; want a dead victim the monster still knows", victim.Dead(), hostile.Knows(victim))
	}

	if err := hostile.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() after the step = %v, want the attack kept", got)
	}
	if got := hostile.AggroHate(victim); got != hate {
		t.Fatalf("hate on the dead victim after the step = %v, want %v kept", got, hate)
	}
	attack := &ai.Desire{Kind: ai.IntentionAttack, FinalTarget: victim}
	if !hostile.AI().Desires().Has(attack) {
		t.Fatal("attack desire on the dead victim dropped by the step")
	}

	for range 3 {
		hostile.Tick()
		if err := hostile.TickThink(); err != nil {
			t.Fatalf("TickThink() error: %v", err)
		}
	}
	if got := hostile.AggroHate(victim); got != 0 {
		t.Fatalf("hate on the dead victim after the refresh = %v, want 0", got)
	}
	if hostile.AI().Desires().Has(attack) {
		t.Fatal("attack desire on the dead victim kept past the refresh")
	}
}
