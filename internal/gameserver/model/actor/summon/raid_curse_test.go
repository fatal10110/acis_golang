package summon

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestSummonRaidCursePetrifiesAndBlocks(t *testing.T) {
	a := mustServitor(t, ServitorConfig{ObjectID: 7, Level: 80, SkillDefs: newRaidCurseSkillTable()})
	target := &raidCurseNPC{id: 2, npcID: 25035, level: 70, attackable: true}

	if !a.TestCursesOnAttack(target) {
		t.Fatal("TestCursesOnAttack() = false, want true")
	}
	if target.hateStops != 1 {
		t.Fatalf("StopAggroHate calls = %d, want 1", target.hateStops)
	}
	if _, ok := a.EffectList().ActiveBySkillID(int(modelskill.RaidCurse2SkillID)); !ok {
		t.Fatal("petrification effect missing")
	}
}

func TestSummonRaidCurseSkipsAntiStrider(t *testing.T) {
	a := mustServitor(t, ServitorConfig{ObjectID: 7, Level: 70, SkillDefs: newRaidCurseSkillTable()})
	target := &raidCurseNPC{id: 2, npcID: 25035, level: 70, attackable: true}

	if a.TestCursesOnAttack(target) {
		t.Fatal("TestCursesOnAttack() = true, want false")
	}
	if _, ok := a.EffectList().ActiveBySkillID(int(modelskill.RaidAntiStriderSlowSkillID)); ok {
		t.Fatal("summon received anti-strider curse")
	}
}

func TestSummonRaidCurseDisabledDoesNotBlock(t *testing.T) {
	a := mustServitor(t, ServitorConfig{ObjectID: 7, Level: 80, SkillDefs: newRaidCurseSkillTable()})
	a.SetRaidCursesDisabled(true)
	target := &raidCurseNPC{id: 2, npcID: 25035, level: 70, attackable: true}

	if a.TestCursesOnAttack(target) {
		t.Fatal("disabled TestCursesOnAttack() = true, want false")
	}
}

func TestBroadcastSkillUseSendsCasterToTarget(t *testing.T) {
	fx := newBroadcastFixture(t)
	fx.actor.BroadcastSkillUse(2, location.Location{X: 1, Y: 2, Z: 3}, 7, location.Location{X: 4, Y: 5, Z: 6}, 4515, 1, 300, 0)
	if fx.frames.skillUses != 1 {
		t.Fatalf("SkillUse translations = %d, want 1", fx.frames.skillUses)
	}
	if len(fx.receiver.frames) == 0 {
		t.Fatal("no observer received MagicSkillUse")
	}
}

type raidCurseNPC struct {
	id         int32
	npcID      int
	level      int
	attackable bool
	hateStops  int
}

func (n *raidCurseNPC) ObjectID() int32           { return n.id }
func (n *raidCurseNPC) SiegeGuard() bool          { return false }
func (n *raidCurseNPC) AlikeDead() bool           { return false }
func (n *raidCurseNPC) Dead() bool                { return false }
func (n *raidCurseNPC) Attackable() bool          { return n.attackable }
func (n *raidCurseNPC) Level() int                { return n.level }
func (n *raidCurseNPC) NpcID() int                { return n.npcID }
func (n *raidCurseNPC) Position() (int, int, int) { return 0, 0, 0 }
func (n *raidCurseNPC) StopAggroHate(attackable.Combatant) {
	n.hateStops++
}

type raidCurseSkillTable map[modelskill.Ref]modelskill.Definition

func (s raidCurseSkillTable) Definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	def, ok := s[ref]
	return def, ok
}

func newRaidCurseSkillTable() raidCurseSkillTable {
	return raidCurseSkillTable{
		{ID: modelskill.RaidCurse2SkillID, Level: 1}: {
			ID: modelskill.RaidCurse2SkillID, Level: 1,
			Activation: modelskill.ActivationActive, Debuff: true, Offensive: true,
			SkillType: "PARALYZE", EffectRange: 2000,
			Effects: []modelskill.EffectTemplate{{
				Name: "Petrification", Count: 1, Time: 120, Icon: true,
				StackType: "turn_stone", StackOrder: 99, EffectPower: -1,
			}},
		},
		{ID: modelskill.RaidAntiStriderSlowSkillID, Level: 1}: {
			ID: modelskill.RaidAntiStriderSlowSkillID, Level: 1,
			Activation: modelskill.ActivationActive, Debuff: true, Offensive: true,
			SkillType: "DEBUFF", EffectRange: 2000,
			Effects: []modelskill.EffectTemplate{{
				Name: "Debuff", Count: 1, Time: 1200, Icon: true,
				StackType: "speed_down", StackOrder: 99, EffectPower: -1,
			}},
		},
	}
}
