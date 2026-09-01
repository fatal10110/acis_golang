package creature

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

func TestCursesOnAttackPetrifiesLevelGapAndStopsHate(t *testing.T) {
	attacker := newCursePlayable(t, 80)
	target := &curseNPC{id: 2, npcID: 25035, level: 70, attackable: true}
	var uses []MagicSkillUse
	blocked := TestCursesOnAttack(RaidCurseInput{
		Attacker:  attacker,
		Target:    target,
		NPCID:     target.npcID,
		Skills:    raidCurseSkills(),
		Broadcast: func(use MagicSkillUse) { uses = append(uses, use) },
	})
	if !blocked {
		t.Fatal("TestCursesOnAttack() = false, want true")
	}
	if target.hateStops != 1 || target.stopped != attacker {
		t.Fatalf("StopAggroHate calls = %d target=%v, want 1 playable", target.hateStops, target.stopped)
	}
	if !hasActiveSkill(attacker.EffectList(), modelskill.RaidCurse2SkillID) {
		t.Fatal("petrification effect missing")
	}
	if len(uses) != 1 {
		t.Fatalf("MagicSkillUse broadcasts = %d, want 1", len(uses))
	}
	got := uses[0]
	if got.CasterID != 2 || got.TargetID != attacker.ObjectID() || got.SkillID != int32(modelskill.RaidCurse2SkillID) || got.Level != 1 || got.HitTime != RaidCurseHitTime || got.ReuseDelay != 0 {
		t.Fatalf("MagicSkillUse = %+v, want caster 2 target playable skill 4515/1 hitTime 300", got)
	}
}

func TestCursesOnAttackExistingPetrifyDoesNotBlock(t *testing.T) {
	attacker := newCursePlayable(t, 80)
	target := &curseNPC{id: 2, npcID: 25035, level: 70, attackable: true}
	skills := raidCurseSkills()
	def, _ := skills.Definition(modelskill.Ref{ID: modelskill.RaidCurse2SkillID, Level: 1})
	effect.Apply(attacker.EffectList(), target, attacker, effect.SkillFromDefinition(def), def.Effects)

	var uses []MagicSkillUse
	blocked := TestCursesOnAttack(RaidCurseInput{
		Attacker:  attacker,
		Target:    target,
		NPCID:     target.npcID,
		Skills:    skills,
		Broadcast: func(use MagicSkillUse) { uses = append(uses, use) },
	})
	if blocked {
		t.Fatal("TestCursesOnAttack() with existing petrify = true, want false")
	}
	if target.hateStops != 0 {
		t.Fatalf("StopAggroHate calls = %d, want 0", target.hateStops)
	}
	if len(uses) != 0 {
		t.Fatalf("MagicSkillUse broadcasts = %d, want 0", len(uses))
	}
}

func TestCursesOnAttackMountedAntiStriderDoesNotBlock(t *testing.T) {
	attacker := newCursePlayable(t, 70)
	target := &curseNPC{id: 2, npcID: 25035, level: 70, attackable: true}
	var uses []MagicSkillUse
	blocked := TestCursesOnAttack(RaidCurseInput{
		Attacker:  attacker,
		Target:    target,
		NPCID:     target.npcID,
		Mounted:   true,
		Skills:    raidCurseSkills(),
		Broadcast: func(use MagicSkillUse) { uses = append(uses, use) },
	})
	if blocked {
		t.Fatal("TestCursesOnAttack() mounted anti-strider = true, want false")
	}
	if target.hateStops != 0 {
		t.Fatalf("StopAggroHate calls = %d, want 0", target.hateStops)
	}
	if !hasActiveSkill(attacker.EffectList(), modelskill.RaidAntiStriderSlowSkillID) {
		t.Fatal("anti-strider effect missing")
	}
	if len(uses) != 1 || uses[0].SkillID != int32(modelskill.RaidAntiStriderSlowSkillID) {
		t.Fatalf("MagicSkillUse = %+v, want anti-strider 4258", uses)
	}
}

