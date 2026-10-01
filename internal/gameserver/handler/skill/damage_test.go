package skill

import (
	"slices"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

func TestPhysicalMagicBlowAndManaDamageHandlersUseFormulaInputs(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{}
	target := &skillTarget{
		hp: 2000,
		mp: 100,
		physicalInput: formulas.PhysicalSkillInput{
			AttackPower: 100, SkillPower: 50, Defence: 60,
			RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, ElementalMul: 1,
		},
		physicalOK: true,
		magicInput: formulas.MagicDamageInput{
			MAtk: 400, MDef: 50, SkillPower: 20,
			PvPMul: 1, ElementalMul: 1,
		},
		magicOK: true,
		blowInput: formulas.BlowInput{
			AttackPower: 100, SkillPower: 50, Defence: 40,
			RandomMul: 1, PosMul: 1.2,
			CritDamageMul: 1.5, CritDamagePosMul: 1, CritVulnMul: 1, DaggerVulnMul: 1, CritDamageAddBase: 5,
			Landed: true, Crit: true,
		},
		blowOK: true,
		manaInput: formulas.ManaDamageInput{
			MAtk: 400, MDef: 50, SkillPower: 20, TargetMaxMp: 970,
			VulnMul: 1, Affected: true,
		},
		manaOK: true,
	}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "PDAM"}, Targets: []Actor{target}})
	if !almost(target.hp, 2000-192.5) {
		t.Fatalf("PDAM hp = %v, want %v", target.hp, 2000-192.5)
	}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "MDAM"}, Targets: []Actor{target}})
	if !almost(target.hp, 2000-192.5-728) {
		t.Fatalf("MDAM hp = %v, want %v", target.hp, 2000-192.5-728)
	}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "BLOW"}, Targets: []Actor{target}})
	if !almost(target.hp, 2000-192.5-728-1154) {
		t.Fatalf("BLOW critical hp = %v, want %v", target.hp, 2000-192.5-728-1154)
	}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "MANADAM"}, Targets: []Actor{target}})
	if target.mp != 20 {
		t.Fatalf("MANADAM mp = %v, want 20", target.mp)
	}
}

