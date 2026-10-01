package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// TestMdamTagsResistedByOrigin pins the fix to a PR-2356 review comment:
// Mdam's own effect-success roll (Mdam.java:69, unconditional
// creature.sendPacket) must tag Resisted.Unconditional true, while a
// resisted per-effect-template landing (L2Skill.java:1196-1197, gated
// `effector instanceof Player`) must tag it false. Mdam.java:69 also adds the
// skill by id only, so its own resist carries level 1 while the per-effect
// landing (and every sibling handler) carries the cast level, 20 here.
func TestMdamTagsResistedByOrigin(t *testing.T) {
	registry := NewDefaultRegistry()
	magicInput := formulas.MagicDamageInput{MAtk: 400, MDef: 50, SkillPower: 20, PvPMul: 1, ElementalMul: 1}

	t.Run("skill's own effect-success roll fails", func(t *testing.T) {
		target := &skillTarget{
			hp: 2000, effects: newTestList(nil),
			magicInput: magicInput, magicOK: true,
			skillSuccessOK: true, skillSuccessChance: chanceOf(0),
		}
		result, ok := registry.UseResult(Cast{
			Skill:   modelskill.Definition{ID: 7, Level: 20, SkillType: "MDAM", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
			Targets: []Actor{target},
		})
		if !ok {
			t.Fatal("UseResult() handled = false, want true for MDAM")
		}
		if len(result.Resisted) != 1 || !result.Resisted[0].Unconditional || result.Resisted[0].SkillLevel != 1 {
			t.Fatalf("Resisted = %+v, want one entry with Unconditional=true, SkillLevel=1", result.Resisted)
		}
	})

	t.Run("per-effect-template landing resists", func(t *testing.T) {
		target := &skillTarget{
			hp: 2000, effects: newTestList(nil),
			magicInput: magicInput, magicOK: true,
			skillSuccessOK: true,
		}
		result, ok := registry.UseResult(Cast{
			Skill:   modelskill.Definition{ID: 7, Level: 20, SkillType: "MDAM", Effects: resistedIconTemplate},
			Targets: []Actor{target},
		})
		if !ok {
			t.Fatal("UseResult() handled = false, want true for MDAM")
		}
		if len(result.Resisted) != 1 || result.Resisted[0].Unconditional || result.Resisted[0].SkillLevel != 20 {
			t.Fatalf("Resisted = %+v, want one entry with Unconditional=false, SkillLevel=20", result.Resisted)
		}
	})
}

// TestBlowTagsResistedByOrigin mirrors TestMdamTagsResistedByOrigin for
// Blow.java:74's unconditional resist vs. the gated per-effect one.
func TestBlowTagsResistedByOrigin(t *testing.T) {
	registry := NewDefaultRegistry()
	blowInput := formulas.BlowInput{Landed: true, AttackPower: 100, SkillPower: 50, Defence: 50, RandomMul: 1, PosMul: 1}

	t.Run("skill's own effect-success roll fails", func(t *testing.T) {
		target := &skillTarget{
			hp: 2000, effects: newTestList(nil),
			blowInput: blowInput, blowOK: true,
			skillSuccessOK: true, skillSuccessChance: chanceOf(0),
		}
		result, ok := registry.UseResult(Cast{
			Skill:   modelskill.Definition{ID: 7, Level: 20, SkillType: "BLOW", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
			Targets: []Actor{target},
		})
		if !ok {
			t.Fatal("UseResult() handled = false, want true for BLOW")
		}
		if len(result.Resisted) != 1 || !result.Resisted[0].Unconditional || result.Resisted[0].SkillLevel != 20 {
			t.Fatalf("Resisted = %+v, want one entry with Unconditional=true, SkillLevel=20", result.Resisted)
		}
	})

	t.Run("per-effect-template landing resists", func(t *testing.T) {
		target := &skillTarget{
			hp: 2000, effects: newTestList(nil),
			blowInput: blowInput, blowOK: true,
			skillSuccessOK: true,
		}
		result, ok := registry.UseResult(Cast{
			Skill:   modelskill.Definition{ID: 7, Level: 20, SkillType: "BLOW", Effects: resistedIconTemplate},
			Targets: []Actor{target},
		})
		if !ok {
			t.Fatal("UseResult() handled = false, want true for BLOW")
		}
		if len(result.Resisted) != 1 || result.Resisted[0].Unconditional || result.Resisted[0].SkillLevel != 20 {
			t.Fatalf("Resisted = %+v, want one entry with Unconditional=false, SkillLevel=20", result.Resisted)
		}
	})
}

type counteringSkillTarget struct {
	*skillTarget
}

func (*counteringSkillTarget) CounterSkillPhysical() float64 { return 100 }

func TestBlowReportsResistBeforeCounter(t *testing.T) {
	target := &counteringSkillTarget{skillTarget: &skillTarget{
		hp: 2000, effects: newTestList(nil),
		blowInput: formulas.BlowInput{Landed: true, AttackPower: 100, SkillPower: 50, Defence: 50, RandomMul: 1, PosMul: 1}, blowOK: true,
		skillSuccessOK: true, skillSuccessChance: chanceOf(0),
	}}
	result, ok := NewDefaultRegistry().UseResult(Cast{
		Caster: &skillTarget{hp: 2000},
		Skill: modelskill.Definition{
			ID: 7, Level: 20, SkillType: "BLOW", CastRange: 40, CanBeReflected: true,
			Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}},
		},
		Targets: []Actor{target},
	})
	if !ok || len(result.Messages) != 2 {
		t.Fatalf("messages = %#v, want resist then counter", result.Messages)
	}
	if _, ok := result.Messages[0].(Resisted); !ok {
		t.Fatalf("first message = %T, want Resisted", result.Messages[0])
	}
	if _, ok := result.Messages[1].(Counterattack); !ok {
		t.Fatalf("second message = %T, want Counterattack", result.Messages[1])
	}
}

