package summon

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect/effecttest"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ---- from effects_test.go ----
func TestSummonMaxBuffCountIncludesTemplateDivineInspiration(t *testing.T) {
	servitor := mustServitor(t, ServitorConfig{
		ObjectID: 1,
		Skills:   map[int]int{int(modelskill.DivineInspirationSkillID): 3},
	})

	if got := servitor.MaxBuffCount(); got != 23 {
		t.Fatalf("MaxBuffCount() = %d, want 23", got)
	}
}

func TestSummonMaxBuffCountUsesConfiguredBase(t *testing.T) {
	servitor := mustServitor(t, ServitorConfig{ObjectID: 1, MaxBuffsAmount: 2})

	if got := servitor.MaxBuffCount(); got != 2 {
		t.Fatalf("MaxBuffCount() = %d, want 2", got)
	}
}

func TestSummonEffectListHoldsAppliedEffects(t *testing.T) {
	servitor := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 40, Stats: CombatStats{MaxHP: 500, MaxMP: 200}, Roll: zeroSummonRoll})

	if servitor.MaxBuffCount() != baseBuffSlots {
		t.Fatalf("MaxBuffCount() = %d, want %d", servitor.MaxBuffCount(), baseBuffSlots)
	}

	list := servitor.EffectList()
	if list == nil {
		t.Fatal("EffectList() = nil, want a live list wired at construction")
	}

	e, err := effect.New(effect.Skill{ID: 2280, Level: 1, SkillType: "BUFF"}, modelskill.EffectTemplate{Name: "BlockBuff", Count: 1, Time: 60})
	if err != nil {
		t.Fatalf("effect.New() error = %v", err)
	}
	e.Effector = servitor
	e.Effected = servitor
	list.Add(e)

	if got := len(list.All()); got != 1 {
		t.Fatalf("EffectList().All() len = %d, want 1", got)
	}
}

func TestSummonDenyAIActionHonorsCrowdControlEffects(t *testing.T) {
	for _, tt := range []struct {
		name string
		flag effect.Flag
	}{
		{"stun", effect.FlagStunned},
		{"immobile until attacked", effect.FlagMeditating},
		{"sleep", effect.FlagSleep},
		{"paralyze", effect.FlagParalyzed},
		{"fear", effect.FlagFear},
	} {
		t.Run(tt.name, func(t *testing.T) {
			summon := mustServitor(t, ServitorConfig{ObjectID: 1})
			summon.EffectList().Add(&effect.Effect{Flag: tt.flag})

			if !summon.DenyAIAction() {
				t.Fatal("DenyAIAction() = false while crowd controlled")
			}
		})
	}
}

func TestSummonDenyAIActionHonorsTransientControlStates(t *testing.T) {
	summon := mustServitor(t, ServitorConfig{ObjectID: 1})

	if !summon.SetParalyzed(true) || !summon.DenyAIAction() {
		t.Fatal("paralyzed summon must deny AI actions")
	}
	if !summon.SetParalyzed(false) || summon.DenyAIAction() {
		t.Fatal("clearing paralysis must allow AI actions")
	}
	if !summon.SetTeleporting(true) || !summon.DenyAIAction() {
		t.Fatal("teleporting summon must deny AI actions")
	}
}

func TestSummonMovementDisabledExcludesFear(t *testing.T) {
	summon := mustServitor(t, ServitorConfig{ObjectID: 1})
	if summon.MovementDisabled() {
		t.Fatal("MovementDisabled() = true on a fresh summon")
	}

	summon.EffectList().Add(&effect.Effect{Flag: effect.FlagRooted})
	if !summon.MovementDisabled() {
		t.Fatal("MovementDisabled() = false while rooted")
	}

	feared := mustServitor(t, ServitorConfig{ObjectID: 2})
	feared.EffectList().Add(&effect.Effect{Flag: effect.FlagFear})
	if feared.MovementDisabled() {
		t.Fatal("MovementDisabled() = true while afraid; fear does not disable movement")
	}
	if !feared.DenyAIAction() {
		t.Fatal("DenyAIAction() = false while afraid; fear still blocks AI")
	}

	meditating := mustServitor(t, ServitorConfig{ObjectID: 3})
	meditating.EffectList().Add(&effect.Effect{Flag: effect.FlagMeditating})
	if !meditating.MovementDisabled() {
		t.Fatal("MovementDisabled() = false while immobile-until-attacked")
	}
}

