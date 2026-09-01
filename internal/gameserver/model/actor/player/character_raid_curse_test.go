package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestCharacterRaidCursePetrifiesAndBlocks(t *testing.T) {
	c := withEffectList(t, liveCharacter(1, combatTemplate(), combatItems()))
	c.CharLevel = 80
	var uses []creature.MagicSkillUse
	c.SetSkillDefinitions(newRaidCurseSkillTable())
	c.SetMagicSkillUseBroadcaster(func(use creature.MagicSkillUse) { uses = append(uses, use) })
	target := &raidCurseNPC{id: 2, npcID: 25035, level: 70, attackable: true}

	if !c.TestCursesOnAttack(target) {
		t.Fatal("TestCursesOnAttack() = false, want true")
	}
	if target.hateStops != 1 {
		t.Fatalf("StopAggroHate calls = %d, want 1", target.hateStops)
	}
	if _, ok := c.EffectList().ActiveBySkillID(int(modelskill.RaidCurse2SkillID)); !ok {
		t.Fatal("petrification effect missing")
	}
	if len(uses) != 1 || uses[0].SkillID != int32(modelskill.RaidCurse2SkillID) || uses[0].HitTime != creature.RaidCurseHitTime {
		t.Fatalf("MagicSkillUse = %+v, want petrify hitTime 300", uses)
	}
}

func TestCharacterRaidCurseMountedAntiStriderContinues(t *testing.T) {
	c := withEffectList(t, liveCharacter(1, combatTemplate(), combatItems()))
	c.CharLevel = 70
	c.Mount(12526, 99)
	c.SetSkillDefinitions(newRaidCurseSkillTable())
	target := &raidCurseNPC{id: 2, npcID: 25035, level: 70, attackable: true}

	if c.TestCursesOnAttack(target) {
		t.Fatal("TestCursesOnAttack() mounted = true, want false")
	}
	if target.hateStops != 0 {
		t.Fatalf("StopAggroHate calls = %d, want 0", target.hateStops)
	}
	if _, ok := c.EffectList().ActiveBySkillID(int(modelskill.RaidAntiStriderSlowSkillID)); !ok {
		t.Fatal("anti-strider effect missing")
	}
}

func TestCharacterRaidCurseDisabledDoesNotBlock(t *testing.T) {
	c := withEffectList(t, liveCharacter(1, combatTemplate(), combatItems()))
	c.CharLevel = 80
	c.SetRaidCursesDisabled(true)
	c.SetSkillDefinitions(newRaidCurseSkillTable())
	target := &raidCurseNPC{id: 2, npcID: 25035, level: 70, attackable: true}

	if c.TestCursesOnAttack(target) {
		t.Fatal("disabled TestCursesOnAttack() = true, want false")
	}
}

func TestCharacterRaidCurseSkillSeePetrifiesAndAborts(t *testing.T) {
	c := withEffectList(t, liveCharacter(1, combatTemplate(), combatItems()))
	c.CharLevel = 80
	var uses []creature.MagicSkillUse
	c.SetSkillDefinitions(newRaidCurseSkillTable())
	c.SetMagicSkillUseBroadcaster(func(use creature.MagicSkillUse) { uses = append(uses, use) })
	raid := &raidCurseNPC{id: 2, npcID: 25035, level: 70, attackable: true, raidRelated: true}

	if !c.TestCursesOnSkillSee(modelskill.Definition{Offensive: true}, []target.Creature{raid}) {
		t.Fatal("TestCursesOnSkillSee() = false, want true")
	}
	if raid.hateStops != 1 {
		t.Fatalf("StopAggroHate calls = %d, want 1", raid.hateStops)
	}
	if _, ok := c.EffectList().ActiveBySkillID(int(modelskill.RaidCurse2SkillID)); !ok {
		t.Fatal("petrification effect missing")
	}
	if len(uses) != 1 || uses[0].SkillID != int32(modelskill.RaidCurse2SkillID) {
		t.Fatalf("MagicSkillUse = %+v, want petrify 4515", uses)
	}
}

func TestCharacterRaidCurseSkillSeeDisabledDoesNotAbort(t *testing.T) {
	c := withEffectList(t, liveCharacter(1, combatTemplate(), combatItems()))
	c.CharLevel = 80
	c.SetRaidCursesDisabled(true)
	c.SetSkillDefinitions(newRaidCurseSkillTable())
	raid := &raidCurseNPC{id: 2, npcID: 25035, level: 70, attackable: true, raidRelated: true}

	if c.TestCursesOnSkillSee(modelskill.Definition{Offensive: true}, []target.Creature{raid}) {
		t.Fatal("disabled TestCursesOnSkillSee() = true, want false")
	}
}

type raidCurseNPC struct {
	id          int32
	npcID       int
	level       int
	attackable  bool
	raidRelated bool
	hateStops   int
}

func (n *raidCurseNPC) ObjectID() int32           { return n.id }
func (n *raidCurseNPC) SiegeGuard() bool          { return false }
func (n *raidCurseNPC) AlikeDead() bool           { return false }
func (n *raidCurseNPC) Dead() bool                { return false }
func (n *raidCurseNPC) Attackable() bool          { return n.attackable }
func (n *raidCurseNPC) Level() int                { return n.level }
func (n *raidCurseNPC) NpcID() int                { return n.npcID }
func (n *raidCurseNPC) Position() (int, int, int) { return 0, 0, 0 }
func (n *raidCurseNPC) Heading() int              { return 0 }
func (n *raidCurseNPC) Category() target.Category { return target.CategoryAttackable }
func (n *raidCurseNPC) RaidRelated() bool         { return n.raidRelated }
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
		{ID: modelskill.RaidCurseSkillID, Level: 1}: {
			ID: modelskill.RaidCurseSkillID, Level: 1,
			Activation: modelskill.ActivationActive, Debuff: true,
			SkillType: "MUTE", EffectRange: 2000,
			Effects: []modelskill.EffectTemplate{{
				Name: "SilenceMagicPhysical", Count: 1, Time: 3600, Icon: true,
				StackType: "silence_all", StackOrder: 99, EffectPower: -1,
			}},
		},
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
