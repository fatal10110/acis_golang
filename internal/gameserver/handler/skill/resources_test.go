package skill

import (
	"math"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

func TestHealPercentRestoresHPOrMP(t *testing.T) {
	registry := NewDefaultRegistry()
	target := &skillTarget{hp: 50, maxHP: 100, mp: 10, maxMP: 50}
	dead := &skillTarget{hp: 10, maxHP: 100, dead: true}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "HEAL_PERCENT", Power: 25},
		Targets: []Actor{target, dead, &placedFakeActor{}},
	})
	if target.hp != 75 {
		t.Fatalf("HEAL_PERCENT hp = %v, want 75", target.hp)
	}
	if dead.hp != 10 {
		t.Fatalf("dead target hp = %v, want unchanged 10", dead.hp)
	}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "MANAHEAL_PERCENT", Power: 40},
		Targets: []Actor{target},
	})
	if target.mp != 30 {
		t.Fatalf("MANAHEAL_PERCENT mp = %v, want 30", target.mp)
	}
}

func TestHealRestoresResolvedAmount(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{healAmount: 80, healOK: true}
	target := &skillTarget{hp: 50, maxHP: 200, healEffectiveness: 125}
	dead := &skillTarget{hp: 10, maxHP: 100, dead: true}

	if !registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "HEAL", Power: 30},
		Targets: []Actor{target, dead, &placedFakeActor{}},
	}) {
		t.Fatal("Use() returned false for HEAL")
	}
	if target.hp != 150 {
		t.Fatalf("HEAL hp = %v, want 150", target.hp)
	}
	if dead.hp != 10 {
		t.Fatalf("dead target hp = %v, want unchanged 10", dead.hp)
	}

	caster.healAmount = 500
	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "HEAL_STATIC", Power: 30},
		Targets: []Actor{target},
	})
	if target.hp != 200 {
		t.Fatalf("HEAL_STATIC hp = %v, want clamped to 200", target.hp)
	}
}

// TestHealLandsSkillEffectsBeforeRestoringHP pins the BUFF pass a heal
// runs first: the skill's effects land on each target (and its self effects
// on the caster) before the HP restore reads the target, and the restore
// still reaches every healable target.
func TestHealLandsSkillEffectsBeforeRestoringHP(t *testing.T) {
	for _, skillType := range []string{"HEAL", "HEAL_STATIC"} {
		t.Run(skillType, func(t *testing.T) {
			caster := &skillTarget{healAmount: 30, healOK: true, effects: newTestList(nil)}
			target := &skillTarget{hp: 50, maxHP: 100, healEffectiveness: 100, effects: newTestList(nil)}
			NewDefaultRegistry().Use(Cast{
				Caster: caster,
				Skill: modelskill.Definition{
					ID: 1217, Level: 1, SkillType: skillType, Power: 30,
					Effects:     []modelskill.EffectTemplate{{Name: "HealOverTime", Value: 10, Count: 3, Time: 1, Icon: true}},
					SelfEffects: buffEffect(),
				},
				Targets: []Actor{target},
			})
			if got := len(target.effects.All()); got != 1 {
				t.Fatalf("target effects = %d, want the skill's heal-over-time", got)
			}
			if target.effectsAtHeal != 1 {
				t.Fatalf("effects on target when HP was restored = %d, want 1 (BUFF pass first)", target.effectsAtHeal)
			}
			if target.hp != 80 {
				t.Fatalf("target hp = %v, want 80", target.hp)
			}
			if got := len(caster.effects.All()); got != 1 {
				t.Fatalf("caster self effects = %d, want 1", got)
			}
		})
	}
}