// TestSummonAttackDisabledMatchesReferenceTerms pins the summon attack gate
// to the reference's creature union (Creature.isAttackingDisabled; a summon
// never flies), distinct from DenyAIAction: teleporting denies AI actions
// and movement but does not disable attacking.
func TestSummonAttackDisabledMatchesReferenceTerms(t *testing.T) {
	flags := []struct {
		name     string
		flag     effect.Flag
		disables bool
	}{
		{"stunned", effect.FlagStunned, true},
		{"meditating", effect.FlagMeditating, true},
		{"sleeping", effect.FlagSleep, true},
		{"paralyzed", effect.FlagParalyzed, true},
		{"afraid", effect.FlagFear, true},
		{"rooted", effect.FlagRooted, false},
	}
	for i, tt := range flags {
		t.Run(tt.name, func(t *testing.T) {
			summon := mustServitor(t, ServitorConfig{ObjectID: int32(i + 1)})
			if summon.AttackDisabled() {
				t.Fatal("AttackDisabled() = true on a fresh summon")
			}
			summon.EffectList().Add(&effect.Effect{Flag: tt.flag})
			if got := summon.AttackDisabled(); got != tt.disables {
				t.Fatalf("AttackDisabled() = %v while %s, want %v", got, tt.name, tt.disables)
			}
		})
	}

	t.Run("transient paralysis", func(t *testing.T) {
		summon := mustServitor(t, ServitorConfig{ObjectID: 10})
		summon.SetParalyzed(true)
		if !summon.AttackDisabled() {
			t.Fatal("AttackDisabled() = false while paralyzed")
		}
	})

	t.Run("teleporting", func(t *testing.T) {
		summon := mustServitor(t, ServitorConfig{ObjectID: 11})
		summon.SetTeleporting(true)
		if !summon.MovementDisabled() {
			t.Fatal("MovementDisabled() = false while teleporting")
		}
		if !summon.DenyAIAction() {
			t.Fatal("DenyAIAction() = false while teleporting")
		}
		if summon.AttackDisabled() {
			t.Fatal("AttackDisabled() = true while only teleporting")
		}
	})
}

// TestSummonImmobilizedRecordsFollowModeOnEveryCall checks that every set of
// the movement lock records the follow mode and every clear restores it, even
// when the flag does not change, and that a clear with no set before it
// restores following.
func TestSummonImmobilizedRecordsFollowModeOnEveryCall(t *testing.T) {
	s := mustServitor(t, ServitorConfig{ObjectID: 1})
	if !s.SetImmobilized(true) {
		t.Fatal("first SetImmobilized(true) = false, want the flag changed")
	}
	if s.FollowActive() {
		t.Fatal("follow mode after the first lock = on, want off")
	}
	if s.SetImmobilized(true) {
		t.Fatal("second SetImmobilized(true) = true, want unchanged")
	}
	if !s.SetImmobilized(false) {
		t.Fatal("first SetImmobilized(false) = false, want the flag changed")
	}
	if s.FollowActive() {
		t.Fatal("follow mode after the clear = on, want off as the second lock recorded it")
	}
	if s.SetImmobilized(false) {
		t.Fatal("second SetImmobilized(false) = true, want unchanged")
	}
	if s.FollowActive() {
		t.Fatal("follow mode after the second clear = on, want off")
	}

	fresh := mustServitor(t, ServitorConfig{ObjectID: 2})
	fresh.setFollowStatus(false)
	fresh.SetImmobilized(false)
	if !fresh.FollowActive() {
		t.Fatal("a clear with no lock before it left follow off, want following restored")
	}
}

func TestSummonImmobilizedIsIndependentOfRooted(t *testing.T) {
	summon := mustServitor(t, ServitorConfig{ObjectID: 1})

	if !summon.SetImmobilized(true) || !summon.MovementDisabled() {
		t.Fatal("SetImmobilized(true) must set the flag and disable movement")
	}
	if summon.EffectList().IsAffected(effect.FlagRooted) {
		t.Fatal("SetImmobilized must not set FlagRooted; the two are distinct states")
	}
	if !summon.SetImmobilized(false) || summon.MovementDisabled() {
		t.Fatal("SetImmobilized(false) must clear the flag and re-enable movement")
	}

	summon.EffectList().Add(&effect.Effect{Flag: effect.FlagRooted})
	if !summon.MovementDisabled() {
		t.Fatal("MovementDisabled() = false while rooted")
	}
	if summon.Immobilized() {
		t.Fatal("Immobilized() must not report true from the unrelated FlagRooted effect")
	}
}

