package skill

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// continuousFake satisfies every optional surface the continuous handler
// probes: a caster/target carrying an effect list, with landing-rate and
// reflect sources wired to a guaranteed-success roll by default.
type continuousFake struct {
	id                int32
	dead, invul       bool
	playable          bool
	attackableFlag    bool
	cursed            bool
	bss               bool
	list              *effect.List
	successOK         bool
	reflectOK         bool
	successInput      formulas.SkillSuccessInput
	skillReflectInput formulas.SkillReflectInput

	// recordSuccessInput, when set, is called with every SkillSuccessInput
	// invocation's raw arguments, letting tests assert on the resolved
	// caster/shield state without duplicating checkSkillSuccess's logic.
	recordSuccessInput func(caster any, def modelskill.Definition, bss bool, shield formulas.ShieldDefense)

	// aggression-event recording: which optional surface fired, and with
	// what arguments.
	aggressionSource  any
	aggressionPower   int
	currentTarget     any
	setTargetCalls    []any
	attackTargetCalls []any
}

func newContinuousFake(id int32) *continuousFake {
	return &continuousFake{
		id:           id,
		list:         effect.NewList(nil),
		successOK:    true,
		successInput: formulas.SkillSuccessInput{IgnoreResists: true, BaseChance: 100},
		reflectOK:    true,
	}
}

func (f *continuousFake) ObjectID() int32                { return f.id }
func (f *continuousFake) Dead() bool                     { return f.dead }
func (f *continuousFake) Invul() bool                    { return f.invul }
func (f *continuousFake) Playable() bool                 { return f.playable }
func (f *continuousFake) Attackable() bool               { return f.attackableFlag }
func (f *continuousFake) CursedWeaponEquipped() bool     { return f.cursed }
func (f *continuousFake) EffectList() *effect.List       { return f.list }
func (f *continuousFake) BlessedSpiritshotCharged() bool { return f.bss }

func (f *continuousFake) SkillSuccessInput(caster any, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	if f.recordSuccessInput != nil {
		f.recordSuccessInput(caster, def, bss, shield)
	}
	return f.successInput, f.successOK
}

func (f *continuousFake) SkillReflectInput(modelskill.Definition) formulas.SkillReflectInput {
	return f.skillReflectInput
}

func (f *continuousFake) NotifyAggression(source any, power int) {
	f.aggressionSource = source
	f.aggressionPower = power
}

func (f *continuousFake) CurrentTarget() any { return f.currentTarget }

func (f *continuousFake) SetTarget(target any) {
	f.setTargetCalls = append(f.setTargetCalls, target)
}

func (f *continuousFake) AttackTarget(target any) {
	f.attackTargetCalls = append(f.attackTargetCalls, target)
}

// addContinuousEffect seeds target's list with one effect of the given effect
// type, used to pre-arm BLOCK_BUFF / BLOCK_BUFF immunity.
func addContinuousEffect(t *testing.T, target *continuousFake, effectType string) {
	t.Helper()
	e, err := effect.New(effect.Skill{}, modelskill.EffectTemplate{Name: "Buff", Time: 600, EffectType: effectType})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = target
	target.list.Add(e)
}

func buffEffect() []modelskill.EffectTemplate {
	return []modelskill.EffectTemplate{{Name: "Buff", Time: 600}}
}

func TestContinuousBuffLandsOnCleanTarget(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newContinuousFake(2)

	registry.Use(Cast{
		Caster:  newContinuousFake(1),
		Skill:   modelskill.Definition{SkillType: "BUFF", Effects: buffEffect()},
		Targets: []any{target},
	})

	if got := len(target.list.All()); got != 1 {
		t.Fatalf("effect list = %d, want 1 landed buff", got)
	}
}

func TestContinuousBuffSkipsBlockedBuffImmuneTarget(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newContinuousFake(2)
	addContinuousEffect(t, target, "BLOCK_BUFF")

	registry.Use(Cast{
		Caster:  newContinuousFake(1),
		Skill:   modelskill.Definition{SkillType: "BUFF", Effects: buffEffect()},
		Targets: []any{target},
	})

	if got := len(target.list.All()); got != 1 {
		t.Fatalf("effect list = %d after BUFF, want 1 (only the pre-existing BLOCK_BUFF)", got)
	}
}

