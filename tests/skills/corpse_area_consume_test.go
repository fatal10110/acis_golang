package skills

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestAreaCorpseSkillConsumesTheCorpse casts an AREA_CORPSE_MOB drain on a
// fresh corpse: like the single-target corpse skills, it force-decays the
// corpse it lands on.
func TestAreaCorpseSkillConsumesTheCorpse(t *testing.T) {
	t.Parallel()
	def := corpseDrainSkill()
	def.Target = modelskill.TargetAreaCorpseMob
	def.Radius = 200
	srv, _, mob, decay, removals := corpseConsumeScene(t, def, nil)

	srv.Client.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
	srv.AdvanceUntil(t, "the area corpse skill consumes the corpse", mob.Decayed)
	srv.Settle(t)
	if decay.Tracked(mob) {
		t.Fatal("the consumed corpse still awaits its decay")
	}
	if got := removals.armed(); len(got) != 1 || got[0] != mob.ObjectID() {
		t.Fatalf("respawns armed = %v, want [%d]", got, mob.ObjectID())
	}
}
