package skill

import (
	"fmt"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// TestPdamSparesPlayablesForFearSparingSkills pins Pdam.java:56: a PDAM
// whose id is in EffectFear.DOESNT_AFFECT_PLAYABLE (98 Sword Symphony)
// skips a player or summon target whole (no dodge, damage or lethal
// strike) while a monster in the same cast is still hit. Another skill id
// hits the playable as usual.
func TestPdamSparesPlayablesForFearSparingSkills(t *testing.T) {
	newTarget := func(kind actor.Kind, evaded bool) *skillTarget {
		return &skillTarget{
			hp: 2000, cp: 300, kind: kind,
			physicalInput: formulas.PhysicalSkillInput{
				AttackPower: 100, SkillPower: 50, Defence: 60, Evaded: evaded,
				RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, ElementalMul: 1,
			},
			physicalOK:  true,
			lethalInput: formulas.LethalInput{AttackerLevel: 40, TargetLevel: 40, LethalMul: 1},
			lethalOK:    true,
		}
	}
	for _, tc := range []struct {
		id       modelskill.ID
		kind     actor.Kind
		evaded   bool
		wantHit  bool
		wantDodg int
	}{
		{id: 98, kind: actor.KindPlayer, wantHit: false},
		{id: 98, kind: actor.KindSummon, wantHit: false},
		{id: 98, kind: actor.KindPlayer, evaded: true, wantHit: false},
		{id: 98, kind: actor.KindNPC, wantHit: true},
		{id: 98, kind: actor.KindNPC, evaded: true, wantHit: false, wantDodg: 1},
		{id: 99, kind: actor.KindPlayer, wantHit: true},
		{id: 99, kind: actor.KindSummon, wantHit: true},
	} {
		t.Run(fmt.Sprintf("skill%d/kind%d/evaded%v", tc.id, tc.kind, tc.evaded), func(t *testing.T) {
			target := newTarget(tc.kind, tc.evaded)
			result, _ := NewDefaultRegistry().UseResult(Cast{
				Caster:  &skillTarget{},
				Skill:   modelskill.Definition{ID: tc.id, Level: 1, SkillType: "PDAM", LethalChance1: 100},
				Targets: []Actor{target},
			})
			if hit := target.hp < 2000; hit != tc.wantHit {
				t.Fatalf("target hp = %v, hit %v; want hit %v", target.hp, hit, tc.wantHit)
			}
			if tc.wantHit && len(target.lethalOutcomes) != 1 {
				t.Fatalf("lethal outcomes = %v, want one roll on a hit target", target.lethalOutcomes)
			}
			if !tc.wantHit && len(target.lethalOutcomes) != 0 {
				t.Fatalf("lethal outcomes = %v, want none on an untouched target", target.lethalOutcomes)
			}
			if len(result.Dodges) != tc.wantDodg {
				t.Fatalf("dodges = %d, want %d", len(result.Dodges), tc.wantDodg)
			}
		})
	}
}

// TestBlowSelfEffectsFollowEachProcessedTarget pins Blow.java:118-127: the
// self effects land inside the target loop after the lethal roll, so an
// evaded or alike-dead target and an empty target list grant none, while a
// landed or missed blow on a live target grants them.
func TestBlowSelfEffectsFollowEachProcessedTarget(t *testing.T) {
	blow := func(landed, evaded bool) formulas.BlowInput {
		return formulas.BlowInput{
			AttackPower: 100, SkillPower: 50, Defence: 50, RandomMul: 1, PosMul: 1,
			Landed: landed, Evaded: evaded,
		}
	}
	for _, tc := range []struct {
		name    string
		targets func() []Actor
		want    int
	}{
		{name: "no target", targets: func() []Actor { return nil }, want: 0},
		{name: "evaded", targets: func() []Actor {
			return []Actor{&skillTarget{hp: 1000, blowInput: blow(true, true), blowOK: true}}
		}, want: 0},
		{name: "alike dead", targets: func() []Actor {
			return []Actor{&skillTarget{hp: 1000, alikeDead: true, blowInput: blow(true, false), blowOK: true}}
		}, want: 0},
		{name: "landed", targets: func() []Actor {
			return []Actor{&skillTarget{hp: 1000, blowInput: blow(true, false), blowOK: true}}
		}, want: 1},
		{name: "missed", targets: func() []Actor {
			return []Actor{&skillTarget{hp: 1000, blowInput: blow(false, false), blowOK: true}}
		}, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caster := &skillTarget{effects: newTestList(nil)}
			NewDefaultRegistry().Use(Cast{
				Caster:  caster,
				Skill:   modelskill.Definition{ID: 321, Level: 1, SkillType: "BLOW", SelfEffects: buffEffect()},
				Targets: tc.targets(),
			})
			if got := len(caster.effects.All()); got != tc.want {
				t.Fatalf("caster self effects = %d, want %d", got, tc.want)
			}
		})
	}
}

// manadamFake is a MANADAM target whose mana hit always connects.
type manadamFake struct {
	*disablerFake
}

func (*manadamFake) ManaDamageInput(creature.FormulaActor, modelskill.Definition) (formulas.ManaDamageInput, bool) {
	return formulas.ManaDamageInput{MAtk: 400, MDef: 50, SkillPower: 20, TargetMaxMp: 100, VulnMul: 1, Affected: true}, true
}

// TestManadamPassesBlessedShotAndShieldToTemplateLanding pins
// Manadam.java:56-63: one shield sample feeds both the skill roll and the
// effect-power landing of every template, together with the caster's
// blessed-spiritshot charge; a perfect block lands nothing.
func TestManadamPassesBlessedShotAndShieldToTemplateLanding(t *testing.T) {
	for _, tc := range []struct {
		bss        bool
		shield     formulas.ShieldDefense
		wantInputs []templateLanding
		wantLanded int
	}{
		{bss: true, shield: formulas.ShieldSuccess, wantInputs: []templateLanding{{bss: true, shield: formulas.ShieldSuccess}}, wantLanded: 1},
		{bss: false, shield: formulas.ShieldFailed, wantInputs: []templateLanding{{bss: false, shield: formulas.ShieldFailed}}, wantLanded: 1},
		{bss: true, shield: formulas.ShieldPerfect},
	} {
		t.Run(fmt.Sprintf("bss%v/shield%d", tc.bss, tc.shield), func(t *testing.T) {
			target := &manadamFake{newDisablerFake(2)}
			target.shield = tc.shield
			NewDefaultRegistry().Use(Cast{
				Caster:  &bssCasterFake{bss: tc.bss},
				Skill:   modelskill.Definition{ID: 1102, Level: 1, SkillType: "MANADAM", Magic: true, Effects: rolledStun()},
				Targets: []Actor{target},
			})
			if target.shieldRolls != 1 {
				t.Fatalf("shield rolls = %d, want 1 per target", target.shieldRolls)
			}
			if target.lastBss != tc.bss || target.lastShield != tc.shield {
				t.Fatalf("skill roll inputs = bss %v shield %d, want bss %v shield %d", target.lastBss, target.lastShield, tc.bss, tc.shield)
			}
			if !slices.Equal(target.templateLandings, tc.wantInputs) {
				t.Fatalf("template landing inputs = %+v, want %+v", target.templateLandings, tc.wantInputs)
			}
			if got := len(target.list.All()); got != tc.wantLanded {
				t.Fatalf("landed effects = %d, want %d", got, tc.wantLanded)
			}
		})
	}
}
