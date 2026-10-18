package skills

import (
	"fmt"
	"slices"
	"testing"
	"time"

	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// corpseClear has the shape of the corpse-removal skill a corpse necromancer
// casts at a corpse it sees: aimed at itself, no hit, cool or reuse time,
// and a skill type no handler acts on. Its initial MP is more than the
// monster has, so a checked cast of it is refused.
const corpseClear = modelskill.ID(4672)

func corpseClearSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: corpseClear, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetSelf, SkillType: "PASSIVE", CastRange: 900, MagicLevel: 40,
		MPInitialConsume: 100_000,
	}
}

// TestScriptCastUncheckedSkipsTheStartGates (CreatureCast.doCast runs none
// of canCast's gates): a script's direct cast starts even though the skill
// is in reuse and its MP cannot be paid, which refuses the same cast
// started through the NPC's checked path. The player sees it aimed at the
// corpse the script names and launched at the caster, whom its target type
// picks, and the cast finishes with the skill-finished hook reporting
// success on that caster.
func TestScriptCastUncheckedSkipsTheStartGates(t *testing.T) {
	t.Parallel()
	def := corpseClearSkill()
	w := bootDeletedCast(t, def)
	corpse := w.srv.SpawnHostileNPC(t)
	onNPCQueue(t, corpse, func() {
		if !corpse.Die(nil, nil) {
			t.Error("the corpse's death was rejected")
		}
	})
	drainUntilQuiet(t, w.c)
	w.caster.DisableSkill(actorcast.ReuseKey(def), time.Minute)
	ref := modelskill.Ref{ID: corpseClear, Level: 1}

	w.cast(corpseClear)
	w.srv.Advance(t, time.Second)
	if log := readFrameLog(w.c); log.index(objectFrame(serverpackets.OpcodeMagicSkillUse, w.caster.ObjectID())) >= 0 || len(w.finished()) != 0 {
		t.Fatal("the checked cast of a skill in reuse with unpayable MP started")
	}

	onNPCQueue(t, w.caster, func() { script.NPCOf(w.caster).CastUnchecked(script.NPCOf(corpse), ref) })
	w.srv.AdvanceUntil(t, "the unchecked cast finishing", func() bool { return len(w.finished()) > 0 })

	log := readFrameLog(w.c)
	use := log.index(objectFrame(serverpackets.OpcodeMagicSkillUse, w.caster.ObjectID()))
	if use < 0 {
		t.Fatal("the player saw no MagicSkillUse of the unchecked cast")
	}
	r := wireReader(log[use][1:])
	r.ReadInt32()
	if target, skill, level := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); target != corpse.ObjectID() || skill != int32(corpseClear) || level != 1 {
		t.Fatalf("MagicSkillUse target %d skill %d-%d, want the corpse %d and skill %d-1", target, skill, level, corpse.ObjectID(), corpseClear)
	}
	launched := log.indexAfter(use, objectFrame(serverpackets.OpcodeMagicSkillLaunched, w.caster.ObjectID()))
	if launched < 0 {
		t.Fatal("the player saw no MagicSkillLaunched after the unchecked cast's MagicSkillUse")
	}
	r = wireReader(log[launched][1:])
	r.ReadInt32()
	r.ReadInt32()
	r.ReadInt32()
	if n, first := r.ReadInt32(), r.ReadInt32(); n != 1 || first != w.caster.ObjectID() {
		t.Fatalf("MagicSkillLaunched targets %d starting %d, want the caster %d alone", n, first, w.caster.ObjectID())
	}
	want := []string{fmt.Sprintf("skill=%d target=%d success=true", corpseClear, w.caster.ObjectID())}
	if got := w.finished(); !slices.Equal(got, want) {
		t.Fatalf("skill-finished hooks = %q, want %q", got, want)
	}
	if corpse.Decayed() {
		t.Fatal("the cast removed the corpse")
	}
}

// TestScriptCastUncheckedReplacesTheCastInFlight (doCast does not check
// isCastingNow, and the replaced cast's steps act on the new one): a direct
// cast started while the monster casts a slow nuke at the player takes its
// place. The nuke never lands, nothing shows it cancelled, and only the
// direct cast raises a skill-finished hook.
func TestScriptCastUncheckedReplacesTheCastInFlight(t *testing.T) {
	t.Parallel()
	slow := deletedCastNukeSkill()
	slow.HitTime = 5000
	w := bootDeletedCast(t, slow, corpseClearSkill())
	hp := w.srv.PlayerCurrentHP(t, w.objID)

	w.cast(deletedCastNuke)
	w.srv.Advance(t, time.Second)
	drainUntilQuiet(t, w.c)
	onNPCQueue(t, w.caster, func() {
		script.NPCOf(w.caster).CastUnchecked(script.NPCOf(w.caster), modelskill.Ref{ID: corpseClear, Level: 1})
	})
	w.srv.Advance(t, 10*time.Second)

	if got := w.srv.PlayerCurrentHP(t, w.objID); got != hp {
		t.Fatalf("player HP %d after the replaced nuke, want %d", got, hp)
	}
	log := readFrameLog(w.c)
	if log.index(objectFrame(serverpackets.OpcodeMagicSkillCanceled, w.caster.ObjectID())) >= 0 {
		t.Fatal("the replaced nuke was shown cancelled")
	}
	if log.index(objectFrame(serverpackets.OpcodeMagicSkillUse, w.caster.ObjectID())) < 0 {
		t.Fatal("the player saw no MagicSkillUse of the direct cast")
	}
	want := []string{fmt.Sprintf("skill=%d target=%d success=true", corpseClear, w.caster.ObjectID())}
	if got := w.finished(); !slices.Equal(got, want) {
		t.Fatalf("skill-finished hooks = %q, want %q", got, want)
	}
}