func TestManaDamageHandlerReportsSystemMessages(t *testing.T) {
	registry := NewDefaultRegistry()
	manaInput := formulas.ManaDamageInput{
		MAtk: 400, MDef: 50, SkillPower: 20, TargetMaxMp: 970,
		VulnMul: 1, Affected: true,
	}

	t.Run("missed target reports MissedTarget only", func(t *testing.T) {
		caster := &skillTarget{name: "Caster"}
		target := &skillTarget{
			mp: 100, maxMP: 100,
			manaInput: formulas.ManaDamageInput{Affected: false},
			manaOK:    true,
		}
		result, ok := registry.UseResult(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "MANADAM"}, Targets: []Actor{target}})
		if !ok {
			t.Fatal("UseResult ok = false")
		}
		if result.ManaDamageMissed != 1 {
			t.Fatalf("ManaDamageMissed = %d, want 1", result.ManaDamageMissed)
		}
		if len(result.ManaDrains) != 0 || len(result.OpponentMPReduced) != 0 {
			t.Fatalf("missed target must not drain or reduce: drains=%v reduced=%v", result.ManaDrains, result.OpponentMPReduced)
		}
	})

	t.Run("invulnerable target reports MissedTarget, matching ManaDamageInput's ok=false shape", func(t *testing.T) {
		caster := &skillTarget{name: "Caster"}
		target := &skillTarget{
			mp: 100, maxMP: 100,
			invulnerable: true,
			manaOK:       false,
		}
		result, ok := registry.UseResult(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "MANADAM"}, Targets: []Actor{target}})
		if !ok {
			t.Fatal("UseResult ok = false")
		}
		if result.ManaDamageMissed != 1 {
			t.Fatalf("ManaDamageMissed = %d, want 1", result.ManaDamageMissed)
		}
		if len(result.ManaDrains) != 0 || len(result.OpponentMPReduced) != 0 {
			t.Fatalf("invulnerable target must not drain or reduce: drains=%v reduced=%v", result.ManaDrains, result.OpponentMPReduced)
		}
		if target.mp != 100 {
			t.Fatalf("invulnerable target mp = %v, want unchanged 100", target.mp)
		}
	})

	t.Run("player caster and player target report both drain messages", func(t *testing.T) {
		caster := &playerActor{skillTarget{name: "Caster"}}
		target := &playerActor{skillTarget{mp: 100, maxMP: 100, manaInput: manaInput, manaOK: true}}
		target.objectID = 42

		result, ok := registry.UseResult(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "MANADAM"}, Targets: []Actor{target}})
		if !ok {
			t.Fatal("UseResult ok = false")
		}
		if len(result.ManaDrains) != 1 {
			t.Fatalf("ManaDrains = %v, want 1 entry", result.ManaDrains)
		}
		drain := result.ManaDrains[0]
		if drain.TargetID != 42 || drain.CasterName != "Caster" || drain.MP <= 0 {
			t.Fatalf("ManaDrain = %+v, unexpected", drain)
		}
		if len(result.OpponentMPReduced) != 1 || result.OpponentMPReduced[0] != drain.MP {
			t.Fatalf("OpponentMPReduced = %v, want [%v]", result.OpponentMPReduced, drain.MP)
		}
	})

	t.Run("non-player target skips the drain message but caster still gets the reduce message", func(t *testing.T) {
		caster := &playerActor{skillTarget{name: "Caster"}}
		target := &skillTarget{mp: 100, maxMP: 100, manaInput: manaInput, manaOK: true}

		result, ok := registry.UseResult(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "MANADAM"}, Targets: []Actor{target}})
		if !ok {
			t.Fatal("UseResult ok = false")
		}
		if len(result.ManaDrains) != 0 {
			t.Fatalf("ManaDrains = %v, want none for non-player target", result.ManaDrains)
		}
		if len(result.OpponentMPReduced) != 1 {
			t.Fatalf("OpponentMPReduced = %v, want 1 entry (player caster)", result.OpponentMPReduced)
		}
	})

	t.Run("non-player caster skips the reduce message but player target still gets the drain message", func(t *testing.T) {
		caster := &skillTarget{name: "Caster"}
		target := &playerActor{skillTarget{mp: 100, maxMP: 100, manaInput: manaInput, manaOK: true}}
		target.objectID = 7

		result, ok := registry.UseResult(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "MANADAM"}, Targets: []Actor{target}})
		if !ok {
			t.Fatal("UseResult ok = false")
		}
		if len(result.ManaDrains) != 1 {
			t.Fatalf("ManaDrains = %v, want 1 entry", result.ManaDrains)
		}
		if len(result.OpponentMPReduced) != 0 {
			t.Fatalf("OpponentMPReduced = %v, want none (non-player caster)", result.OpponentMPReduced)
		}
	})
}

// TestRegistryPassesMagicFailuresToMdam pins that the server's MagicFailures
// switch reaches the MDAM resist roll through the registry, with the shipped
// default on and a per-registry override off.
func TestRegistryPassesMagicFailuresToMdam(t *testing.T) {
	for _, tc := range []struct {
		name     string
		registry *Registry
		want     bool
	}{
		{"default", NewDefaultRegistry(), true},
		{"signet registry off", NewDefaultRegistryWithSignet(nil, false, nil, SignetDeps{}), false},
		{"signet registry on", NewDefaultRegistryWithSignet(nil, true, nil, SignetDeps{}), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := &skillTarget{hp: 2000}
			tc.registry.Use(Cast{Skill: modelskill.Definition{SkillType: "MDAM"}, Targets: []Actor{target}})
			if target.magicFailures == nil || *target.magicFailures != tc.want {
				t.Fatalf("MagicDamageInput magicFailures = %v, want %v", target.magicFailures, tc.want)
			}
		})
	}
}

func TestMdamHalfFailureHalvesDamageAndReportsAttackFailed(t *testing.T) {
	registry := NewDefaultRegistry()
	target := &skillTarget{
		hp:      2000,
		magicOK: true,
		magicInput: formulas.MagicDamageInput{
			MAtk: 400, MDef: 50, SkillPower: 20,
			PvPMul: 1, ElementalMul: 1,
			Failure: formulas.MagicFailureHalf,
		},
	}
	result, ok := registry.UseResult(Cast{
		Skill:   modelskill.Definition{SkillType: "MDAM"},
		Targets: []Actor{target},
	})
	if !ok {
		t.Fatal("UseResult() handled = false, want true for MDAM")
	}
	if result.AttackFailed != 1 {
		t.Fatalf("AttackFailed = %d, want 1", result.AttackFailed)
	}
	if !almost(target.hp, 2000-364) {
		t.Fatalf("MDAM half-fail hp = %v, want %v", target.hp, 2000-364)
	}
}

