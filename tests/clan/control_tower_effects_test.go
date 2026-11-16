package clan

import (
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: LifeTower and FlameTower are plain NPCs, not Folk, so
// Folk.addEffect's buff-and-debuff-only filter does not apply to them:
// every effect lands. A damage-over-time tick goes through the tower's
// reduceCurrentHp, which unpolymorphs a working tower left at 1 HP or less
// during its siege; a hit that is no such tick ends a sleep
// (CreatureStatus.reduceHp).

const (
	towerDOTSkillID   = 9301
	towerHOTSkillID   = 9302
	towerSleepSkillID = 9303
)

// towerEffectSkills are a poison, a heal over time and a sleep, each cast
// on one target with no cost.
func towerEffectSkills() []modelskill.Definition {
	def := func(id int, offensive bool, skillType string, effect modelskill.EffectTemplate) modelskill.Definition {
		return modelskill.Definition{
			ID: modelskill.ID(id), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			Offensive: offensive, SkillType: skillType, Power: 100, CastRange: 900, HitTime: 500,
			StaticHitTime: true, StaticReuse: true, Effects: []modelskill.EffectTemplate{effect},
		}
	}
	return []modelskill.Definition{
		def(towerDOTSkillID, true, "POISON", modelskill.EffectTemplate{
			Name: "DamOverTime", Count: 3, Time: 1, Value: 1_000_000, StackType: "poison", StackOrder: 1, EffectType: "POISON", Icon: true,
		}),
		def(towerHOTSkillID, false, "HOT", modelskill.EffectTemplate{
			Name: "HealOverTime", Count: 2, Time: 1, Value: 200, StackType: "hot", StackOrder: 1, Icon: true,
		}),
		def(towerSleepSkillID, true, "SLEEP", modelskill.EffectTemplate{
			Name: "Sleep", Count: 1, Time: 60, StackType: "sleep", StackOrder: 1, Icon: true,
		}),
	}
}

// TestControlTowersHoldEveryEffect has the attacker cast, during the
// siege, a poison on the flame tower, a heal over time on a wounded life
// tower and a sleep on the other: each lands. The poison's ticks leave the
// flame tower at 1 HP, which drops it dead; the heal ticks fill the life
// tower; a poison tick leaves the sleep on, a hit ends it.
func TestControlTowersHoldEveryEffect(t *testing.T) {
	t.Parallel()
	db := sqltest.SharedDB(t)
	w := towerWorld(t, gameservertest.WithSkills(skillstate.NewPersistence(
		gamesql.NewSkillSaveStore(db), modelskill.NewTable(towerEffectSkills()), gamesql.NewCharacterSkillStore(db))))
	if !w.srv.DrivesClock() {
		t.Skip("the effects' ticks need the driven clock")
	}
	gludio, life1, life2, flame := towers(t, w)
	for _, id := range []int{towerDOTSkillID, towerHOTSkillID, towerSleepSkillID} {
		if err := w.srv.KnownSkills.SetKnownSkill(t.Context(), w.leaderID, 0, id, 1); err != nil {
			t.Fatalf("seed known skill %d: %v", id, err)
		}
	}
	startInWorld(t, w.leader)
	player, ok := w.srv.State.Player(w.leaderID)
	if !ok {
		t.Fatal("founder not in the world")
	}
	// Every landing roll succeeds.
	player.(interface{ SetRollSource(func(int) int) }).SetRollSource(func(int) int { return 0 })
	gludio.Start()
	drainFrames(t, w.leader)

	cast := func(target *npc.Folk, skillID int32) {
		t.Helper()
		w.leader.Send(encodeAction(target.ObjectID(), w.at))
		drainFrames(t, w.leader)
		w.leader.Send(encodeRequestMagicSkillUse(skillID))
		if _, ok := firstOpcode(drainFrames(t, w.leader), serverpackets.OpcodeMagicSkillUse); !ok {
			t.Fatalf("skill %d on tower %d did not start", skillID, target.ObjectID())
		}
		w.srv.AdvanceWithEffects(t, time.Second)
		drainFrames(t, w.leader)
		if _, held := target.EffectList().ActiveBySkillID(int(skillID)); !held {
			t.Fatalf("skill %d landed nothing on tower %d", skillID, target.ObjectID())
		}
	}

	cast(flame, towerDOTSkillID)
	for tick := 0; flame.TowerWorking(); tick++ {
		if tick == 3 {
			t.Fatalf("flame tower still works after the poison's ticks, HP %v", flame.HP())
		}
		w.srv.AdvanceWithEffects(t, time.Second)
	}
	drainFrames(t, w.leader)
	if hp := flame.HP(); hp != 1 {
		t.Fatalf("flame tower HP once it fell = %v, want 1", hp)
	}

	wounded := 100.0
	life1.SetHP(wounded)
	cast(life1, towerHOTSkillID)
	w.srv.AdvanceWithEffects(t, 3*time.Second)
	drainFrames(t, w.leader)
	if hp := life1.HP(); hp < wounded+2*200 {
		t.Fatalf("life tower HP after the heal over time = %v, want at least %v", hp, wounded+2*200)
	}

	cast(life2, towerSleepSkillID)
	life2.ReduceHPByDOT(1, nil, true)
	if _, held := life2.EffectList().ActiveBySkillID(towerSleepSkillID); !held {
		t.Fatal("a poison tick ended the life tower's sleep")
	}
	life2.TakeDamage(1, nil)
	if _, held := life2.EffectList().ActiveBySkillID(towerSleepSkillID); held {
		t.Fatal("a hit left the life tower asleep")
	}
}