// A real BlockBuff marker loaded from the datapack (<effect name="BlockBuff">
// with no effectType attribute) must still suppress incoming BUFF effects:
// its classification is resolved from the runtime kind, not the attribute.
func TestContinuousBuffSkipsBlockedBuffFromRealMarkerEffect(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newContinuousFake(2)

	blocker, err := effect.New(effect.Skill{}, modelskill.EffectTemplate{Name: "BlockBuff", Time: 600})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	blocker.Effected = target
	target.list.Add(blocker)

	registry.Use(Cast{
		Caster:  newContinuousFake(1),
		Skill:   modelskill.Definition{SkillType: "BUFF", Effects: buffEffect()},
		Targets: []any{target},
	})

	if got := len(target.list.All()); got != 1 {
		t.Fatalf("effect list = %d after BUFF, want 1 (only the pre-existing BlockBuff marker)", got)
	}
}

// A real BlockDebuff marker loaded from the datapack must still suppress an
// incoming offensive debuff for the same reason as the BlockBuff case.
func TestContinuousDebuffSkipsBlockedDebuffFromRealMarkerEffect(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newContinuousFake(2)

	blocker, err := effect.New(effect.Skill{}, modelskill.EffectTemplate{Name: "BlockDebuff", Time: 600})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	blocker.Effected = target
	target.list.Add(blocker)

	registry.Use(Cast{
		Caster:  newContinuousFake(1),
		Skill:   modelskill.Definition{SkillType: "DEBUFF", Offensive: true, Debuff: true, Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 600}}},
		Targets: []any{target},
	})

	if got := len(target.list.All()); got != 1 {
		t.Fatalf("effect list = %d after DEBUFF, want 1 (only the pre-existing BlockDebuff marker)", got)
	}
}

func TestContinuousBuffSkipsCursedOther(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newContinuousFake(2)
	target.cursed = true
	caster := newContinuousFake(1)

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "BUFF", Effects: buffEffect()},
		Targets: []any{target},
	})

	if got := len(target.list.All()); got != 0 {
		t.Fatalf("effect list = %d, want 0 (cursed target cannot be buffed by another)", got)
	}
}

func TestContinuousBuffLandsOnCursedSelf(t *testing.T) {
	registry := NewDefaultRegistry()
	self := newContinuousFake(1)
	self.cursed = true

	registry.Use(Cast{
		Caster:  self,
		Skill:   modelskill.Definition{SkillType: "BUFF", Effects: buffEffect()},
		Targets: []any{self},
	})

	if got := len(self.list.All()); got != 1 {
		t.Fatalf("effect list = %d, want 1 (self-buff is exempt from the cursed gate)", got)
	}
}

func TestContinuousHOTSkippedWhenCasterInvulnerable(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newContinuousFake(1)
	caster.invul = true
	target := newContinuousFake(2)

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "HOT", Effects: buffEffect()},
		Targets: []any{target},
	})

	if got := len(target.list.All()); got != 0 {
		t.Fatalf("effect list = %d, want 0 (HOT cannot tick while caster is invul)", got)
	}
}

func TestContinuousFearImmunePlayableSkillIDsSkipped(t *testing.T) {
	for _, id := range []modelskill.ID{98, 1272, 1381} {
		registry := NewDefaultRegistry()
		target := newContinuousFake(2)
		target.playable = true

		registry.Use(Cast{
			Caster:  newContinuousFake(1),
			Skill:   modelskill.Definition{ID: id, SkillType: "FEAR", Effects: buffEffect()},
			Targets: []any{target},
		})

		if got := len(target.list.All()); got != 0 {
			t.Fatalf("skill %d: effect list = %d, want 0 (FEAR immune list skips playables)", id, got)
		}
	}
}

func TestContinuousFearLandsOnPlayableForOtherSkill(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newContinuousFake(2)
	target.playable = true

	registry.Use(Cast{
		Caster:  newContinuousFake(1),
		Skill:   modelskill.Definition{ID: 9999, SkillType: "FEAR", Effects: buffEffect()},
		Targets: []any{target},
	})

	if got := len(target.list.All()); got != 1 {
		t.Fatalf("effect list = %d, want 1 (FEAR not in immune list lands)", got)
	}
}