func TestMdamFullFailureFlattensDamage(t *testing.T) {
	registry := NewDefaultRegistry()
	target := &skillTarget{
		hp:      2000,
		magicOK: true,
		magicInput: formulas.MagicDamageInput{
			MAtk: 400, MDef: 50, SkillPower: 20,
			PvPMul: 1, ElementalMul: 1, MagicCrit: true,
			Failure: formulas.MagicFailureFull,
		},
	}
	result, ok := registry.UseResult(Cast{
		Skill:   modelskill.Definition{SkillType: "MDAM"},
		Targets: []Actor{target},
	})
	if !ok {
		t.Fatal("UseResult() handled = false, want true for MDAM")
	}
	if result.AttackFailed != 0 {
		t.Fatalf("AttackFailed = %d, want 0", result.AttackFailed)
	}
	if !almost(target.hp, 1999) {
		t.Fatalf("MDAM full-fail hp = %v, want 1999", target.hp)
	}
}

func TestMdamPerfectShieldDealsOneAndSkipsFailureFeedback(t *testing.T) {
	registry := NewDefaultRegistry()
	target := &skillTarget{
		hp:      2000,
		magicOK: true,
		magicInput: formulas.MagicDamageInput{
			MAtk: 400, MDef: 50, SkillPower: 20,
			PvPMul: 1.1, ElementalMul: 2, MagicCrit: true,
			Failure: formulas.MagicFailureHalf,
			Shield:  formulas.ShieldPerfect,
		},
	}
	result, ok := registry.UseResult(Cast{
		Skill:   modelskill.Definition{SkillType: "MDAM"},
		Targets: []Actor{target},
	})
	if !ok {
		t.Fatal("UseResult() handled = false, want true for MDAM")
	}
	if result.AttackFailed != 0 {
		t.Fatalf("AttackFailed = %d, want 0 on perfect shield", result.AttackFailed)
	}
	if !almost(target.hp, 1999) {
		t.Fatalf("MDAM perfect-shield hp = %v, want 1999", target.hp)
	}
}

func TestMdamReusesResolvedShieldForEffectLanding(t *testing.T) {
	registry := NewDefaultRegistry()
	target := &skillTarget{
		hp:             2000,
		magicOK:        true,
		skillSuccessOK: true,
		effects:        newTestList(nil),
		magicInput: formulas.MagicDamageInput{
			MAtk: 400, MDef: 50, SkillPower: 20,
			PvPMul: 1, ElementalMul: 1,
			Shield: formulas.ShieldPerfect,
		},
	}
	result, ok := registry.UseResult(Cast{
		Skill: modelskill.Definition{
			SkillType: "MDAM",
			Effects:   []modelskill.EffectTemplate{{Name: "Stun", Time: 10}},
		},
		Targets: []Actor{target},
	})
	if !ok {
		t.Fatal("UseResult() handled = false, want true for MDAM")
	}
	if target.lastShield != formulas.ShieldPerfect {
		t.Fatalf("effect-landing shield = %v, want ShieldPerfect from MagicDamageInput", target.lastShield)
	}
	if len(target.effects.All()) != 0 {
		t.Fatal("perfect shield must block MDAM effects")
	}
	if !almost(target.hp, 1999) {
		t.Fatalf("MDAM perfect-shield hp = %v, want 1999", target.hp)
	}
	if result.AttackFailed != 0 {
		t.Fatalf("AttackFailed = %d, want 0", result.AttackFailed)
	}
}

func TestPdamReportsDodgeWithoutDealingDamage(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{}
	target := &skillTarget{
		hp:            1000,
		physicalInput: formulas.PhysicalSkillInput{Evaded: true},
		physicalOK:    true,
	}

	result, _ := registry.UseResult(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "PDAM"}, Targets: []Actor{target}})
	if target.hp != 1000 || len(result.Dodges) != 1 {
		t.Fatalf("PDAM dodge = hp %v, dodges %d; want hp 1000 and one dodge", target.hp, len(result.Dodges))
	}
}