func TestCursesOnAttackDisabledAndNonAttackableDoNotBlock(t *testing.T) {
	attacker := newCursePlayable(t, 80)
	target := &curseNPC{id: 2, npcID: 25035, level: 70, attackable: true}
	if TestCursesOnAttack(RaidCurseInput{Attacker: attacker, Target: target, NPCID: target.npcID, Disabled: true, Skills: raidCurseSkills()}) {
		t.Fatal("disabled curses blocked")
	}
	target.attackable = false
	if TestCursesOnAttack(RaidCurseInput{Attacker: attacker, Target: target, NPCID: target.npcID, Skills: raidCurseSkills()}) {
		t.Fatal("non-attackable target blocked")
	}
}

func TestCursesOnAttackLevelGapEightDoesNotPetrify(t *testing.T) {
	attacker := newCursePlayable(t, 78)
	target := &curseNPC{id: 2, npcID: 25035, level: 70, attackable: true}
	if TestCursesOnAttack(RaidCurseInput{Attacker: attacker, Target: target, NPCID: target.npcID, Skills: raidCurseSkills()}) {
		t.Fatal("level gap of 8 blocked")
	}
	if hasActiveSkill(attacker.EffectList(), modelskill.RaidCurse2SkillID) {
		t.Fatal("petrification applied at gap 8")
	}
}

func TestCursesOnAttackExistingPetrifyStillAppliesAntiStrider(t *testing.T) {
	attacker := newCursePlayable(t, 80)
	target := &curseNPC{id: 2, npcID: 25035, level: 70, attackable: true}
	skills := raidCurseSkills()
	def, _ := skills.Definition(modelskill.Ref{ID: modelskill.RaidCurse2SkillID, Level: 1})
	effect.Apply(attacker.EffectList(), target, attacker, effect.SkillFromDefinition(def), def.Effects)

	var uses []MagicSkillUse
	blocked := TestCursesOnAttack(RaidCurseInput{
		Attacker:  attacker,
		Target:    target,
		NPCID:     target.npcID,
		Mounted:   true,
		Skills:    skills,
		Broadcast: func(use MagicSkillUse) { uses = append(uses, use) },
	})
	if blocked {
		t.Fatal("existing petrify + anti-strider blocked")
	}
	if !hasActiveSkill(attacker.EffectList(), modelskill.RaidAntiStriderSlowSkillID) {
		t.Fatal("anti-strider missing after existing petrify")
	}
	if len(uses) != 1 || uses[0].SkillID != int32(modelskill.RaidAntiStriderSlowSkillID) {
		t.Fatalf("broadcasts = %+v, want anti-strider only", uses)
	}
}

func TestCursesOnSkillSeeOffensivePetrifiesAndAborts(t *testing.T) {
	caster := newCursePlayable(t, 80)
	raid := &curseNPC{id: 2, npcID: 25035, level: 70, attackable: true, raidRelated: true}
	var uses []MagicSkillUse
	blocked := TestCursesOnSkillSee(RaidCurseSkillInput{
		Caster:    caster,
		Offensive: true,
		Targets:   []RaidCurseSkillSeeTarget{SkillSeeTargetOf(raid, false)},
		Skills:    raidCurseSkills(),
		Broadcast: func(use MagicSkillUse) { uses = append(uses, use) },
	})
	if !blocked {
		t.Fatal("TestCursesOnSkillSee() offensive = false, want true")
	}
	if raid.hateStops != 1 || raid.stopped != caster {
		t.Fatalf("StopAggroHate calls = %d target=%v, want 1 caster", raid.hateStops, raid.stopped)
	}
	if !hasActiveSkill(caster.EffectList(), modelskill.RaidCurse2SkillID) {
		t.Fatal("petrification effect missing")
	}
	if len(uses) != 1 || uses[0].SkillID != int32(modelskill.RaidCurse2SkillID) || uses[0].HitTime != RaidCurseHitTime || uses[0].ReuseDelay != 0 {
		t.Fatalf("MagicSkillUse = %+v, want petrify 4515/1 hitTime 300", uses)
	}
}