// TestHealSpiritshotBonusUsesHealSpsAndScaling drives a heal under each
// spiritshot through the registry: the healSps correction (scaled by 0.41
// for a plain shot) and the caster's M.Atk multiplier join the amount, and
// the sampled shot is the one spent: once by the BUFF pass and once by the
// heal's own discharge, which a static heal skips.
func TestHealSpiritshotBonusUsesHealSpsAndScaling(t *testing.T) {
	table, err := modelskill.NewHealSpsTable([]modelskill.HealSps{{MagicLevel: 1, Correction: 17, NeededMAtk: 6}})
	if err != nil {
		t.Fatalf("NewHealSpsTable() error: %v", err)
	}
	registry := newDefaultRegistry(nil, true, table)
	for _, tc := range []struct {
		name      string
		scaling   formulas.HealShotScaling
		shot      item.ShotKind
		skillType string
		want      float64
		wantShots []item.ShotKind
	}{
		{"mage blessed", formulas.HealShotScalingMage, item.ShotBlessedSpirit, "HEAL", 20 + (17 + math.Sqrt(4*100)), []item.ShotKind{item.ShotBlessedSpirit, item.ShotBlessedSpirit}},
		{"mage plain", formulas.HealShotScalingMage, item.ShotSpirit, "HEAL", 20 + (17*0.41 + math.Sqrt(2*100)), []item.ShotKind{item.ShotSpirit, item.ShotSpirit}},
		{"fighter blessed", formulas.HealShotScalingNone, item.ShotBlessedSpirit, "HEAL", 20 + (17 + math.Sqrt(100)), []item.ShotKind{item.ShotBlessedSpirit, item.ShotBlessedSpirit}},
		{"npc plain", formulas.HealShotScalingNPC, item.ShotSpirit, "HEAL", 20 + (17*0.41 + math.Sqrt(4*100)), []item.ShotKind{item.ShotSpirit, item.ShotSpirit}},
		{"static spends only in the buff pass", formulas.HealShotScalingMage, item.ShotBlessedSpirit, "HEAL_STATIC", 20, []item.ShotKind{item.ShotBlessedSpirit}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caster := &skillTarget{
				healAmount: 20, healMAtk: 100, healScaling: tc.scaling, healOK: true,
				charged: map[item.ShotKind]bool{tc.shot: true},
			}
			target := &skillTarget{hp: 100, maxHP: 1000, healEffectiveness: 100}
			registry.Use(Cast{
				Caster:  caster,
				Skill:   modelskill.Definition{ID: 1011, Level: 1, MagicLevel: 10, SkillType: tc.skillType, Power: 20},
				Targets: []Actor{target},
			})
			if got := target.hp - 100; math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("healed %v, want %v", got, tc.want)
			}
			if !slices.Equal(caster.shots, tc.wantShots) {
				t.Fatalf("discharged shots = %v, want %v", caster.shots, tc.wantShots)
			}
		})
	}
}

// TestManaHealRefreshesSelfEffects pins the self-effect refresh after the
// MP restore: the caster's prior self effect of the skill is replaced, not
// stacked.
func TestManaHealRefreshesSelfEffects(t *testing.T) {
	caster := &skillTarget{mp: 10, maxMP: 100, effects: newTestList(nil)}
	def := modelskill.Definition{ID: 1013, Level: 1, SkillType: "MANAHEAL", Power: 20, SelfEffects: buffEffect()}
	registry := NewDefaultRegistry()
	for range 2 {
		registry.Use(Cast{Caster: caster, Skill: def, Targets: []Actor{caster}})
	}
	if got := len(caster.effects.All()); got != 1 {
		t.Fatalf("caster self effects after two casts = %d, want 1", got)
	}
	if caster.mp != 50 {
		t.Fatalf("caster mp = %v, want 50", caster.mp)
	}
}

func TestManaHealAndRecharge(t *testing.T) {
	registry := NewDefaultRegistry()
	target := &skillTarget{mp: 70, maxMP: 100, recharge: 0.5}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "MANAHEAL", Power: 50},
		Targets: []Actor{target},
	})
	if target.mp != 100 {
		t.Fatalf("MANAHEAL mp = %v, want clamped to 100", target.mp)
	}

	target.mp = 10
	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "MANARECHARGE", Power: 50},
		Targets: []Actor{target},
	})
	if target.mp != 35 {
		t.Fatalf("MANARECHARGE mp = %v, want 35", target.mp)
	}
}

func TestCombatPointHealClampsAndSkipsInvalidTargets(t *testing.T) {
	registry := NewDefaultRegistry()
	// COMBATPOINTHEAL is player-only: CP lives on a player character.
	target := &skillTarget{isPlayer: true, cp: 80, maxCP: 100}
	dead := &skillTarget{isPlayer: true, cp: 1, maxCP: 100, dead: true}
	invulnerable := &skillTarget{isPlayer: true, cp: 1, maxCP: 100, invulnerable: true}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "COMBATPOINTHEAL", Power: 40},
		Targets: []Actor{target, dead, invulnerable},
	})
	if target.cp != 100 {
		t.Fatalf("cp = %v, want clamped to 100", target.cp)
	}
	if dead.cp != 1 || invulnerable.cp != 1 {
		t.Fatalf("invalid target cp changed: dead=%v invulnerable=%v", dead.cp, invulnerable.cp)
	}
}