func TestBlowSkipsAlikeDeadTargets(t *testing.T) {
	registry := NewDefaultRegistry()
	target := &skillTarget{
		hp:        1000,
		alikeDead: true,
		blowInput: formulas.BlowInput{Landed: true, AttackPower: 100, SkillPower: 50, Defence: 50, RandomMul: 1, PosMul: 1},
		blowOK:    true,
	}

	registry.Use(Cast{Caster: &skillTarget{}, Skill: modelskill.Definition{SkillType: "BLOW"}, Targets: []Actor{target}})
	if target.hp != 1000 {
		t.Fatalf("BLOW changed alike-dead target hp to %v, want 1000", target.hp)
	}
}

func TestPhysicalAndBlowHandlersResolveLethalHits(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{}
	target := &skillTarget{
		hp:           2000,
		cp:           300,
		lethalPlayer: true,
		physicalInput: formulas.PhysicalSkillInput{
			AttackPower: 100, SkillPower: 50, Defence: 60,
			RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, ElementalMul: 1,
		},
		physicalOK: true,
		blowInput: formulas.BlowInput{
			AttackPower: 100, SkillPower: 50, Defence: 40,
			RandomMul: 1, PosMul: 1.2,
			CritDamageMul: 1.5, CritDamagePosMul: 1, CritVulnMul: 1, DaggerVulnMul: 1, CritDamageAddBase: 5,
			Landed: true,
		},
		blowOK: true,
		lethalInput: formulas.LethalInput{
			AttackerLevel: 40,
			TargetLevel:   40,
			LethalMul:     1,
		},
		lethalOK: true,
	}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "PDAM", LethalChance2: 100},
		Targets: []Actor{target},
	})
	if target.hp != 1 || target.cp != 1 {
		t.Fatalf("PDAM lethal2 hp/cp = %v/%v, want 1/1", target.hp, target.cp)
	}
	if len(target.lethalOutcomes) != 1 || target.lethalOutcomes[0] != formulas.LethalFull {
		t.Fatalf("PDAM lethal outcomes = %v, want [LethalFull]", target.lethalOutcomes)
	}

	target.hp = 2000
	target.cp = 300
	target.lethalOutcomes = nil

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "BLOW", LethalChance1: 100},
		Targets: []Actor{target},
	})
	if !almost(target.hp, 1423) || target.cp != 1 {
		t.Fatalf("BLOW lethal1 hp/cp = %v/%v, want 1423/1", target.hp, target.cp)
	}
	if len(target.lethalOutcomes) != 1 || target.lethalOutcomes[0] != formulas.LethalHalf {
		t.Fatalf("BLOW lethal outcomes = %v, want [LethalHalf]", target.lethalOutcomes)
	}
}

// TestBlowMissStillResolvesLethalHit guards Blow.java:117-118, which rolls
// calcLethalHit outside/after the landing-rate gate: a missed blow can
// still proc a lethal strike.
func TestBlowMissStillResolvesLethalHit(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{}
	target := &skillTarget{
		hp: 2000,
		cp: 300,
		blowInput: formulas.BlowInput{
			AttackPower: 100, SkillPower: 50, Defence: 40,
			RandomMul: 1, PosMul: 1.2,
			CritDamageMul: 1.5, CritDamagePosMul: 1, CritVulnMul: 1, DaggerVulnMul: 1, CritDamageAddBase: 5,
			Landed: false,
		},
		blowOK: true,
		lethalInput: formulas.LethalInput{
			AttackerLevel: 40,
			TargetLevel:   40,
			LethalMul:     1,
		},
		lethalOK: true,
	}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "BLOW", LethalChance2: 100},
		Targets: []Actor{target},
	})
	if target.hp != 1 {
		t.Fatalf("BLOW miss hp = %v, want 1 (lethal full still fires despite the miss)", target.hp)
	}
	if len(target.lethalOutcomes) != 1 || target.lethalOutcomes[0] != formulas.LethalFull {
		t.Fatalf("BLOW miss lethal outcomes = %v, want [LethalFull]", target.lethalOutcomes)
	}
}

