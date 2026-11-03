package ai

import (
	"testing"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// TestMoreSiegeGuardsSpareTheirCastleOwner ports the castle-state tests of
// GludioStand, GludioBowGuardStand and GludioWizard on Aden's guards and the
// character whose clan owns Aden Castle: the standing guard and the wizard
// that see it want nothing of it, and a hit on the standing guard or the
// standing bow guard leaves both alone (attacked and the hit's clan calls
// alike). Seal of Ruler, whose spelled hooks there have no such test, makes
// the standing bow guard, a spear guard and a sword guard attack it with
// weight 50000.
func TestMoreSiegeGuardsSpareTheirCastleOwner(t *testing.T) {
	t.Parallel()
	w := bootSiegeGuards(t, adenOwnerBoot(t),
		siegeGuardTemplate(adenStandID), siegeCasterTemplate(adenWizardID, npc.SkillTypeDDMagic, siegeDDMagicID),
		siegeGuardTemplate(adenBowGuardStandID), siegeGuardTemplate(adenSpearGuardID), siegeGuardTemplate(adenSwordGuardID))
	stand := w.spawn(t, adenStandID, 150, 0)
	wizard := w.spawn(t, adenWizardID, -150, 0)
	w.srv.InstallNPCCastSeam(t, wizard, siegeNPCSkills())
	w.ticks(t, 2)
	if d, ok := desire(stand, actorai.IntentionAttack); ok {
		t.Fatalf("standing guard wants %+v on its castle's owner", d)
	}
	if d, ok := desire(wizard, actorai.IntentionCast); ok {
		t.Fatalf("wizard wants %+v on its castle's owner", d)
	}

	bow := w.spawn(t, adenBowGuardStandID, 150, 100)
	for name, h := range map[string]*npc.Hostile{"standing guard": stand, "standing bow guard": bow} {
		w.hit(t, h, siegeTwentieth)
		if got := h.AggroHate(w.player); got != 0 {
			t.Fatalf("%s hate of its castle's owner after a hit = %v, want none", name, got)
		}
	}

	for _, c := range []struct {
		name string
		h    *npc.Hostile
	}{
		{"standing bow guard", bow},
		{"spear guard", w.spawn(t, adenSpearGuardID, 100, -500)},
		{"sword guard", w.spawn(t, adenSwordGuardID, 100, 500)},
	} {
		w.selectTarget(t, c.h)
		w.cast(t, sealOfRulerID)
		if got := c.h.AggroHate(w.player); got != 50000 {
			t.Fatalf("%s hate after its castle owner's Seal of Ruler = %v, want 50000", c.name, got)
		}
	}
}