func TestContinuousOffensiveDebuffSkipsBlockedDebuffTarget(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newContinuousFake(2)
	addContinuousEffect(t, target, "BLOCK_DEBUFF")

	registry.Use(Cast{
		Caster:  newContinuousFake(1),
		Skill:   modelskill.Definition{SkillType: "DEBUFF", Offensive: true, Effects: buffEffect()},
		Targets: []any{target},
	})

	if got := len(target.list.All()); got != 1 {
		t.Fatalf("effect list = %d, want 1 (only the pre-existing BLOCK_DEBUFF)", got)
	}
}

func TestContinuousDebuffFailRollDoesNotLand(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newContinuousFake(2)
	target.successInput = formulas.SkillSuccessInput{IgnoreResists: true, BaseChance: 0}

	registry.Use(Cast{
		Caster:  newContinuousFake(1),
		Skill:   modelskill.Definition{SkillType: "DEBUFF", Debuff: true, Effects: buffEffect()},
		Targets: []any{target},
	})

	if got := len(target.list.All()); got != 0 {
		t.Fatalf("effect list = %d, want 0 (failed landing roll applies nothing)", got)
	}
}

func TestContinuousDebuffFailRollReportsAttackFailed(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newContinuousFake(2)
	target.successInput = formulas.SkillSuccessInput{IgnoreResists: true, BaseChance: 0}

	result, ok := registry.UseResult(Cast{
		Caster:  newContinuousFake(1),
		Skill:   modelskill.Definition{SkillType: "DEBUFF", Debuff: true, Effects: buffEffect()},
		Targets: []any{target},
	})
	if !ok {
		t.Fatal("UseResult() handled = false, want true for DEBUFF")
	}
	if result.AttackFailed != 1 {
		t.Fatalf("AttackFailed = %d, want 1", result.AttackFailed)
	}
}

func TestContinuousDebuffSuccessRollLands(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newContinuousFake(2)

	registry.Use(Cast{
		Caster:  newContinuousFake(1),
		Skill:   modelskill.Definition{SkillType: "DEBUFF", Debuff: true, Effects: buffEffect()},
		Targets: []any{target},
	})

	if got := len(target.list.All()); got != 1 {
		t.Fatalf("effect list = %d, want 1 (successful landing roll applies the debuff)", got)
	}
}

func TestContinuousDebuffNoLandingSourceDoesNotLand(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newContinuousFake(2)
	target.successOK = false

	registry.Use(Cast{
		Caster:  newContinuousFake(1),
		Skill:   modelskill.Definition{SkillType: "DEBUFF", Debuff: true, Effects: buffEffect()},
		Targets: []any{target},
	})

	if got := len(target.list.All()); got != 0 {
		t.Fatalf("effect list = %d, want 0 (no resolved landing-rate source => not applied)", got)
	}
}

func TestContinuousToggleDropsPriorSameSkillBeforeReapplying(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newContinuousFake(2)

	// Pre-existing effect from the same skill id the toggle refresh casts.
	stale, err := effect.New(effect.Skill{ID: 555}, modelskill.EffectTemplate{Name: "Buff", Time: 600})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	stale.Effected = target
	target.list.Add(stale)

	registry.Use(Cast{
		Caster: newContinuousFake(1),
		Skill: modelskill.Definition{
			ID: 555, SkillType: "BUFF", Activation: modelskill.ActivationToggle,
			Effects: buffEffect(),
		},
		Targets: []any{target},
	})

	if hasEffect(target.list, stale) {
		t.Error("stale same-skill toggle effect should be dropped before reapplying")
	}
	if got := len(target.list.All()); got != 1 {
		t.Fatalf("effect list = %d, want exactly 1 refreshed effect", got)
	}
}

func TestContinuousAppliesSelfEffectsOnCaster(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newContinuousFake(1)
	target := newContinuousFake(2)

	registry.Use(Cast{
		Caster: caster,
		Skill: modelskill.Definition{
			SkillType: "BUFF", ID: 3,
			Effects:     buffEffect(),
			SelfEffects: []modelskill.EffectTemplate{{Name: "Buff", Time: 600, Self: true}},
		},
		Targets: []any{target},
	})

	if got := len(target.list.All()); got != 1 {
		t.Fatalf("target effect list = %d, want 1 target effect", got)
	}
	if got := len(caster.list.All()); got != 1 {
		t.Fatalf("caster effect list = %d, want 1 self effect", got)
	}
}