func TestResourceHealNotificationsMatchCasterBranches(t *testing.T) {
	for _, tc := range []struct {
		name, skillType, kind string
		casterPlayer          bool
		wantOther             bool
	}{
		{"heal player caster", "HEAL", "hp", true, true},
		{"mana heal npc caster", "MANAHEAL", "mp", false, false},
		{"mana heal player caster", "MANAHEAL", "mp", true, true},
		{"heal percent npc caster", "HEAL_PERCENT", "hp", false, true},
		{"mana heal percent npc caster", "MANAHEAL_PERCENT", "mp", false, true},
		{"combat point player caster", "COMBATPOINTHEAL", "cp", true, true},
		{"combat point npc caster", "COMBATPOINTHEAL", "cp", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caster := &skillTarget{isPlayer: tc.casterPlayer, name: "Caster", healAmount: 50, healOK: true}
			target := &skillTarget{isPlayer: true, hp: 90, maxHP: 100, mp: 90, maxMP: 100, cp: 90, maxCP: 100}
			NewDefaultRegistry().Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: tc.skillType, Power: 50}, Targets: []Actor{target}})
			if target.noticeKind != tc.kind || target.noticeName != "Caster" || target.noticeAmount != 10 || target.noticeOther != tc.wantOther {
				t.Fatalf("notice = %q/%q/%d/%v, want %q/Caster/10/%v", target.noticeKind, target.noticeName, target.noticeAmount, target.noticeOther, tc.kind, tc.wantOther)
			}
		})
	}
}

func TestCPDamagePercentReducesCurrentCP(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{}
	target := &skillTarget{isPlayer: true, cp: 80, maxCP: 100}
	dead := &skillTarget{isPlayer: true, cp: 80, maxCP: 100, dead: true}
	invulnerable := &skillTarget{isPlayer: true, cp: 80, maxCP: 100, invulnerable: true}
	// CpDamPercent.java:33 skips every non-Player target before the
	// dead/invulnerable checks, even one carrying CP.
	nonPlayer := &skillTarget{cp: 80, maxCP: 100}

	if !registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "CPDAMPERCENT", Power: 35},
		Targets: []Actor{target, dead, invulnerable, nonPlayer, &placedFakeActor{}},
	}) {
		t.Fatal("Use() returned false for CPDAMPERCENT")
	}
	if target.cp != 52 {
		t.Fatalf("CPDAMPERCENT cp = %v, want 52", target.cp)
	}
	if dead.cp != 80 || invulnerable.cp != 80 {
		t.Fatalf("invalid target cp changed: dead=%v invulnerable=%v", dead.cp, invulnerable.cp)
	}
	if nonPlayer.cp != 80 || len(nonPlayer.castBreakDamage) != 0 {
		t.Fatalf("non-player target hit: cp=%v castBreakDamage=%v", nonPlayer.cp, nonPlayer.castBreakDamage)
	}
	if len(target.castBreakDamage) != 1 || target.castBreakDamage[0] != 28 {
		t.Fatalf("castBreakDamage = %v, want single call with 28 (CpDamPercent.java:44 calcCastBreak(targetPlayer, damage) before the CP reduction)", target.castBreakDamage)
	}
	if len(dead.castBreakDamage) != 0 || len(invulnerable.castBreakDamage) != 0 {
		t.Fatalf("cast break rolled for skipped target: dead=%v invulnerable=%v", dead.castBreakDamage, invulnerable.castBreakDamage)
	}
}

func TestBalanceLifeEqualizesLivingTargets(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{}
	a := &skillTarget{hp: 20, maxHP: 100}
	b := &skillTarget{hp: 80, maxHP: 200}
	dead := &skillTarget{hp: 1, maxHP: 100, dead: true}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "BALANCE_LIFE"},
		Targets: []Actor{a, b, dead},
	})

	if !almost(a.hp, 100.0/3.0) || !almost(b.hp, 200.0/3.0) {
		t.Fatalf("balanced hp = %v/%v, want one-third of max hp", a.hp, b.hp)
	}
	if dead.hp != 1 {
		t.Fatalf("dead hp = %v, want unchanged 1", dead.hp)
	}
}

func TestGiveSPRealDamageAndDummy(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{}
	target := &skillTarget{hp: 25, maxHP: 100}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "GIVE_SP", Power: 42.9},
		Targets: []Actor{target},
	})
	if target.sp != 42 {
		t.Fatalf("sp = %d, want truncated skill power 42", target.sp)
	}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "REAL_DAMAGE", Power: 10},
		Targets: []Actor{target},
	})
	if target.hp != 15 || target.dead {
		t.Fatalf("after nonlethal real damage hp=%v dead=%v, want 15/false", target.hp, target.dead)
	}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "REAL_DAMAGE", Power: 20},
		Targets: []Actor{target},
	})
	if !target.dead || target.diedBy != caster {
		t.Fatalf("lethal real damage dead=%v diedBy=%p, want caster %p", target.dead, target.diedBy, caster)
	}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "DUMMY", Power: 1000},
		Targets: []Actor{target},
	})
	if target.hp != 15 {
		t.Fatalf("dummy changed hp to %v, want unchanged 15", target.hp)
	}
}