func TestPdamReportsTargetsInOrder(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{hp: 2000}
	damage := formulas.PhysicalSkillInput{
		AttackPower: 100, SkillPower: 50, Defence: 60,
		RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, ElementalMul: 1,
	}

	dodger := &skillTarget{physicalInput: formulas.PhysicalSkillInput{Evaded: true}, physicalOK: true}
	counter := &counteringSkillTarget{skillTarget: &skillTarget{hp: 2000, physicalInput: damage, physicalOK: true}}
	result, _ := registry.UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "PDAM", CastRange: 40, CanBeReflected: true},
		Targets: []Actor{dodger, counter},
	})
	if len(result.Messages) != 2 {
		t.Fatalf("dodge/counter messages = %#v", result.Messages)
	}
	if _, ok := result.Messages[0].(Dodge); !ok {
		t.Fatalf("first message = %T, want Dodge", result.Messages[0])
	}
	if _, ok := result.Messages[1].(Counterattack); !ok {
		t.Fatalf("second message = %T, want Counterattack", result.Messages[1])
	}

	failed := &skillTarget{physicalInput: formulas.PhysicalSkillInput{}, physicalOK: true}
	lethal := &skillTarget{
		hp: 2000, physicalInput: damage, physicalOK: true,
		lethalInput: formulas.LethalInput{AttackerLevel: 40, TargetLevel: 40, LethalMul: 1}, lethalOK: true,
	}
	result, _ = registry.UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "PDAM", LethalChance2: 100},
		Targets: []Actor{failed, lethal},
	})
	if len(result.Messages) != 2 {
		t.Fatalf("failed/lethal messages = %#v", result.Messages)
	}
	if _, ok := result.Messages[0].(AttackFailedMessage); !ok {
		t.Fatalf("first message = %T, want AttackFailedMessage", result.Messages[0])
	}
	if _, ok := result.Messages[1].(Lethal); !ok {
		t.Fatalf("second message = %T, want Lethal", result.Messages[1])
	}
}