func TestContinuousReflectsOffensiveBackToCaster(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newContinuousFake(1)
	target := newContinuousFake(2)
	target.skillReflectInput = formulas.SkillReflectInput{
		CanBeReflected: true, Magic: true, ReflectChance: 100,
	}
	target.successOK = true

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "DEBUFF", Offensive: true, Magic: true, Effects: buffEffect()},
		Targets: []any{target},
	})

	if got := len(target.list.All()); got != 0 {
		t.Fatalf("target effect list = %d, want 0 (reflected skill must not land on the target)", got)
	}
	if got := len(caster.list.All()); got != 1 {
		t.Fatalf("caster effect list = %d, want 1 (reflected debuff lands on the caster)", got)
	}
}

func TestContinuousRegistryHasAllHandledTypes(t *testing.T) {
	registry := NewDefaultRegistry()
	for _, typ := range []string{
		"BUFF", "DEBUFF", "DOT", "MDOT", "POISON", "BLEED",
		"HOT", "MPHOT", "FEAR", "CONT", "WEAKNESS", "REFLECT",
		"AGGDEBUFF", "FUSION",
	} {
		if _, ok := registry.Handler(typ); !ok {
			t.Errorf("continuous handler missing registered skill type %q", typ)
		}
	}
}

func TestContinuousAGGDEBUFFNotifiesAttackableTarget(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newContinuousFake(1)
	target := newContinuousFake(2)
	target.attackableFlag = true

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "AGGDEBUFF", Power: 42, Effects: buffEffect()},
		Targets: []any{target},
	})

	if got := len(target.list.All()); got != 1 {
		t.Fatalf("effect list = %d, want 1 (the debuff itself still lands)", got)
	}
	if target.aggressionSource != any(caster) {
		t.Fatalf("aggressionSource = %v, want caster", target.aggressionSource)
	}
	if target.aggressionPower != 42 {
		t.Fatalf("aggressionPower = %v, want 42", target.aggressionPower)
	}
	if len(target.setTargetCalls) != 0 || len(target.attackTargetCalls) != 0 {
		t.Fatal("an attackable target must not be retargeted through the playable branch")
	}
}

func TestContinuousAGGDEBUFFRetargetsPlayableNotAlreadyTargetingCaster(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newContinuousFake(1)
	other := newContinuousFake(3)
	target := newContinuousFake(2)
	target.playable = true
	target.currentTarget = other

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "AGGDEBUFF", Power: 10, Effects: buffEffect()},
		Targets: []any{target},
	})

	if len(target.setTargetCalls) != 1 || target.setTargetCalls[0] != any(caster) {
		t.Fatalf("setTargetCalls = %v, want exactly one call with caster", target.setTargetCalls)
	}
	if len(target.attackTargetCalls) != 0 {
		t.Fatal("a playable not already targeting the caster must be retargeted, not attacked")
	}
}

func TestContinuousAGGDEBUFFAttacksPlayableAlreadyTargetingCaster(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newContinuousFake(1)
	target := newContinuousFake(2)
	target.playable = true
	target.currentTarget = caster

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "AGGDEBUFF", Power: 10, Effects: buffEffect()},
		Targets: []any{target},
	})

	if len(target.attackTargetCalls) != 1 || target.attackTargetCalls[0] != any(caster) {
		t.Fatalf("attackTargetCalls = %v, want exactly one call with caster", target.attackTargetCalls)
	}
	if len(target.setTargetCalls) != 0 {
		t.Fatal("a playable already targeting the caster must be attacked, not retargeted")
	}
}

func TestContinuousDebuffUsesCasterBlessedSpiritshotCharge(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newContinuousFake(1)
	caster.bss = true
	target := newContinuousFake(2)
	var seenBss bool
	target.recordSuccessInput = func(_ any, _ modelskill.Definition, bss bool, _ formulas.ShieldDefense) {
		seenBss = bss
	}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "DEBUFF", Debuff: true, Effects: buffEffect()},
		Targets: []any{target},
	})

	if !seenBss {
		t.Fatal("continuous landing roll should have resolved the caster's blessed-spiritshot charge as true")
	}
}
