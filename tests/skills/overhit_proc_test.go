package skills

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// Reference: CreatureCast.callSkill (CreatureCast.java:454-476). For each
// target, a playable's overhit skill arms the monster's overhit state
// before the caster's weapon on-magic skill and both sides' chance procs
// run on it, so an on-magic proc that kills the monster already lands as an
// overhit and the kill pays the overhit bonus.

const (
	overhitCastSkill = 9611
	overhitProcSkill = 9612
)

func overhitProcSkills() []modelskill.Definition {
	return append(chanceSkills(),
		modelskill.Definition{
			ID: overhitCastSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "MDAM", Offensive: true, Power: 1, CastRange: 900, HitTime: 500, StaticHitTime: true,
			Overhit: true,
		},
		modelskill.Definition{
			ID: overhitProcSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "MDAM", Offensive: true, Power: 1_000_000,
		},
	)
}

// TestWeaponOnMagicProcKillIsAnOverhit: the player's overhit MDAM sets off
// its Sword's on-magic MDAM, which kills the monster before the cast's own
// handler runs. The kill still counts as the player's overhit.
func TestWeaponOnMagicProcKillIsAnOverhit(t *testing.T) {
	t.Parallel()
	catalog := procWeaponCatalog(t, func(w *item.WeaponDetail) {
		w.OnCastSkill = &item.SkillTrigger{Skill: item.SkillRef{ID: overhitProcSkill, Level: 1}, Chance: -1}
	})
	srv, c, objID := bootArmedWith(t, overhitProcSkills(), catalog, mdamLandsRoll, []int32{fixtureSwordID}, overhitCastSkill)
	hostile := spawnReflector(t, srv, objID, 0)
	// A real M.Def keeps the cast's own power-1 MDAM from killing it.
	hostile.AddStatFuncs([]effect.Mod{{Stat: stat.MagicDefence, Op: effect.OpSet, Value: 100}})
	px, py, pz := srv.PlayerPosition(t, objID)
	c.Send(encodeAction(hostile.ObjectID(), int32(px), int32(py), int32(pz), false))
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(overhitCastSkill, false, false))
	srv.AdvanceUntil(t, "the on-magic proc kills the monster", hostile.Dead)
	srv.Settle(t)
	frames := queueFrames(t, c)

	ids := systemMessageIDs(frames)
	if !slices.Contains(ids, int32(serverpackets.SystemMessageS1HasBeenActivated)) {
		t.Fatalf("no S1_HAS_BEEN_ACTIVATED among %v: the proc did not fire", ids)
	}
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world")
	}
	player, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("world player %T is not an online character", obj)
	}
	if !hostile.OverhitValid(player) {
		t.Fatal("the proc's kill is not the player's overhit")
	}
	if !slices.Contains(ids, int32(serverpackets.SystemMessageOverHit)) {
		t.Fatalf("no OVER_HIT among %v", ids)
	}
}
