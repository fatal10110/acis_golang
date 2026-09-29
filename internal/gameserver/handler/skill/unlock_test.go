package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// racedChest is a real box chest whose Interacted check still reads false
// after another opener's claim landed: the window between the handler's
// Interacted check and its ClaimInteraction. It records every outcome the
// handler could apply instead of running it.
type racedChest struct {
	*npc.Hostile
	killed, deleted, hated, desired bool
}

func (*racedChest) Interacted() bool { return false }
func (c *racedChest) Kill(attackable.Combatant) bool {
	c.killed = true
	return true
}
func (c *racedChest) DeleteMe() { c.deleted = true }
func (c *racedChest) AddDamageHate(attackable.Combatant, float64, float64) {
	c.hated = true
}
func (c *racedChest) AddAttackDesire(attackable.Combatant, float64) { c.desired = true }

func newBoxChest(t *testing.T) *npc.Hostile {
	t.Helper()
	h := newTestHostile(t, 18265, 0)
	h.Instance.Kind = "Chest"
	h.Instance.Template.Type = "Chest"
	if !h.Box() {
		t.Fatal("chest 18265 is not a box")
	}
	return h
}

func castUnlockOn(t *testing.T, chest *racedChest) Result {
	t.Helper()
	result, ok := NewDefaultRegistry().UseResult(Cast{
		Caster:  &skillTarget{isPlayer: true},
		Skill:   modelskill.Definition{SkillType: "UNLOCK", Level: 11},
		Targets: []Actor{chest},
	})
	if !ok {
		t.Fatal("UNLOCK handler missing")
	}
	return result
}

// A second opener that passes the Interacted check but loses the claim
// must neither roll nor apply any outcome: only the claim winner opens or
// destroys the chest. The unclaimed control shows the same cast would
// otherwise open it (a level 11 unlock always opens a level 1 chest).
func TestUnlockLosingChestClaimAppliesNothing(t *testing.T) {
	control := &racedChest{Hostile: newBoxChest(t)}
	castUnlockOn(t, control)
	if !control.killed || !control.hated {
		t.Fatalf("unclaimed chest killed=%v hated=%v, want both", control.killed, control.hated)
	}

	claimed := newBoxChest(t)
	if !claimed.ClaimInteraction() {
		t.Fatal("winner's ClaimInteraction() = false")
	}
	chest := &racedChest{Hostile: claimed}

	result := castUnlockOn(t, chest)

	if chest.killed || chest.deleted || chest.hated || chest.desired {
		t.Fatalf("losing claim applied killed=%v deleted=%v hated=%v desired=%v, want none",
			chest.killed, chest.deleted, chest.hated, chest.desired)
	}
	if len(result.Messages) != 0 {
		t.Fatalf("messages = %v, want none", result.Messages)
	}
}

func TestUnlockNonChestTargetReportsInvalidTarget(t *testing.T) {
	monster := newTestHostile(t, 20001, 0)

	result, _ := NewDefaultRegistry().UseResult(Cast{
		Caster:  &skillTarget{isPlayer: true},
		Skill:   modelskill.Definition{SkillType: "UNLOCK_SPECIAL", Target: modelskill.TargetOne, Power: 100},
		Targets: []Actor{monster},
	})
	if len(result.Messages) != 1 {
		t.Fatalf("messages = %v, want one invalid-target message", result.Messages)
	}
	if _, ok := result.Messages[0].(InvalidTargetMessage); !ok {
		t.Fatalf("message = %T, want InvalidTargetMessage", result.Messages[0])
	}
}