func TestCursesOnSkillSeeBeneficialSilencesHatedPlayable(t *testing.T) {
	caster := newCursePlayable(t, 80)
	helped := newCursePlayable(t, 40)
	helped.id = 3
	raid := &curseNPC{id: 2, npcID: 25035, level: 70, attackable: true, raidRelated: true, hate: map[int32]float64{3: 10}}
	var uses []MagicSkillUse
	blocked := TestCursesOnSkillSee(RaidCurseSkillInput{
		Caster: caster,
		Targets: []RaidCurseSkillSeeTarget{
			SkillSeeTargetOf(helped, true),
		},
		Nearby:    []RaidCurseSkillRaid{raid},
		Skills:    raidCurseSkills(),
		Broadcast: func(use MagicSkillUse) { uses = append(uses, use) },
	})
	if !blocked {
		t.Fatal("TestCursesOnSkillSee() beneficial = false, want true")
	}
	if raid.hateStops != 1 || raid.stopped != caster {
		t.Fatalf("StopAggroHate calls = %d target=%v, want 1 caster", raid.hateStops, raid.stopped)
	}
	if !hasActiveSkill(caster.EffectList(), modelskill.RaidCurseSkillID) {
		t.Fatal("silence effect missing")
	}
	if len(uses) != 1 || uses[0].SkillID != int32(modelskill.RaidCurseSkillID) || uses[0].CasterID != 2 || uses[0].TargetID != caster.ObjectID() {
		t.Fatalf("MagicSkillUse = %+v, want silence 4215 from raid onto caster", uses)
	}
}

func TestCursesOnSkillSeeExistingEffectsAndDisabledDoNotAbort(t *testing.T) {
	skills := raidCurseSkills()
	caster := newCursePlayable(t, 80)
	raid := &curseNPC{id: 2, npcID: 25035, level: 70, attackable: true, raidRelated: true}
	def, _ := skills.Definition(modelskill.Ref{ID: modelskill.RaidCurse2SkillID, Level: 1})
	effect.Apply(caster.EffectList(), raid, caster, effect.SkillFromDefinition(def), def.Effects)

	if TestCursesOnSkillSee(RaidCurseSkillInput{
		Caster:    caster,
		Offensive: true,
		Targets:   []RaidCurseSkillSeeTarget{SkillSeeTargetOf(raid, false)},
		Skills:    skills,
	}) {
		t.Fatal("existing petrify aborted leftover skill")
	}

	helped := newCursePlayable(t, 40)
	helped.id = 3
	silenced := newCursePlayable(t, 80)
	silenced.id = 4
	silence, _ := skills.Definition(modelskill.Ref{ID: modelskill.RaidCurseSkillID, Level: 1})
	effect.Apply(silenced.EffectList(), raid, silenced, effect.SkillFromDefinition(silence), silence.Effects)
	raid.hate = map[int32]float64{3: 10}
	if TestCursesOnSkillSee(RaidCurseSkillInput{
		Caster:  silenced,
		Targets: []RaidCurseSkillSeeTarget{SkillSeeTargetOf(helped, true)},
		Nearby:  []RaidCurseSkillRaid{raid},
		Skills:  skills,
	}) {
		t.Fatal("existing silence aborted leftover skill")
	}

	if TestCursesOnSkillSee(RaidCurseSkillInput{
		Caster:    caster,
		Offensive: true,
		Disabled:  true,
		Targets:   []RaidCurseSkillSeeTarget{SkillSeeTargetOf(raid, false)},
		Skills:    skills,
	}) {
		t.Fatal("disabled curses aborted leftover skill")
	}
}

func TestCursesOnSkillSeeOffensiveOnPlayableDoesNotSilence(t *testing.T) {
	caster := newCursePlayable(t, 80)
	helped := newCursePlayable(t, 40)
	helped.id = 3
	raid := &curseNPC{id: 2, npcID: 25035, level: 70, attackable: true, raidRelated: true, hate: map[int32]float64{3: 10}}
	if TestCursesOnSkillSee(RaidCurseSkillInput{
		Caster:    caster,
		Offensive: true,
		Targets:   []RaidCurseSkillSeeTarget{SkillSeeTargetOf(helped, true)},
		Nearby:    []RaidCurseSkillRaid{raid},
		Skills:    raidCurseSkills(),
	}) {
		t.Fatal("offensive skill on playable silenced")
	}
	if hasActiveSkill(caster.EffectList(), modelskill.RaidCurseSkillID) {
		t.Fatal("silence applied on offensive playable target")
	}
}

