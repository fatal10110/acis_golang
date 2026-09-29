package pets

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// petMirageTriggered mirrors the skill shipped Mirage's trigger casts, with
// a debuff that lands at a fixed rate so the landing is observable.
const petMirageTriggered = 5144

// TestPetChanceTriggerProcsWhenPetIsHit: a pet holding an ON_ATTACKED
// chance trigger answers another player's landed hit by casting the
// triggered debuff on that player. The owner sees the pet's launch and
// animation, and the debuff lands on the attacker.
func TestPetChanceTriggerProcsWhenPetIsHit(t *testing.T) {
	t.Parallel()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, StaticReuse: true,
		},
		{
			ID: petMirageTriggered, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "DEBUFF", EffectType: "DEBUFF", Debuff: true, Offensive: true, BaseLandRate: 100, IgnoreResists: true,
			Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 30, Icon: true, StackType: "mirage_debuff", StackOrder: 1}},
		},
	}), gamesql.NewCharacterSkillStore(db))
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithSkills(skills),
		gameservertest.WithPvPFlags(task.NewPvPFlags(task.DefaultPvPFlagOptions(), nil)),
	})
	pet, _ := h.spawnWolf(t)
	other := h.joinSecondPlayer(t, "Hitter")
	landEveryHit(t, h.srv, other.id)

	trigger, err := effect.New(effect.Skill{ID: 445, Level: 1}, modelskill.EffectTemplate{
		Name: "ChanceSkillTrigger", Time: 60, Icon: true, StackType: "mirage", StackOrder: 1,
		TriggeredID: petMirageTriggered, TriggeredLevel: 1, ChanceType: "ON_ATTACKED", ActivationChance: -1,
	})
	if err != nil {
		t.Fatalf("effect.New(ChanceSkillTrigger): %v", err)
	}
	trigger.Effector, trigger.Effected = pet, pet
	runOn(t, pet.Queue(), func() { pet.EffectList().Add(trigger) })
	drainFrames(t, h.client)

	attacker, ok := other.actor.(interface{ EffectList() *effect.List })
	if !ok {
		t.Fatalf("attacker %T has no effect list", other.actor)
	}
	x, y, z := pet.Position()
	other.client.Send(encodeAction(pet.ObjectID(), int32(x), int32(y), int32(z), false))
	drainFrames(t, other.client)
	other.client.Send(encodeAttackRequest(pet.ObjectID(), int32(x), int32(y), int32(z), false))
	h.srv.AdvanceUntil(t, "pet proc", func() bool {
		return slices.ContainsFunc(attacker.EffectList().All(), func(e *effect.Effect) bool { return e.Skill.ID == petMirageTriggered })
	})

	launched, used := false, false
	for _, frame := range drainFrames(t, h.client) {
		r := wire.NewReader(frame[1:])
		switch frame[0] {
		case serverpackets.OpcodeMagicSkillLaunched:
			caster, id, level, count := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
			if id != petMirageTriggered {
				continue
			}
			if caster != pet.ObjectID() || level != 1 || count != 1 || r.ReadInt32() != other.id {
				t.Fatalf("MagicSkillLaunched = caster %d level %d count %d, want pet %d level 1 onto %d", caster, level, count, pet.ObjectID(), other.id)
			}
			launched = true
		case serverpackets.OpcodeMagicSkillUse:
			caster, target, id := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
			if id != petMirageTriggered {
				continue
			}
			if !launched {
				t.Fatal("MagicSkillUse of the triggered skill came before its MagicSkillLaunched")
			}
			if caster != pet.ObjectID() || target != other.id {
				t.Fatalf("MagicSkillUse = %d->%d, want pet %d -> attacker %d", caster, target, pet.ObjectID(), other.id)
			}
			used = true
		}
	}
	if !launched || !used {
		t.Fatalf("owner saw MagicSkillLaunched %v, MagicSkillUse %v for the triggered skill, want both", launched, used)
	}
}