func TestHealPercentAndCombatPointHealApplySkillEffects(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{}
	hp := &skillTarget{hp: 50, maxHP: 100, effects: newTestList(noopStatOwner{})}
	cp := &skillTarget{cp: 50, maxCP: 100, effects: newTestList(noopStatOwner{})}
	effects := []modelskill.EffectTemplate{{Name: "Buff", Time: 60}}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{ID: 1, SkillType: "HEAL_PERCENT", Power: 10, Effects: effects}, Targets: []Actor{hp}})
	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{ID: 2, SkillType: "COMBATPOINTHEAL", Power: 10, Effects: effects}, Targets: []Actor{cp}})

	if len(hp.effects.All()) != 1 || len(cp.effects.All()) != 1 {
		t.Fatalf("healing effects = hp %d, cp %d; want one each", len(hp.effects.All()), len(cp.effects.All()))
	}
}

// TestResourceHandlersRunBuffPassFirst drives HEAL_PERCENT,
// MANAHEAL_PERCENT, COMBATPOINTHEAL and BALANCE_LIFE through the registry:
// each lands the skill's effects once through the BUFF pass before its own
// restore, and spends the spiritshot sampled at cast start with the
// static-reuse flag, unless the skill is a potion.
func TestResourceHandlersRunBuffPassFirst(t *testing.T) {
	shots := []struct {
		name        string
		charged     map[item.ShotKind]bool
		potion      bool
		staticReuse bool
		wantShots   []item.ShotKind
		wantFlags   []bool
	}{
		{name: "plain", charged: map[item.ShotKind]bool{item.ShotSpirit: true}, wantShots: []item.ShotKind{item.ShotSpirit}, wantFlags: []bool{false}},
		{name: "blessed", charged: map[item.ShotKind]bool{item.ShotSpirit: true, item.ShotBlessedSpirit: true}, wantShots: []item.ShotKind{item.ShotBlessedSpirit}, wantFlags: []bool{false}},
		{name: "static reuse", charged: map[item.ShotKind]bool{item.ShotBlessedSpirit: true}, staticReuse: true, wantShots: []item.ShotKind{item.ShotBlessedSpirit}, wantFlags: []bool{true}},
		{name: "potion", charged: map[item.ShotKind]bool{item.ShotBlessedSpirit: true}, potion: true},
	}
	for _, skillType := range []string{"HEAL_PERCENT", "MANAHEAL_PERCENT", "COMBATPOINTHEAL", "BALANCE_LIFE"} {
		for _, shot := range shots {
			t.Run(skillType+"/"+shot.name, func(t *testing.T) {
				caster := &skillTarget{isPlayer: true, name: "Caster", charged: shot.charged, effects: newTestList(nil)}
				target := &skillTarget{isPlayer: true, hp: 50, maxHP: 100, mp: 10, maxMP: 100, cp: 10, maxCP: 100, effects: newTestList(nil)}
				NewDefaultRegistry().Use(Cast{
					Caster: caster,
					Skill: modelskill.Definition{
						ID: 1, Level: 1, SkillType: skillType, Power: 20,
						Potion: shot.potion, StaticReuse: shot.staticReuse,
						Effects: buffEffect(), SelfEffects: buffEffect(),
					},
					Targets: []Actor{target},
				})
				if !slices.Equal(caster.shots, shot.wantShots) || !slices.Equal(caster.shotFlags, shot.wantFlags) {
					t.Fatalf("spent shots = %v %v, want %v %v", caster.shots, caster.shotFlags, shot.wantShots, shot.wantFlags)
				}
				if got := len(target.effects.All()); got != 1 {
					t.Fatalf("target effects = %d, want the skill's effect exactly once", got)
				}
				if got := len(caster.effects.All()); got != 1 {
					t.Fatalf("caster self effects = %d, want 1", got)
				}
				if skillType == "HEAL_PERCENT" && target.effectsAtHeal != 1 {
					t.Fatalf("effects on target when HP was restored = %d, want 1 (BUFF pass first)", target.effectsAtHeal)
				}
			})
		}
	}
}