func TestCursesOnSkillSeeZeroHateAndGapEightDoNotAbort(t *testing.T) {
	caster := newCursePlayable(t, 78)
	helped := newCursePlayable(t, 40)
	helped.id = 3
	raid := &curseNPC{id: 2, npcID: 25035, level: 70, attackable: true, raidRelated: true, hate: map[int32]float64{3: 10}}
	if TestCursesOnSkillSee(RaidCurseSkillInput{
		Caster:  caster,
		Targets: []RaidCurseSkillSeeTarget{SkillSeeTargetOf(helped, true)},
		Nearby:  []RaidCurseSkillRaid{raid},
		Skills:  raidCurseSkills(),
	}) {
		t.Fatal("level gap of 8 aborted beneficial skill")
	}

	caster.level = 80
	raid.hate[3] = 0
	if TestCursesOnSkillSee(RaidCurseSkillInput{
		Caster:  caster,
		Targets: []RaidCurseSkillSeeTarget{SkillSeeTargetOf(helped, true)},
		Nearby:  []RaidCurseSkillRaid{raid},
		Skills:  raidCurseSkills(),
	}) {
		t.Fatal("hate 0 aborted beneficial skill")
	}
}

type cursePlayable struct {
	id    int32
	level int
	list  *effect.List
}

func newCursePlayable(t *testing.T, level int) *cursePlayable {
	t.Helper()
	p := &cursePlayable{id: 1, level: level}
	p.list = effect.NewList(p)
	return p
}

func (p *cursePlayable) ObjectID() int32                    { return p.id }
func (p *cursePlayable) SiegeGuard() bool                   { return false }
func (p *cursePlayable) AlikeDead() bool                    { return false }
func (p *cursePlayable) Dead() bool                         { return false }
func (p *cursePlayable) Invul() bool                        { return false }
func (p *cursePlayable) Level() int                         { return p.level }
func (p *cursePlayable) Position() (int, int, int)          { return 0, 0, 0 }
func (p *cursePlayable) EffectList() *effect.List           { return p.list }
func (p *cursePlayable) AddStatFuncs([]effect.Mod)          {}
func (p *cursePlayable) RemoveStatsByOwner(effect.ModOwner) {}
func (p *cursePlayable) MaxBuffCount() int                  { return 20 }

type curseNPC struct {
	id          int32
	npcID       int
	level       int
	attackable  bool
	raidRelated bool
	hate        map[int32]float64
	hateStops   int
	stopped     attackable.Combatant
}

func (n *curseNPC) ObjectID() int32           { return n.id }
func (n *curseNPC) SiegeGuard() bool          { return false }
func (n *curseNPC) AlikeDead() bool           { return false }
func (n *curseNPC) Dead() bool                { return false }
func (n *curseNPC) Attackable() bool          { return n.attackable }
func (n *curseNPC) Level() int                { return n.level }
func (n *curseNPC) NpcID() int                { return n.npcID }
func (n *curseNPC) Position() (int, int, int) { return 0, 0, 0 }
func (n *curseNPC) RaidRelated() bool         { return n.raidRelated }
func (n *curseNPC) AggroHate(c attackable.Combatant) float64 {
	if n.hate == nil || c == nil {
		return 0
	}
	return n.hate[c.ObjectID()]
}
func (n *curseNPC) StopAggroHate(attacker attackable.Combatant) {
	n.hateStops++
	n.stopped = attacker
}

type curseSkills map[modelskill.Ref]modelskill.Definition

func (s curseSkills) Definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	def, ok := s[ref]
	return def, ok
}

func raidCurseSkills() curseSkills {
	return curseSkills{
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