func TestManadamReportsMissBeforeLaterResist(t *testing.T) {
	missed := &skillTarget{manaInput: formulas.ManaDamageInput{Affected: false}, manaOK: true}
	resisted := &skillTarget{
		mp: 100, effects: newTestList(nil), skillSuccessOK: true,
		skillSuccessChance: chanceOf(0),
		manaInput:          formulas.ManaDamageInput{MAtk: 400, MDef: 50, SkillPower: 20, TargetMaxMp: 100, VulnMul: 1, Affected: true},
		manaOK:             true,
	}
	result, _ := NewDefaultRegistry().UseResult(Cast{
		Caster:  &skillTarget{},
		Skill:   modelskill.Definition{ID: 7, Level: 1, SkillType: "MANADAM", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{missed, resisted},
	})
	if len(result.Messages) < 2 {
		t.Fatalf("messages = %#v, want miss before resist", result.Messages)
	}
	if _, ok := result.Messages[0].(ManaDamageMissedMessage); !ok {
		t.Fatalf("first message = %T, want ManaDamageMissedMessage", result.Messages[0])
	}
	if _, ok := result.Messages[1].(Resisted); !ok {
		t.Fatalf("second message = %T, want Resisted", result.Messages[1])
	}
}

type interleavedMessageHandler struct{}

func (interleavedMessageHandler) Types() []string { return []string{"ORDER_TEST"} }
func (interleavedMessageHandler) Use(Cast)        {}
func (interleavedMessageHandler) UseResult(cast Cast) Result {
	result := Result{messages: cast.messages, AttackFailed: 1}
	result.record(AttackFailedMessage{})
	cast.reportResisted(cast.Targets[0], cast.Skill, 1)
	result.AttackFailed++
	result.record(AttackFailedMessage{})
	return result
}

func TestRegistryInterleavesGenericEffectReports(t *testing.T) {
	registry := NewRegistry(interleavedMessageHandler{})
	result, ok := registry.UseResult(Cast{Skill: modelskill.Definition{ID: 7, Level: 1, SkillType: "ORDER_TEST"}, Targets: []Actor{&skillTarget{}}})
	if !ok || result.AttackFailed != 2 || len(result.Resisted) != 1 || len(result.Messages) != 3 {
		t.Fatalf("result = %+v, want two failures with a resist between", result)
	}
	if _, ok := result.Messages[1].(Resisted); !ok {
		t.Fatalf("middle message = %T, want Resisted", result.Messages[1])
	}
}

// TestChargeDamTagsResistedByOrigin mirrors TestMdamTagsResistedByOrigin for
// L2SkillChargeDmg.java:77's unconditional resist vs. the gated per-effect
// one; CHARGEDAM has no damage-gate on applyChargeDamEffects, unlike
// Mdam/Blow, so no damage-input fields are needed to reach it.
func TestChargeDamTagsResistedByOrigin(t *testing.T) {
	registry := NewDefaultRegistry()
	physicalInput := formulas.PhysicalSkillInput{AttackPower: 100, SkillPower: 50, Defence: 50, RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, ElementalMul: 1}

	t.Run("skill's own effect-success roll fails", func(t *testing.T) {
		target := &skillTarget{
			hp: 2000, effects: newTestList(nil),
			physicalInput: physicalInput, physicalOK: true,
			skillSuccessOK: true, skillSuccessChance: chanceOf(0),
		}
		result, ok := registry.UseResult(Cast{
			Skill:   modelskill.Definition{ID: 7, Level: 20, SkillType: "CHARGEDAM", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
			Targets: []Actor{target},
		})
		if !ok {
			t.Fatal("UseResult() handled = false, want true for CHARGEDAM")
		}
		if len(result.Resisted) != 1 || !result.Resisted[0].Unconditional || result.Resisted[0].SkillLevel != 20 {
			t.Fatalf("Resisted = %+v, want one entry with Unconditional=true, SkillLevel=20", result.Resisted)
		}
	})

	t.Run("per-effect-template landing resists", func(t *testing.T) {
		target := &skillTarget{
			hp: 2000, effects: newTestList(nil),
			physicalInput: physicalInput, physicalOK: true,
			skillSuccessOK: true,
		}
		result, ok := registry.UseResult(Cast{
			Skill:   modelskill.Definition{ID: 7, Level: 20, SkillType: "CHARGEDAM", Effects: resistedIconTemplate},
			Targets: []Actor{target},
		})
		if !ok {
			t.Fatal("UseResult() handled = false, want true for CHARGEDAM")
		}
		if len(result.Resisted) != 1 || result.Resisted[0].Unconditional || result.Resisted[0].SkillLevel != 20 {
			t.Fatalf("Resisted = %+v, want one entry with Unconditional=false, SkillLevel=20", result.Resisted)
		}
	})
}

// TestChargeDamEvasionReportsDodgeBeforeEffects covers L2SkillChargeDmg.java:46-56:
// an evaded target gets the dodge report and skips the effect-landing roll, so
// neither a Resisted entry nor damage nor effects result, even when the
// effect-success roll would have failed.
func TestChargeDamEvasionReportsDodgeBeforeEffects(t *testing.T) {
	registry := NewDefaultRegistry()
	target := &skillTarget{
		hp: 1000, effects: newTestList(nil),
		physicalInput: formulas.PhysicalSkillInput{Evaded: true}, physicalOK: true,
		skillSuccessOK: true, skillSuccessChance: chanceOf(0),
	}

	result, ok := registry.UseResult(Cast{
		Caster:  &skillTarget{},
		Skill:   modelskill.Definition{ID: 7, Level: 20, SkillType: "CHARGEDAM", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{target},
	})
	if !ok {
		t.Fatal("UseResult() handled = false, want true for CHARGEDAM")
	}
	if len(result.Dodges) != 1 || len(result.Resisted) != 0 {
		t.Fatalf("Dodges = %d, Resisted = %+v; want one dodge and no resist", len(result.Dodges), result.Resisted)
	}
	if target.hp != 1000 || len(target.effects.All()) != 0 {
		t.Fatalf("hp = %v, effects = %d; want untouched target", target.hp, len(target.effects.All()))
	}
}

// TestManaDamageTagsResistedByOrigin mirrors TestMdamTagsResistedByOrigin
// for MANADAM's checkSkillSuccess-gated resist (Manadam.java:55) vs. the
// per-effect-template one it produces on a successful roll.
func TestManaDamageTagsResistedByOrigin(t *testing.T) {
	registry := NewDefaultRegistry()
	manaInput := formulas.ManaDamageInput{MAtk: 400, MDef: 50, SkillPower: 20, TargetMaxMp: 970, VulnMul: 1, Affected: true}

	t.Run("skill's own effect-success roll fails", func(t *testing.T) {
		target := &skillTarget{
			mp: 100, maxMP: 100, effects: newTestList(nil),
			manaInput: manaInput, manaOK: true,
			skillSuccessOK: true, skillSuccessChance: chanceOf(0),
		}
		result, ok := registry.UseResult(Cast{
			Skill:   modelskill.Definition{ID: 7, Level: 20, SkillType: "MANADAM", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
			Targets: []Actor{target},
		})
		if !ok {
			t.Fatal("UseResult() handled = false, want true for MANADAM")
		}
		if len(result.Resisted) != 1 || !result.Resisted[0].Unconditional || result.Resisted[0].SkillLevel != 20 {
			t.Fatalf("Resisted = %+v, want one entry with Unconditional=true, SkillLevel=20", result.Resisted)
		}
	})

	t.Run("per-effect-template landing resists", func(t *testing.T) {
		target := &skillTarget{
			mp: 100, maxMP: 100, effects: newTestList(nil),
			manaInput: manaInput, manaOK: true,
			skillSuccessOK: true,
		}
		result, ok := registry.UseResult(Cast{
			Skill:   modelskill.Definition{ID: 7, Level: 20, SkillType: "MANADAM", Effects: resistedIconTemplate},
			Targets: []Actor{target},
		})
		if !ok {
			t.Fatal("UseResult() handled = false, want true for MANADAM")
		}
		if len(result.Resisted) != 1 || result.Resisted[0].Unconditional || result.Resisted[0].SkillLevel != 20 {
			t.Fatalf("Resisted = %+v, want one entry with Unconditional=false, SkillLevel=20", result.Resisted)
		}
	})
}

type summonSkillCaster struct {
	*skillTarget
	owner int32
	pet   bool
}

func (*summonSkillCaster) Kind() actor.Kind { return actor.KindSummon }
func (s *summonSkillCaster) OwnerID() int32 { return s.owner }
func (s *summonSkillCaster) IsPet() bool    { return s.pet }

// TestMdamReportsDamageBeforeEachTargetsResist pins Mdam.java's per-target
// order: the damage message, then that target's effect resist, then the
// next target's damage. The first target's rolled magic critical is carried
// even though its effect resisted.
func TestMdamReportsDamageBeforeEachTargetsResist(t *testing.T) {
	in := formulas.MagicDamageInput{MAtk: 400, MDef: 100, SkillPower: 50, PvPMul: 1, ElementalMul: 1}
	crit := in
	crit.MagicCrit = true
	resister := &skillTarget{
		fakeActor: fakeActor{objectID: 2}, hp: 5000, name: "A", effects: newTestList(nil),
		skillSuccessOK: true, skillSuccessChance: chanceOf(0), magicInput: crit, magicOK: true,
	}
	hit := &skillTarget{fakeActor: fakeActor{objectID: 3}, hp: 5000, name: "B", magicInput: in, magicOK: true}
	caster := &skillTarget{fakeActor: fakeActor{objectID: 1}, isPlayer: true}

	result, _ := NewDefaultRegistry().UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{ID: 7, Level: 3, SkillType: "MDAM", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{resister, hit},
	})

	if len(result.Messages) != 3 {
		t.Fatalf("messages = %#v, want damage, resist, damage", result.Messages)
	}
	first, ok := result.Messages[0].(Damage)
	if !ok || first != (Damage{RecipientID: 1, Source: DamageByPlayer, Amount: int32(5000 - resister.hp), MagicCrit: true}) {
		t.Fatalf("first message = %#v, want A's critical damage", result.Messages[0])
	}
	if r, ok := result.Messages[1].(Resisted); !ok || r.TargetName != "A" {
		t.Fatalf("second message = %#v, want A's resist", result.Messages[1])
	}
	second, ok := result.Messages[2].(Damage)
	if !ok || second != (Damage{RecipientID: 1, Source: DamageByPlayer, Amount: int32(5000 - hit.hp)}) {
		t.Fatalf("third message = %#v, want B's damage", result.Messages[2])
	}
}

// TestPhysicalSkillsReportDamageAfterTheHit covers PDAM's plain feedback
// and BLOW's, which always reports a physical critical.
func TestPhysicalSkillsReportDamageAfterTheHit(t *testing.T) {
	pdam := formulas.PhysicalSkillInput{
		AttackPower: 100, SkillPower: 50, Defence: 60,
		RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, ElementalMul: 1,
	}
	blow := formulas.BlowInput{Landed: true, AttackPower: 100, SkillPower: 50, Defence: 50, RandomMul: 1, PosMul: 1}
	for _, tc := range []struct {
		skillType string
		pcrit     bool
	}{{"PDAM", false}, {"CHARGEDAM", false}, {"BLOW", true}} {
		t.Run(tc.skillType, func(t *testing.T) {
			target := &skillTarget{
				fakeActor: fakeActor{objectID: 2}, hp: 5000,
				physicalInput: pdam, physicalOK: true, blowInput: blow, blowOK: true,
			}
			result, _ := NewDefaultRegistry().UseResult(Cast{
				Caster:  &skillTarget{fakeActor: fakeActor{objectID: 1}, isPlayer: true},
				Skill:   modelskill.Definition{SkillType: tc.skillType},
				Targets: []Actor{target},
			})
			got := damageMessages(t, result.Messages)
			want := Damage{RecipientID: 1, Source: DamageByPlayer, Amount: int32(5000 - target.hp), PhysicalCrit: tc.pcrit}
			if len(got) != 1 || got[0] != want || want.Amount <= 0 {
				t.Fatalf("damage messages = %#v, want %#v", got, want)
			}
		})
	}
}

// TestCounteredSkillReportsCounterDamageToTheDefender pins the counter
// branch: the countering player hears about the damage it dealt back, after
// the counter notices, with BLOW's physical-critical flag.
func TestCounteredSkillReportsCounterDamageToTheDefender(t *testing.T) {
	pdam := formulas.PhysicalSkillInput{
		AttackPower: 100, SkillPower: 50, Defence: 60,
		RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, ElementalMul: 1,
	}
	blow := formulas.BlowInput{Landed: true, AttackPower: 100, SkillPower: 50, Defence: 50, RandomMul: 1, PosMul: 1}
	for _, tc := range []struct {
		skillType string
		pcrit     bool
	}{{"PDAM", false}, {"CHARGEDAM", false}, {"BLOW", true}} {
		t.Run(tc.skillType, func(t *testing.T) {
			caster := &skillTarget{fakeActor: fakeActor{objectID: 1}, hp: 5000, isPlayer: true}
			defender := &counteringSkillTarget{skillTarget: &skillTarget{
				fakeActor: fakeActor{objectID: 2}, hp: 5000, isPlayer: true,
				physicalInput: pdam, physicalOK: true, blowInput: blow, blowOK: true,
			}}
			result, _ := NewDefaultRegistry().UseResult(Cast{
				Caster:  caster,
				Skill:   modelskill.Definition{SkillType: tc.skillType, CastRange: 40, CanBeReflected: true},
				Targets: []Actor{defender},
			})
			if len(result.Messages) != 2 {
				t.Fatalf("messages = %#v, want counter then damage", result.Messages)
			}
			if _, ok := result.Messages[0].(Counterattack); !ok {
				t.Fatalf("first message = %T, want Counterattack", result.Messages[0])
			}
			want := Damage{RecipientID: 2, Source: DamageByPlayer, Amount: int32(5000 - caster.hp), PhysicalCrit: tc.pcrit}
			if got, ok := result.Messages[1].(Damage); !ok || got != want || want.Amount <= 0 || defender.hp != 5000 {
				t.Fatalf("second message = %#v, want %#v with the defender untouched", result.Messages[1], want)
			}
		})
	}
}

func TestSkillDamageFeedbackRecipientAndBlockedTarget(t *testing.T) {
	pdam := formulas.PhysicalSkillInput{
		AttackPower: 100, SkillPower: 50, Defence: 60,
		RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, ElementalMul: 1,
	}
	player := &skillTarget{fakeActor: fakeActor{objectID: 1}, isPlayer: true}
	for _, tc := range []struct {
		name     string
		caster   Actor
		targetID int32
		invul    bool
		para     bool
		want     []Damage
	}{
		{
			"pet", &summonSkillCaster{skillTarget: &skillTarget{}, owner: 9, pet: true}, 2, false, false,
			[]Damage{{RecipientID: 9, Source: DamageByPet}},
		},
		{
			"servitor", &summonSkillCaster{skillTarget: &skillTarget{}, owner: 9}, 2, false, false,
			[]Damage{{RecipientID: 9, Source: DamageByServitor}},
		},
		{"summon against its owner", &summonSkillCaster{skillTarget: &skillTarget{}, owner: 9, pet: true}, 9, false, false, nil},
		{"ownerless summon", &summonSkillCaster{skillTarget: &skillTarget{}}, 2, false, false, nil},
		{"npc", &skillTarget{}, 2, false, false, nil},
		{"invulnerable", player, 2, true, false, []Damage{{RecipientID: 1, Blocked: true}}},
		{"petrified", player, 2, true, true, []Damage{{RecipientID: 1, Blocked: true, Petrified: true}}},
		{"paralyzed only", player, 2, false, true, []Damage{{RecipientID: 1}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := &guardedSkillTarget{skillTarget: &skillTarget{
				fakeActor: fakeActor{objectID: tc.targetID}, hp: 5000,
				physicalInput: pdam, physicalOK: true,
			}, invul: tc.invul, paralyzed: tc.para}
			result, _ := NewDefaultRegistry().UseResult(Cast{
				Caster: tc.caster.(Creature),
				Skill:  modelskill.Definition{SkillType: "PDAM"}, Targets: []Actor{target},
			})
			got := damageMessages(t, result.Messages)
			for i := range tc.want {
				tc.want[i].Amount = int32(5000 - target.hp)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("damage messages = %#v, want %#v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("damage message %d = %#v, want %#v", i, got[i], tc.want[i])
				}
			}
		})
	}
}