// fakePlayerEffector is a minimal cast-effector satisfying effect.Actor,
// objectIDTarget, and playerTarget surfaces immobilizePetBuffStart checks
// (isPlayer(e.Effector), e.Effector.(objectIDTarget)).
type fakePlayerEffector struct {
	world.Presence
	effecttest.Actor
	id int32
}

func (p *fakePlayerEffector) ObjectID() int32 { return p.id }
func (p *fakePlayerEffector) Dead() bool      { return false }
func (p *fakePlayerEffector) IsPlayer() bool  { return true }

// TestSummonImobilePetBuffHookAppliesAndClearsImmobilized is the regression
// test for #2198: ImobilePetBuff's start hook type-asserted e.Effected
// against an interface summon.Actor never implemented, so the assertion two
// lines after the ownership check always failed and the buff was a silent
// no-op. This drives the real registered hook (immobilizePetBuffStart/Exit,
// core.go's "ImobilePetBuff" -> TypeImmobilizePetBuff wiring) through
// effect.New and EffectList().Add/Remove, rather than calling
// SetImmobilized directly, so it would fail again if summon.Actor ever
// stopped satisfying immobilizeTarget/summonOwnerTarget.
func TestSummonImobilePetBuffHookAppliesAndClearsImmobilized(t *testing.T) {
	owner := &fakeSummonOwner{id: 42}
	summon := mustServitor(t, ServitorConfig{ObjectID: 1, Owner: owner})

	newImobilePetBuff := func() *effect.Effect {
		e, err := effect.New(effect.Skill{ID: 2181, Level: 1, SkillType: "BUFF"}, modelskill.EffectTemplate{Name: "ImobilePetBuff", Count: 1, Time: 30})
		if err != nil {
			t.Fatalf("effect.New(ImobilePetBuff) error = %v", err)
		}
		e.Effected = summon
		return e
	}

	t.Run("owner cast sets and clears the flag", func(t *testing.T) {
		e := newImobilePetBuff()
		e.Effector = &fakePlayerEffector{id: owner.ObjectID()}

		summon.EffectList().Add(e)
		if !summon.Immobilized() || !summon.MovementDisabled() {
			t.Fatal("owned ImobilePetBuff must set Immobilized() and disable movement")
		}

		summon.EffectList().Remove(e)
		if summon.Immobilized() || summon.MovementDisabled() {
			t.Fatal("removing ImobilePetBuff must clear Immobilized() and re-enable movement")
		}
	})

	t.Run("non-owner cast is rejected", func(t *testing.T) {
		e := newImobilePetBuff()
		e.Effector = &fakePlayerEffector{id: owner.ObjectID() + 1}

		summon.EffectList().Add(e)
		if summon.Immobilized() {
			t.Fatal("a non-owner's ImobilePetBuff must not set Immobilized()")
		}
	})
}

// TestSummonOutOfControlHonorsBetrayedFlag is the regression test for the
// review finding that OutOfControl only read a.disabled, so a betrayed
// summon kept accepting owner commands instead of refusing them with
// PET_REFUSING_ORDER, matching Summon.isOutOfControl (Summon.java:296-298):
// super.isOutOfControl() || isBetrayed().
func TestSummonOutOfControlHonorsBetrayedFlag(t *testing.T) {
	summon := mustServitor(t, ServitorConfig{ObjectID: 1})

	if summon.OutOfControl() {
		t.Fatal("OutOfControl() = true before any betray effect, want false")
	}

	summon.EffectList().Add(&effect.Effect{Flag: effect.FlagBetrayed})

	if !summon.OutOfControl() {
		t.Fatal("OutOfControl() = false while betrayed, want true")
	}

	result := summon.ApplyCommand(CommandContext{Command: CommandStop})
	if result.Outcome != OutcomeRefusedOutOfControl {
		t.Fatalf("ApplyCommand(Stop) outcome = %v while betrayed, want OutcomeRefusedOutOfControl", result.Outcome)
	}
}

func TestPetEffectListHoldsAppliedEffects(t *testing.T) {
	pet := mustPet(t, PetConfig{ObjectID: 1, Level: 40, Stats: CombatStats{MaxHP: 500, MaxMP: 200}, Roll: zeroSummonRoll})

	if pet.EffectList() == nil {
		t.Fatal("EffectList() = nil, want a live list wired at construction")
	}
	if pet.MaxBuffCount() != baseBuffSlots {
		t.Fatalf("MaxBuffCount() = %d, want %d", pet.MaxBuffCount(), baseBuffSlots)
	}
}

func (*fakePlayerEffector) Kind() actor.Kind { return actor.KindPlayer }