// TestManadamStopsSleepAndImmobileOnDrain guards Manadam.java:62-66, which
// stops SLEEP and IMMOBILE_UNTIL_ATTACKED once the raw (pre-clamp) drain is
// positive. mp is set to 0 so the clamped drain is 0 while raw damage stays
// positive: a handler that (wrongly) gates on the post-clamp drain instead
// of the reference's pre-clamp raw damage would fail this test.
func TestManadamStopsSleepAndImmobileOnDrain(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{}
	target := &skillTarget{
		mp: 0,
		manaInput: formulas.ManaDamageInput{
			MAtk: 400, MDef: 50, SkillPower: 20, TargetMaxMp: 970,
			VulnMul: 1, Affected: true,
		},
		manaOK:  true,
		effects: newTestList(noopStatOwner{}),
	}
	for _, name := range []string{"Sleep", "ImmobileUntilAttacked"} {
		e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: name})
		if err != nil {
			t.Fatalf("build %s effect: %v", name, err)
		}
		e.Effected = target
		target.effects.Add(e)
	}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "MANADAM"}, Targets: []Actor{target}})

	if all := target.effects.All(); len(all) != 0 {
		t.Fatalf("MANADAM drain left effects = %v, want none (Sleep and ImmobileUntilAttacked should be stopped)", all)
	}
}

// TestManadamStopsSleepBeforeTheDrainMessages streams a MANADAM cast's
// messages through Cast.Sink: the target's Sleep is already gone when the
// drain messages go out, as Manadam.java stops it before sending them, and
// every message reaches the sink in production order instead of
// Result.Messages (issue #2589).
func TestManadamStopsSleepBeforeTheDrainMessages(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{name: "Caster", isPlayer: true}
	target := &skillTarget{
		name: "Target", isPlayer: true, mp: 100, maxMP: 100,
		manaInput: formulas.ManaDamageInput{
			MAtk: 400, MDef: 50, SkillPower: 20, TargetMaxMp: 970,
			VulnMul: 1, Affected: true,
		},
		manaOK:  true,
		effects: newTestList(noopStatOwner{}),
	}
	e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: "Sleep"})
	if err != nil {
		t.Fatalf("build Sleep effect: %v", err)
	}
	e.Effected = target
	target.effects.Add(e)

	var streamed []any
	result, ok := registry.UseResult(Cast{
		Caster: caster, Skill: modelskill.Definition{SkillType: "MANADAM"}, Targets: []Actor{target},
		Sink: func(message any) {
			if n := len(target.effects.All()); n != 0 {
				t.Errorf("%T delivered while the target still has %d effects, want Sleep stopped first", message, n)
			}
			streamed = append(streamed, message)
		},
	})
	if !ok {
		t.Fatal("UseResult ok = false")
	}
	if len(result.Messages) != 0 {
		t.Fatalf("Result.Messages = %#v, want none once a sink took them", result.Messages)
	}
	if len(streamed) != 2 {
		t.Fatalf("streamed = %#v, want the target's drain then the caster's MP report", streamed)
	}
	if _, ok := streamed[0].(ManaDrain); !ok {
		t.Fatalf("streamed[0] = %#v, want ManaDrain", streamed[0])
	}
	if _, ok := streamed[1].(OpponentMPReducedMessage); !ok {
		t.Fatalf("streamed[1] = %#v, want OpponentMPReducedMessage", streamed[1])
	}
}

// reflectedDamageCase is one damage handler whose reflect branch swaps the
// effect participants: the reflecting target becomes the effector and the
// caster the effected, as Pdam/Mdam/Blow/L2SkillChargeDmg/L2SkillDrain's
// getEffects(targetCreature, creature) does.
type reflectedDamageCase struct {
	skillType string
	reflector func() *skillTarget
}

func reflectedDamageCases() []reflectedDamageCase {
	target := func() *skillTarget {
		return &skillTarget{
			fakeActor: fakeActor{objectID: 2}, hp: 5000, isPlayer: true, name: "Reflector",
			effects: newTestList(nil), reflects: true, skillSuccessOK: true,
			physicalInput: formulas.PhysicalSkillInput{
				AttackPower: 100, SkillPower: 50, Defence: 60,
				RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, ElementalMul: 1,
			},
			physicalOK: true,
			magicInput: formulas.MagicDamageInput{MAtk: 400, MDef: 50, SkillPower: 20, PvPMul: 1, ElementalMul: 1},
			magicOK:    true,
			blowInput:  formulas.BlowInput{Landed: true, AttackPower: 100, SkillPower: 50, Defence: 50, RandomMul: 1, PosMul: 1},
			blowOK:     true,
		}
	}
	return []reflectedDamageCase{{"PDAM", target}, {"MDAM", target}, {"BLOW", target}, {"CHARGEDAM", target}, {"DRAIN", target}}
}

func reflectCaster() *skillTarget {
	return &skillTarget{
		fakeActor: fakeActor{objectID: 1}, hp: 5000, maxHP: 5000, isPlayer: true, name: "Caster",
		effects: newTestList(nil), skillSuccessOK: true,
	}
}

