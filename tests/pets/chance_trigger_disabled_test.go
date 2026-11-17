package pets

import (
	"slices"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPetChanceTriggerWhileAllSkillsDisabled hits a pet holding an ON_ATTACKED
// chance trigger while it is paralyzed, so every skill is disabled. (A stun
// would do, but a hit can break it before the trigger runs.) The disabled
// check only answers true for a pet tracking at least one reuse delay: a
// paralyzed pet with none still procs, a paralyzed pet holding any reuse
// entry (the key is another skill's) does not, and a pet with only the
// reuse entry procs.
func TestPetChanceTriggerWhileAllSkillsDisabled(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		paralyzed  bool
		reuseEntry bool
		wantProc   bool
	}{
		{name: "paralyzed with no reuse entry", paralyzed: true, wantProc: true},
		{name: "paralyzed holding a reuse entry", paralyzed: true, reuseEntry: true},
		{name: "reuse entry but not paralyzed", reuseEntry: true, wantProc: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := sqltest.SharedDB(t)
			skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
				{
					ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
					SkillType: "SUMMON_CREATURE", StaticHitTime: true, StaticReuse: true,
				},
				{
					ID: petMirageTriggered, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
					SkillType: "DEBUFF", EffectType: "DEBUFF", Debuff: true, Offensive: true, EffectPower: 100, IgnoreResists: true,
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
			if tc.paralyzed {
				addPetEffect(t, pet, "Paralyze")
			}
			if tc.reuseEntry {
				const unrelatedKey = 9999*256 + 1
				runOn(t, pet.Queue(), func() { pet.DisableSkill(unrelatedKey, time.Hour) })
			}
			drainFrames(t, h.client)

			attacker, ok := other.actor.(interface{ EffectList() *effect.List })
			if !ok {
				t.Fatalf("attacker %T has no effect list", other.actor)
			}
			procced := func() bool {
				return slices.ContainsFunc(attacker.EffectList().All(), func(e *effect.Effect) bool { return e.Skill.ID == petMirageTriggered })
			}
			hpBefore := pet.HP()
			x, y, z := pet.Position()
			other.client.Send(encodeAction(pet.ObjectID(), int32(x), int32(y), int32(z), false))
			drainFrames(t, other.client)
			other.client.Send(encodeAttackRequest(pet.ObjectID(), int32(x), int32(y), int32(z), false))
			if tc.wantProc {
				h.srv.AdvanceUntil(t, "pet proc", procced)
				return
			}
			// The hit is what the trigger answers: wait for it to land, then
			// give a proc the time it would need.
			h.srv.AdvanceUntil(t, "hit landing on the pet", func() bool { return pet.HP() < hpBefore })
			h.srv.Advance(t, 5*time.Second)
			h.srv.Settle(t)
			if procced() {
				t.Fatal("a paralyzed pet holding a reuse entry cast its chance trigger")
			}
		})
	}
}
