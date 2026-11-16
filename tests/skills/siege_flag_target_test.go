package skills

import (
	"slices"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestOffensiveCastOnHeadquartersNeedsNoCtrl (TargetOne: only a Folk or a
// Guard takes an offensive single-target skill with CTRL alone; a siege
// headquarters is neither) casts the shipped Wind Strike at a rival clan's
// headquarters without CTRL, then with it: both casts start.
func TestOffensiveCastOnHeadquartersNeedsNoCtrl(t *testing.T) {
	t.Parallel()
	w := bootHQ(t, hqSetup{known: []modelskill.Definition{shippedSkill(t, mdamSkill, 1)}})
	flag := w.otherFlag(t, "Rivals", hqEffectCastSpace)
	selectTarget(t, w.c, flag.ObjectID())
	for _, ctrl := range []bool{false, true} {
		w.c.Send(encodeRequestMagicSkillUse(mdamSkill, ctrl, false))
		started := collectUntilQuiet(t, w.c)
		if !slices.Contains(opcodes(started), serverpackets.OpcodeMagicSkillUse) {
			t.Fatalf("ctrl=%v: Wind Strike at the rival headquarters answered %x, want it started", ctrl, opcodes(started))
		}
		// The cast lands and its reuse delay runs out before the next one.
		w.srv.Advance(t, 20*time.Second)
		drainUntilQuiet(t, w.c)
	}
}