// TestReflectedDamageSkillSwapsEffectorAndEffected pins the reflect swap on
// every damage handler: a self-target kind (StunSelf, skill 81's effect) is
// hosted by the reflecting target, an ordinary kind lands on the caster, and
// both name the reflector as effector and the caster as effected. The
// caster's own landing roll is forced to fail: a reflected landing rolls
// none.
func TestReflectedDamageSkillSwapsEffectorAndEffected(t *testing.T) {
	for _, tc := range reflectedDamageCases() {
		t.Run(tc.skillType, func(t *testing.T) {
			caster := reflectCaster()
			caster.skillSuccessChance = chanceOf(0)
			reflector := tc.reflector()
			NewDefaultRegistry().UseResult(Cast{
				Caster: caster,
				Skill: modelskill.Definition{
					ID: 81, Level: 1, SkillType: tc.skillType, CanBeReflected: true, Offensive: true,
					Effects: []modelskill.EffectTemplate{{Name: "StunSelf", Time: 9}, {Name: "Debuff", Time: 9}},
				},
				Targets: []Actor{reflector},
			})

			held := reflector.effects.All()
			if len(held) != 1 || held[0].Type != effect.TypeStunSelf || held[0].Effector != effect.Actor(reflector) || held[0].Effected != effect.Actor(caster) {
				t.Fatalf("reflector-held effects = %+v, want one StunSelf with effector reflector and effected caster", held)
			}
			landed := caster.effects.All()
			if len(landed) != 1 || landed[0].Type != effect.TypeDebuff || landed[0].Effector != effect.Actor(reflector) || landed[0].Effected != effect.Actor(caster) {
				t.Fatalf("caster-held effects = %+v, want one Debuff with effector reflector and effected caster", landed)
			}
		})
	}
}

// TestReflectedDamageSkillReportsResistToTheReflector pins who hears a
// reflected template's landing resist: the reflector, as the effects'
// effector, naming the caster at the cast level — never the caster.
func TestReflectedDamageSkillReportsResistToTheReflector(t *testing.T) {
	for _, tc := range reflectedDamageCases() {
		t.Run(tc.skillType, func(t *testing.T) {
			caster := reflectCaster()
			reflector := tc.reflector()
			result, _ := NewDefaultRegistry().UseResult(Cast{
				Caster: caster,
				Skill: modelskill.Definition{
					ID: 7, Level: 20, SkillType: tc.skillType, CanBeReflected: true,
					Effects: resistedIconTemplate,
				},
				Targets: []Actor{reflector},
			})
			if len(result.Resisted) != 0 {
				t.Fatalf("caster-facing Resisted = %+v, want none", result.Resisted)
			}
			want := []Resisted{{TargetName: "Caster", SkillID: 7, SkillLevel: 20}}
			if !slices.Equal(reflector.resistNotices, want) {
				t.Fatalf("reflector resist notices = %+v, want %+v", reflector.resistNotices, want)
			}
		})
	}
}

// TestReflectedDamageSkillGatesOnTheSwappedPair pins the landing gates on
// the swapped pair: an invulnerable caster refuses the reflected offensive
// effects, and a perfect shield block of the original strike does not stop
// them.
func TestReflectedDamageSkillGatesOnTheSwappedPair(t *testing.T) {
	def := modelskill.Definition{
		ID: 7, Level: 1, SkillType: "PDAM", CanBeReflected: true, Offensive: true,
		Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 9}},
	}
	t.Run("invulnerable caster", func(t *testing.T) {
		caster := &guardedSkillTarget{skillTarget: reflectCaster(), invul: true}
		reflector := reflectedDamageCases()[0].reflector()
		NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{reflector}})
		if got := caster.effects.All(); len(got) != 0 {
			t.Fatalf("invulnerable caster effects = %+v, want none", got)
		}
	})
	t.Run("perfect shield", func(t *testing.T) {
		caster := reflectCaster()
		reflector := reflectedDamageCases()[0].reflector()
		reflector.physicalInput.Shield = formulas.ShieldPerfect
		NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{reflector}})
		if got := caster.effects.All(); len(got) != 1 || got[0].Effector != effect.Actor(reflector) {
			t.Fatalf("caster effects = %+v, want the reflected Debuff despite the perfect block", got)
		}
	})
}
