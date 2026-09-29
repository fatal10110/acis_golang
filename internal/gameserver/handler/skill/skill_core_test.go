package skill

import (
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect/effecttest"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ---- from actor_test.go ----
// The handlers in this package reach every capability they need by asserting
// a cast participant into one of the focused interfaces below. Those
// assertions fail silently by design — a target that doesn't implement the
// surface is skipped rather than rejected — so a real actor that stops
// satisfying one of them disables a skill path without failing any test that
// uses a double. These assertions pin the real production actors against the
// surfaces they are expected to reach, so that regression is a build failure.
//
// Every participant surface embeds Actor, so this also pins the claim Actor
// rests on: the actors that report Dead() all report ObjectID() too.
var (
	_ Actor = (*player.Character)(nil)
	_ Actor = (*npc.Hostile)(nil)
	_ Actor = (*summon.Actor)(nil)
	_ Actor = (*npc.EffectPoint)(nil)

	// Effect-carrying targets: the destination of any effect-applying,
	// effect-cancelling, or continuous (buff/debuff/over-time) skill.
	_ effect.Actor = (*player.Character)(nil)
	_ effect.Actor = (*npc.Hostile)(nil)
	_ effect.Actor = (*summon.Actor)(nil)
	_ effect.Actor = (*npc.EffectPoint)(nil)

	// Damage and resource targets: PDAM/CHARGEDAM, MDAM/DEATHLINK, BLOW and
	// MANADAM all reach HP and MP through Creature, and the player-only
	// resources and notifications through Player.
	_ Creature = (*player.Character)(nil)
	_ Creature = (*npc.Hostile)(nil)
	_ Creature = (*summon.Actor)(nil)
	_ Player   = (*player.Character)(nil)
	_ NPC      = (*npc.Hostile)(nil)

	// DRAIN rolls a player target's cast break ahead of its effects and
	// HP loss.
	_ earlyCastBreaker = (*player.Character)(nil)

	// Caster-side surfaces resolved from Cast.Caster. cancelTarget above
	// and these three share a Level() int requirement that *player.Character
	// could not meet until its persisted level field was renamed off of
	// Level to make room for the method (see player.Character.CharLevel).
	_ magicCaster   = (*player.Character)(nil)
	_ sowCaster     = (*player.Character)(nil)
	_ harvestCaster = (*player.Character)(nil)

	// Signet: the radius scan hands each found object to the tick as an
	// Actor, and an anti-summon signet narrows that to a dismissable summon.
	_ signetUnsummonable = (*summon.Actor)(nil)

	// Erase: the servitor surface disableErase reaches through, and the
	// owner-facing notification it fires once erased.

	// SummonFriend/SummonParty: the caster-side gate, the target-side gate,
	// the pending teleport-request/confirm-summon surface, the required-item
	// check, and the teleport itself.
	_ summonFriendCaster       = (*player.Character)(nil)
	_ summonFriendRequester    = (*player.Character)(nil)
	_ summonFriendItemConsumer = (*player.Character)(nil)
	_ summonFriendTraveler     = (*player.Character)(nil)
)

// fakeActor supplies the Actor surface every cast participant carries, so a
// test double only has to spell out the capability the case under test
// actually exercises. Prefer a real actor (see newDisablerHostile) when the
// case is about the behavior rather than about one narrow surface; a double
// that models death or identity itself declares its own Dead or ObjectID,
// which shadows the one embedded here.
type fakeActor struct {
	effecttest.Actor
	objectID int32
}

func (f fakeActor) ObjectID() int32 { return f.objectID }

func (fakeActor) Dead() bool { return false }

// ---- from apply_test.go ----
type effectLandingFake struct {
	neutralCreature
	world.Presence
	fakeActor
	list    *effect.List
	x, y, z int
	invul   bool
}

func (f *effectLandingFake) EffectList() *effect.List { return f.list }

func (f *effectLandingFake) Position() (int, int, int) { return f.x, f.y, f.z }

func (f *effectLandingFake) Invul() bool { return f.invul }

type damagePermissionFake struct {
	neutralCreature
	world.Presence
	fakeActor
	allow bool
}

func (f *damagePermissionFake) CanGiveDamage() bool { return f.allow }
func (*damagePermissionFake) Heading() int          { return 0 }

// combatantOnlyCaster is a combatant that is not a formula caster: the shape
// a new combatant kind has before it grows the formula surface.
type combatantOnlyCaster struct {
	world.Presence
	fakeActor
	allow bool
}

var _ attackable.Combatant = (*combatantOnlyCaster)(nil)

func (f *combatantOnlyCaster) CanGiveDamage() bool { return f.allow }

type positionedFakeActor struct {
	neutralCreature
	world.Presence
	fakeActor
	x, y, z int
}

func (f *positionedFakeActor) Position() (int, int, int) { return f.x, f.y, f.z }

type effectListOnlyFake struct {
	world.Presence
	fakeActor
	list *effect.List
}

func (f *effectListOnlyFake) EffectList() *effect.List { return f.list }

func (*effectLandingFake) EffectSuccessInput(_ creature.FormulaActor, _ modelskill.Definition, tmpl modelskill.EffectTemplate, _ bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	return formulas.SkillSuccessInput{BaseChance: tmpl.EffectPower, IgnoreResists: true, Shield: shield}, true
}

func TestApplyEffectsRollsEachConfiguredTemplate(t *testing.T) {
	target := &effectLandingFake{list: newTestList(nil)}
	templates := []modelskill.EffectTemplate{
		{Name: "Buff", Time: 60, EffectPower: 100, EffectPowerSet: true},
		{Name: "Buff", Time: 60, EffectPower: 0, EffectPowerSet: true},
	}
	applyEffects(nil, target, modelskill.Definition{}, templates)

	if got := len(target.list.All()); got != 1 {
		t.Fatalf("landed effects = %d, want 1", got)
	}
}

func TestApplyEffectsEffectRangeAtLanding(t *testing.T) {
	configured := []modelskill.EffectTemplate{{Name: "Buff", Time: 60, EffectPower: 100, EffectPowerSet: true}}
	tests := []struct {
		name      string
		caster    effect.Actor
		target    effect.Actor
		templates []modelskill.EffectTemplate
		want      int
	}{
		{
			name:      "outside range",
			caster:    &positionedFakeActor{fakeActor: fakeActor{objectID: 1}},
			target:    &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil), x: 101},
			templates: configured,
		},
		{
			name:      "at range",
			caster:    &positionedFakeActor{fakeActor: fakeActor{objectID: 1}},
			target:    &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil), x: 100},
			templates: configured,
		},
		{
			name:      "inside range",
			caster:    &positionedFakeActor{fakeActor: fakeActor{objectID: 1}},
			target:    &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil), x: 99},
			templates: configured,
			want:      1,
		},
		{
			name:      "self cast",
			caster:    &positionedFakeActor{fakeActor: fakeActor{objectID: 1}},
			target:    &effectLandingFake{fakeActor: fakeActor{objectID: 1}, list: newTestList(nil), x: 1000},
			templates: configured,
			want:      1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyEffects(tt.caster, tt.target, modelskill.Definition{EffectRange: 100}, tt.templates)
			if got := len(tt.target.EffectList().All()); got != tt.want {
				t.Fatalf("landed effects = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestApplyEffectsRejectsPerfectShieldBeforeTemplates(t *testing.T) {
	target := &effectLandingFake{list: newTestList(nil)}
	applyEffectsWithLanding(nil, target, modelskill.Definition{}, []modelskill.EffectTemplate{{Name: "Buff", Time: 60, EffectPower: 100, EffectPowerSet: true}}, formulas.ShieldPerfect, false)

	if got := len(target.list.All()); got != 0 {
		t.Fatalf("landed effects after perfect shield = %d, want 0", got)
	}
}

func TestApplyEffectsRefusesOffensiveAndDebuffOnInvulOrDeniedDamage(t *testing.T) {
	if _, ok := effect.Actor(&combatantOnlyCaster{}).(creature.FormulaActor); ok {
		t.Fatal("combatantOnlyCaster must not satisfy creature.FormulaActor")
	}
	landing := []modelskill.EffectTemplate{{Name: "Buff", Time: 60}}
	tests := []struct {
		name   string
		caster effect.Actor
		target *effectLandingFake
		def    modelskill.Definition
		want   int
	}{
		{
			name:   "offensive vs invul",
			caster: &positionedFakeActor{fakeActor: fakeActor{objectID: 1}},
			target: &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil), invul: true},
			def:    modelskill.Definition{Offensive: true},
		},
		{
			name:   "debuff vs invul",
			caster: &positionedFakeActor{fakeActor: fakeActor{objectID: 1}},
			target: &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil), invul: true},
			def:    modelskill.Definition{Debuff: true},
		},
		{
			name:   "buff vs invul still lands",
			caster: &positionedFakeActor{fakeActor: fakeActor{objectID: 1}},
			target: &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil), invul: true},
			want:   1,
		},
		{
			name:   "self offensive on invul still lands",
			caster: &positionedFakeActor{fakeActor: fakeActor{objectID: 1}},
			target: &effectLandingFake{fakeActor: fakeActor{objectID: 1}, list: newTestList(nil), invul: true},
			def:    modelskill.Definition{Offensive: true},
			want:   1,
		},
		{
			name:   "nil caster still refuses invul offensive",
			target: &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil), invul: true},
			def:    modelskill.Definition{Offensive: true},
		},
		{
			name:   "offensive when caster cannot give damage",
			caster: &damagePermissionFake{fakeActor: fakeActor{objectID: 1}, allow: false},
			target: &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil)},
			def:    modelskill.Definition{Offensive: true},
		},
		{
			name:   "buff when caster cannot give damage still lands",
			caster: &damagePermissionFake{fakeActor: fakeActor{objectID: 1}, allow: false},
			target: &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil)},
			want:   1,
		},
		{
			name:   "offensive when a non-formula combatant cannot give damage",
			caster: &combatantOnlyCaster{fakeActor: fakeActor{objectID: 1}, allow: false},
			target: &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil)},
			def:    modelskill.Definition{Offensive: true},
		},
		{
			name:   "offensive when a non-formula combatant may give damage",
			caster: &combatantOnlyCaster{fakeActor: fakeActor{objectID: 1}, allow: true},
			target: &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil)},
			def:    modelskill.Definition{Offensive: true},
			want:   1,
		},
		{
			name:   "offensive when caster may give damage",
			caster: &damagePermissionFake{fakeActor: fakeActor{objectID: 1}, allow: true},
			target: &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil)},
			def:    modelskill.Definition{Offensive: true},
			want:   1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyEffects(tt.caster, tt.target, tt.def, landing)
			if got := len(tt.target.list.All()); got != tt.want {
				t.Fatalf("landed effects = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestApplyEffectsRejectsConfiguredTemplateWithoutLandingInput(t *testing.T) {
	target := &effectListOnlyFake{list: newTestList(nil)}
	applyEffects(nil, target, modelskill.Definition{}, []modelskill.EffectTemplate{
		{Name: "Buff", Time: 60, EffectPower: 100, EffectPowerSet: true},
		{Name: "Buff", Time: 60},
	})

	if got := len(target.list.All()); got != 1 {
		t.Fatalf("landed effects = %d, want 1 unconfigured template", got)
	}
}

func TestActiveEffectFindsAMatchingLiveInstance(t *testing.T) {
	target := newCancelFakeActor(10)
	addBuff(t, target, modelskill.EffectTemplate{Name: "Buff", Time: 60}, effect.Skill{ID: 288})

	if !ActiveEffect(target, 288) {
		t.Fatal("ActiveEffect() = false, want true for a live instance of skill 288")
	}
	if ActiveEffect(target, 99) {
		t.Fatal("ActiveEffect() = true, want false for a skill id with no live instance")
	}
}

// placedFakeActor is a fakeActor with a world placement, for the call sites
// that pass a bare actor rather than a positioned double.
type placedFakeActor struct {
	fakeActor
	world.Presence
}

func TestActiveEffectOnATargetWithNoEffectListIsFalse(t *testing.T) {
	if ActiveEffect(&placedFakeActor{}, 288) {
		t.Fatal("ActiveEffect() = true, want false for a target with no effect list")
	}
}

func TestStopEffectRemovesTheMatchingLiveInstance(t *testing.T) {
	target := newCancelFakeActor(10)
	e := addBuff(t, target, modelskill.EffectTemplate{Name: "Buff", Time: 60}, effect.Skill{ID: 288})
	addBuff(t, target, modelskill.EffectTemplate{Name: "Buff", Time: 60}, effect.Skill{ID: 4})

	StopEffect(target, 288)

	if hasEffect(target.list, e) {
		t.Fatal("skill 288's effect is still active after StopEffect")
	}
	if !ActiveEffect(target, 4) {
		t.Fatal("StopEffect removed an unrelated skill's active effect")
	}
}

func TestStopEffectOnATargetWithNoEffectListIsANoop(t *testing.T) {
	StopEffect(&placedFakeActor{}, 288)
}

// ---- from cancel_test.go ----
type cancelFakeActor struct {
	neutralCreature
	world.Presence
	fakeActor
	dead  bool
	level int
	list  *effect.List
}

func newCancelFakeActor(level int) *cancelFakeActor {
	return &cancelFakeActor{level: level, list: newTestList(nil)}
}

func (a *cancelFakeActor) Dead() bool               { return a.dead }
func (a *cancelFakeActor) Level() int               { return a.level }
func (a *cancelFakeActor) EffectList() *effect.List { return a.list }

func addBuff(t *testing.T, actor *cancelFakeActor, tmpl modelskill.EffectTemplate, meta effect.Skill) *effect.Effect {
	t.Helper()
	e, err := effect.New(meta, tmpl)
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = actor
	actor.list.Add(e)
	return e
}

func hasEffect(list *effect.List, e *effect.Effect) bool {
	for _, cur := range list.All() {
		if cur == e {
			return true
		}
	}
	return false
}

func TestCancelNeverStripsToggleOrDebuffEffects(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newCancelFakeActor(40)

	toggle := addBuff(t, target, modelskill.EffectTemplate{Name: "Buff", Time: 600}, effect.Skill{Toggle: true})
	debuff := addBuff(t, target, modelskill.EffectTemplate{Name: "Debuff", Time: 600}, effect.Skill{Debuff: true})

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "CANCEL", Power: 50, MaxNegatedEffects: 10, MagicLevel: 40},
		Targets: []Actor{target},
	})

	if !hasEffect(target.list, toggle) {
		t.Error("a toggle effect must never be stripped by CANCEL")
	}
	if !hasEffect(target.list, debuff) {
		t.Error("a debuff effect must never be stripped by CANCEL")
	}
}

func TestCancelNeverStripsNonCancellableEffectType(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newCancelFakeActor(40)

	blessing := addBuff(t, target, modelskill.EffectTemplate{Name: "Buff", Time: 600, EffectType: "noblesse_blessing"}, effect.Skill{})

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "CANCEL", Power: 50, MaxNegatedEffects: 10, MagicLevel: 40},
		Targets: []Actor{target},
	})

	if !hasEffect(target.list, blessing) {
		t.Error("noblesse blessing must never be stripped by CANCEL")
	}
}

// A real ProtectionBlessing marker loaded from the datapack carries no
// effectType attribute, so its cancel-exemption must be resolved from the
// runtime kind the same way the attribute-tagged blessing above is.
func TestCancelNeverStripsProtectionBlessingMarkerEffect(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newCancelFakeActor(40)

	protection := addBuff(t, target, modelskill.EffectTemplate{Name: "ProtectionBlessing", Time: 600}, effect.Skill{})

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "CANCEL", Power: 50, MaxNegatedEffects: 10, MagicLevel: 40},
		Targets: []Actor{target},
	})

	if !hasEffect(target.list, protection) {
		t.Error("protection blessing must never be stripped by CANCEL")
	}
}

func TestMageBaneOnlyConsidersMatchingStackTypes(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newCancelFakeActor(40)

	unrelated := addBuff(t, target, modelskill.EffectTemplate{Name: "Buff", Time: 600, StackType: "speed_up"}, effect.Skill{})

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "MAGE_BANE", Power: 50, MaxNegatedEffects: 10, MagicLevel: 40},
		Targets: []Actor{target},
	})

	if !hasEffect(target.list, unrelated) {
		t.Error("MAGE_BANE must never strip a stack type it doesn't cover")
	}
}

func TestCancelRefreshesCasterSelfEffect(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newCancelFakeActor(40)

	// A pre-existing self effect from the same skill should be dropped
	// before the fresh copy is applied, so re-casting doesn't stack it.
	stale := addBuff(t, caster, modelskill.EffectTemplate{Name: "Buff", Time: 600, Self: true}, effect.Skill{ID: 99})

	registry.Use(Cast{
		Caster: caster,
		Skill: modelskill.Definition{
			SkillType:   "CANCEL",
			ID:          99,
			SelfEffects: []modelskill.EffectTemplate{{Name: "Buff", Time: 600, Self: true}},
		},
	})

	if hasEffect(caster.list, stale) {
		t.Error("stale self effect should have been dropped before reapplying")
	}
	if len(caster.list.All()) != 1 {
		t.Fatalf("caster effect list = %d entries, want exactly 1 refreshed self effect", len(caster.list.All()))
	}
}

// ---- from continuous_fixtures_test.go ----
// reflect sources wired to a guaranteed-success roll by default.
type continuousFake struct {
	neutralCreature
	world.Presence
	id                int32
	dead, invul       bool
	denyDamage        bool
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
	currentTarget     world.Tracked
	setTargetCalls    []world.Tracked
	attackTargetCalls []world.Tracked
}

func newContinuousFake(id int32) *continuousFake {
	return &continuousFake{
		id:           id,
		list:         newTestList(nil),
		successOK:    true,
		successInput: formulas.SkillSuccessInput{IgnoreResists: true, BaseChance: 100},
		reflectOK:    true,
	}
}

func (f *continuousFake) ObjectID() int32                { return f.id }
func (*continuousFake) Kind() actor.Kind                 { return actor.KindNPC }
func (*continuousFake) CharacterName() string            { return "Target" }
func (f *continuousFake) Dead() bool                     { return f.dead }
func (f *continuousFake) Invul() bool                    { return f.invul }
func (f *continuousFake) CanGiveDamage() bool            { return !f.denyDamage }
func (f *continuousFake) Playable() bool                 { return f.playable }
func (f *continuousFake) Attackable() bool               { return f.attackableFlag }
func (f *continuousFake) CursedWeaponEquipped() bool     { return f.cursed }
func (f *continuousFake) EffectList() *effect.List       { return f.list }
func (f *continuousFake) BlessedSpiritshotCharged() bool { return f.bss }

func (f *continuousFake) SkillSuccessInput(caster creature.FormulaActor, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	if f.recordSuccessInput != nil {
		f.recordSuccessInput(caster, def, bss, shield)
	}
	return f.successInput, f.successOK
}

func (f *continuousFake) SkillReflectInput(modelskill.Definition) formulas.SkillReflectInput {
	return f.skillReflectInput
}

func (f *continuousFake) NotifyAggression(source attackable.Combatant, power int) {
	f.aggressionSource = source
	f.aggressionPower = power
}

func (f *continuousFake) CurrentTarget() world.Tracked { return f.currentTarget }

func (f *continuousFake) SetTarget(target world.Tracked) {
	f.setTargetCalls = append(f.setTargetCalls, target)
}

func (f *continuousFake) AttackTarget(target world.Tracked) {
	f.attackTargetCalls = append(f.attackTargetCalls, target)
}

func buffEffect() []modelskill.EffectTemplate {
	return []modelskill.EffectTemplate{{Name: "Buff", Time: 600}}
}

type continuousDefinitions map[modelskill.Ref]modelskill.Definition

func (d continuousDefinitions) Definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	def, ok := d[ref]
	return def, ok
}

func (d continuousDefinitions) MaxLevel(id modelskill.ID) int {
	max := 0
	for ref := range d {
		if ref.ID == id && ref.Level > max {
			max = ref.Level
		}
	}
	return max
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

func TestContinuousDebuffSkipsInvulnerableTargetWithoutAttackFailed(t *testing.T) {
	caster := newContinuousFake(1)
	target := newContinuousFake(2)
	target.invul = true
	result := continuousHandler{}.UseResult(Cast{
		Caster: caster,
		Skill: modelskill.Definition{
			SkillType: "DEBUFF", Debuff: true, Offensive: true,
			IgnoreResists: true, BaseLandRate: 100,
			Effects: buffEffect(),
		},
		Targets: []Actor{target},
	})
	if result.AttackFailed != 0 {
		t.Fatalf("AttackFailed = %d, want 0 (land roll succeeded; apply refused)", result.AttackFailed)
	}
	if got := len(target.list.All()); got != 0 {
		t.Fatalf("invulnerable target received %d effects, want 0", got)
	}
}

func TestContinuousBuffLandsOnInvulnerableTarget(t *testing.T) {
	caster := newContinuousFake(1)
	target := newContinuousFake(2)
	target.invul = true
	continuousHandler{}.UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "BUFF", Effects: buffEffect()},
		Targets: []Actor{target},
	})
	if got := len(target.list.All()); got != 1 {
		t.Fatalf("buff on invulnerable target landed %d effects, want 1", got)
	}
}

func TestContinuousDebuffSkipsWhenCasterCannotGiveDamage(t *testing.T) {
	caster := newContinuousFake(1)
	caster.denyDamage = true
	target := newContinuousFake(2)
	result := continuousHandler{}.UseResult(Cast{
		Caster: caster,
		Skill: modelskill.Definition{
			SkillType: "DEBUFF", Debuff: true,
			IgnoreResists: true, BaseLandRate: 100,
			Effects: buffEffect(),
		},
		Targets: []Actor{target},
	})
	if result.AttackFailed != 0 {
		t.Fatalf("AttackFailed = %d, want 0 (land roll succeeded; apply refused)", result.AttackFailed)
	}
	if got := len(target.list.All()); got != 0 {
		t.Fatalf("denied-damage caster landed %d effects, want 0", got)
	}
}

// TestContinuousRollsLethalAfterLandingRoll pins the lethal strike the
// continuous handler rolls on every creature target after the landing roll:
// a failed landing still rolls it, ATTACK_FAILED is reported before the
// lethal, a reflected cast rolls it against the caster, and the effect-id
// substitute skill supplies the lethal chances.
func TestContinuousRollsLethalAfterLandingRoll(t *testing.T) {
	sureLethal := formulas.LethalInput{AttackerLevel: 40, TargetLevel: 40, LethalMul: 1}
	fear := modelskill.Definition{SkillType: "FEAR", Offensive: true, LethalChance2: 100}

	t.Run("failed landing", func(t *testing.T) {
		target := &skillTarget{hp: 500, lethalInput: sureLethal, lethalOK: true}
		result, _ := NewDefaultRegistry().UseResult(Cast{Caster: &skillTarget{hp: 500}, Skill: fear, Targets: []Actor{target}})
		if result.AttackFailed != 1 || target.hp != 1 {
			t.Fatalf("AttackFailed = %d, target HP = %v; want 1 and a lethal strike to 1 HP", result.AttackFailed, target.hp)
		}
		if len(result.Messages) != 2 {
			t.Fatalf("messages = %#v, want ATTACK_FAILED then the lethal", result.Messages)
		}
		if _, ok := result.Messages[0].(AttackFailedMessage); !ok {
			t.Fatalf("first message = %T, want AttackFailedMessage", result.Messages[0])
		}
		if _, ok := result.Messages[1].(Lethal); !ok {
			t.Fatalf("second message = %T, want Lethal", result.Messages[1])
		}
	})

	t.Run("reflected onto caster", func(t *testing.T) {
		caster := &skillTarget{hp: 500, lethalInput: sureLethal, lethalOK: true}
		target := &skillTarget{hp: 500, reflects: true, lethalInput: sureLethal, lethalOK: true}
		result, _ := NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: fear, Targets: []Actor{target}})
		if caster.hp != 1 || target.hp != 500 {
			t.Fatalf("caster HP = %v, target HP = %v; want the reflected lethal on the caster only", caster.hp, target.hp)
		}
		if len(result.Lethals) != 1 {
			t.Fatalf("lethals = %+v, want one", result.Lethals)
		}
	})

	t.Run("raid related target", func(t *testing.T) {
		target := &skillTarget{hp: 500, raidRelated: true, lethalInput: sureLethal, lethalOK: true}
		result, _ := NewDefaultRegistry().UseResult(Cast{Caster: &skillTarget{hp: 500}, Skill: fear, Targets: []Actor{target}})
		if target.hp != 500 || len(result.Lethals) != 0 {
			t.Fatalf("raid target HP = %v, lethals = %+v; want untouched", target.hp, result.Lethals)
		}
	})

	t.Run("effect-id substitute supplies the chances", func(t *testing.T) {
		defs := continuousDefinitions{{ID: 7, Level: 1}: {ID: 7, Level: 1, SkillType: "DEBUFF", Debuff: true, LethalChance2: 100}}
		target := &skillTarget{hp: 500, lethalInput: sureLethal, lethalOK: true}
		continuousHandler{defs: defs}.UseResult(Cast{
			Caster:  &skillTarget{hp: 500},
			Skill:   modelskill.Definition{SkillType: "DEBUFF", Debuff: true, EffectID: 7},
			Targets: []Actor{target},
		})
		if target.hp != 1 {
			t.Fatalf("target HP = %v, want the substitute skill's lethal strike to 1 HP", target.hp)
		}
	})
}

// ---- from cubic_test.go ----
type fakeCubicSummoner struct {
	neutralCreature
	world.Presence
	fakeActor
	added        map[cubic.ID]bool
	givenByOther map[cubic.ID]bool
	nextAdded    bool
	servitor     modelskill.Definition
}

// newFakeCubicSummoner returns a summoner with its own world object id, so a
// mass cast can tell the caster from the other recipients.
func newFakeCubicSummoner(nextAdded bool) *fakeCubicSummoner {
	return &fakeCubicSummoner{
		fakeActor:    fakeActor{objectID: nextFakeObjectID()},
		added:        map[cubic.ID]bool{},
		givenByOther: map[cubic.ID]bool{},
		nextAdded:    nextAdded,
	}
}

func (f *fakeCubicSummoner) AddOrRefreshCubic(id cubic.ID, givenByOther bool) (touched, added bool) {
	f.added[id] = true
	f.givenByOther[id] = givenByOther
	return true, f.nextAdded
}

func (f *fakeCubicSummoner) SummonServitor(def modelskill.Definition) {
	f.servitor = def
}

func TestCubicHandlerAddsToSelfWhenSingleTarget(t *testing.T) {
	caster := newFakeCubicSummoner(true)

	result := cubicHandler{}.UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Storm)},
		Targets: []Actor{caster},
	})

	if !result.CubicAdded {
		t.Fatal("UseResult().CubicAdded = false, want true")
	}
	if !caster.added[cubic.Storm] {
		t.Fatal("caster's cubic list was never touched")
	}
	if caster.givenByOther[cubic.Storm] {
		t.Fatal("caster's own cast reported givenByOther=true, want false")
	}
}

func TestCubicHandlerDelegatesServitorBranch(t *testing.T) {
	caster := newFakeCubicSummoner(true)

	result := cubicHandler{}.UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "SUMMON", IsCubic: false, NpcID: 14848},
		Targets: []Actor{caster},
	})

	if result.CubicAdded {
		t.Fatal("UseResult().CubicAdded = true for a non-cubic SUMMON skill, want false")
	}
	if len(caster.added) != 0 {
		t.Fatal("servitor-branch cast touched the cubic list, want untouched")
	}
	if caster.servitor.NpcID != 14848 {
		t.Fatalf("SummonServitor() NpcID = %d, want 14848", caster.servitor.NpcID)
	}
}

func TestCubicHandlerMassCubicMarksOthersGivenByOther(t *testing.T) {
	caster := newFakeCubicSummoner(true)
	other := newFakeCubicSummoner(true)

	result := cubicHandler{}.UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Storm)},
		Targets: []Actor{caster, other},
	})

	if !result.CubicAdded {
		t.Fatal("UseResult().CubicAdded = false, want true (caster's own admission)")
	}
	if caster.givenByOther[cubic.Storm] {
		t.Fatal("caster's own admission reported givenByOther=true, want false")
	}
	if !other.givenByOther[cubic.Storm] {
		t.Fatal("other recipient's admission reported givenByOther=false, want true")
	}
	if got := result.CubicTargets; len(got) != 1 || got[0] != other {
		t.Fatalf("CubicTargets = %v, want other", got)
	}
	if got := result.CubicAddedTargets; len(got) != 1 || got[0] != other {
		t.Fatalf("CubicAddedTargets = %v, want other", got)
	}
	if result.CubicID != cubic.Storm {
		t.Fatalf("CubicID = %d, want %d", result.CubicID, cubic.Storm)
	}
}

func TestCubicHandlerRegisteredForSummonType(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newFakeCubicSummoner(true)

	if !registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Vampiric)},
		Targets: []Actor{caster},
	}) {
		t.Fatal("Use() returned false for SUMMON")
	}
	if !caster.added[cubic.Vampiric] {
		t.Fatal("registry dispatch never reached cubicHandler")
	}
}

// ---- from disablers_test.go ----
// disablerFake is a Combatant (for the hate-table skill types) that also
// satisfies every optional interface disablersHandler probes for, wired to
// a guaranteed-success SkillSuccessInput by default (IgnoreResists with a
// 100 base chance always beats a [0,100) roll).
type disablerFake struct {
	world.Presence
	neutralCreature
	id                     int32
	dead, invul, paralyzed bool
	list                   *effect.List
	successOK              bool
	attackableFlag         bool
	raidRelated            bool
	undeadFlag             bool
	aggro                  *attackable.ThreatTable
	hate                   *attackable.HateTable
	shield                 formulas.ShieldDefense
	level                  int
	reflects               bool
	name                   string
	// failRoll makes the skill's own landing roll against d always fail.
	failRoll bool

	// shieldRolls counts ShieldDefense calls; templateLandings records the
	// blessed-spiritshot and shield inputs of every per-template landing
	// roll made against d.
	shieldRolls      int
	templateLandings []templateLanding

	// lastBss and lastShield record the most recent SkillSuccessInput call's
	// resolved caster/target state, for tests asserting checkSkillSuccess
	// threaded them through correctly.
	lastBss    bool
	lastShield formulas.ShieldDefense

	// aggressionSource and aggressionPower record the most recent
	// NotifyAggression call, for tests asserting AGGDAMAGE's aggro
	// notification.
	aggressionSource any
	aggressionPower  int
}

func newDisablerFake(id int32) *disablerFake {
	d := &disablerFake{id: id, list: newTestList(nil), successOK: true}
	d.aggro = attackable.NewThreatTable(d, time.Now)
	d.hate = attackable.NewHateTable(d)
	return d
}

func (d *disablerFake) ObjectID() int32          { return d.id }
func (d *disablerFake) SiegeGuard() bool         { return false }
func (d *disablerFake) AlikeDead() bool          { return d.dead }
func (d *disablerFake) Dead() bool               { return d.dead }
func (d *disablerFake) Invul() bool              { return d.invul }
func (d *disablerFake) Paralyzed() bool          { return d.paralyzed }
func (d *disablerFake) EffectList() *effect.List { return d.list }

func (d *disablerFake) SkillSuccessInput(caster creature.FormulaActor, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	d.lastBss = bss
	d.lastShield = shield
	chance := 100.0
	if d.failRoll {
		chance = 0
	}
	return formulas.SkillSuccessInput{IgnoreResists: true, BaseChance: chance, Shield: shield}, d.successOK
}

func (d *disablerFake) CharacterName() string { return d.name }

// ShieldDefense reports d's pre-set shield-block outcome, letting tests
// exercise checkSkillSuccess's shield-block threading.
func (d *disablerFake) ShieldDefense(caster creature.FormulaActor, def modelskill.Definition, isCrit bool) formulas.ShieldDefense {
	d.shieldRolls++
	return d.shield
}

// templateLanding is one per-template landing roll's resolved inputs.
type templateLanding struct {
	bss    bool
	shield formulas.ShieldDefense
}

// EffectSuccessInput records the landing inputs and lands the template
// unless the shield block was perfect.
func (d *disablerFake) EffectSuccessInput(_ creature.FormulaActor, _ modelskill.Definition, _ modelskill.EffectTemplate, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	d.templateLandings = append(d.templateLandings, templateLanding{bss: bss, shield: shield})
	return formulas.SkillSuccessInput{IgnoreResists: true, BaseChance: 100, Shield: shield}, true
}

// SkillReflectInput reports a guaranteed reflect when d.reflects is set
// (ReflectChance 100 always beats a [0,100) roll), and no reflect otherwise.
func (d *disablerFake) SkillReflectInput(def modelskill.Definition) formulas.SkillReflectInput {
	if !d.reflects {
		return formulas.SkillReflectInput{}
	}
	return formulas.SkillReflectInput{CanBeReflected: true, Magic: true, ReflectChance: 100}
}

func (d *disablerFake) Attackable() bool                  { return d.attackableFlag }
func (d *disablerFake) RaidRelated() bool                 { return d.raidRelated }
func (d *disablerFake) Undead() bool                      { return d.undeadFlag }
func (d *disablerFake) ReduceAllAggroHate(amount float64) { d.aggro.ReduceAllHate(amount) }
func (d *disablerFake) StopAggroHate(attacker attackable.Combatant) {
	d.aggro.StopHate(attacker)
}
func (d *disablerFake) StopHateList(attacker attackable.Combatant) { d.hate.StopHate(attacker) }
func (d *disablerFake) ClearAggroTables() {
	d.aggro.Clear()
	d.hate.Clear()
}
func (d *disablerFake) Level() int { return d.level }

func (d *disablerFake) NotifyAggression(source attackable.Combatant, power int) {
	d.aggressionSource = source
	d.aggressionPower = power
}

func TestDisablersSkipsDeadAndUnparalyzedInvulTargets(t *testing.T) {
	registry := NewDefaultRegistry()
	dead := newDisablerFake(1)
	dead.dead = true
	invul := newDisablerFake(2)
	invul.invul = true

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "FAKE_DEATH", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{dead, invul},
	})

	if len(dead.list.All()) != 0 || len(invul.list.All()) != 0 {
		t.Fatal("a dead or unparalyzed-invulnerable target must never receive an effect")
	}
}

func TestDisablersRespectsBlockDebuffForOffensiveSkills(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)
	blocker, err := effect.New(effect.Skill{}, modelskill.EffectTemplate{Name: "Buff", EffectType: "BLOCK_DEBUFF"})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	blocker.Effected = target
	target.list.Add(blocker)

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "FAKE_DEATH", Offensive: true, Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{target},
	})

	if len(target.list.All()) != 1 {
		t.Fatalf("target under BLOCK_DEBUFF should not receive a new offensive effect, got %d effects", len(target.list.All()))
	}
}

func TestDisablersRespectsBlockDebuffFromRealMarkerEffect(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)

	// A real BlockDebuff marker loaded from the datapack carries no effectType
	// attribute; its debuff immunity is resolved from the runtime kind.
	blocker, err := effect.New(effect.Skill{}, modelskill.EffectTemplate{Name: "BlockDebuff", Time: 600})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	blocker.Effected = target
	target.list.Add(blocker)

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "FAKE_DEATH", Offensive: true, Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{target},
	})

	if len(target.list.All()) != 1 {
		t.Fatalf("target under BlockDebuff should not receive a new offensive effect, got %d effects", len(target.list.All()))
	}
}

func TestFakeDeathAppliesUnconditionally(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)
	target.successOK = false // even without a success source, FAKE_DEATH doesn't roll

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "FAKE_DEATH", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{target},
	})
	if len(target.list.All()) != 1 {
		t.Fatal("FAKE_DEATH should apply its effects with no success check")
	}
}

func TestStunAppliesOnGuaranteedSuccess(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "STUN", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{target},
	})
	if len(target.list.All()) != 1 {
		t.Fatal("STUN should apply its effect on a guaranteed-success roll")
	}
}

func TestControlDisablersReportFailedRollToPlayer(t *testing.T) {
	for _, skillType := range []string{"ROOT", "STUN", "SLEEP", "PARALYZE", "MUTE"} {
		t.Run(skillType, func(t *testing.T) {
			caster := &skillTarget{isPlayer: true}
			target := &skillTarget{name: "Victim", effects: newTestList(nil), skillSuccessOK: true, skillSuccessChance: chanceOf(0)}
			def := modelskill.Definition{ID: 44, Level: 7, SkillType: skillType}
			result, ok := NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
			if !ok || len(result.Resisted) != 1 || result.Resisted[0] != (Resisted{TargetName: "Victim", SkillID: 44, SkillLevel: 1}) {
				t.Fatalf("UseResult() = %+v, %t; want one level-1 resisted message", result.Resisted, ok)
			}
			if len(result.Messages) != 1 || result.Messages[0] != result.Resisted[0] {
				t.Fatalf("Messages = %+v, want the resisted message in order", result.Messages)
			}
		})
	}
}

// TestDisablersReportFailedRollAtCastLevel covers the Disablers types
// whose resist names the skill with its level: a failed landing roll tells a
// player caster each target resisted the skill at the cast level, in target
// order, a landed roll tells it nothing, and a non-player caster hears
// nothing either way.
func TestDisablersReportFailedRollAtCastLevel(t *testing.T) {
	for _, skillType := range []string{"BETRAY", "CONFUSION", "AGGREDUCE_CHAR", "AGGREMOVE", "ERASE"} {
		t.Run(skillType, func(t *testing.T) {
			def := modelskill.Definition{ID: 1380, Level: 7, SkillType: skillType}
			targets := func(fail bool) []Actor {
				var out []Actor
				for i, name := range []string{"First", "Second"} {
					target := newDisablerFake(int32(i + 1))
					target.name = name
					target.attackableFlag = true
					target.failRoll = fail
					out = append(out, target)
				}
				return out
			}

			result, ok := NewDefaultRegistry().UseResult(Cast{Caster: &skillTarget{isPlayer: true}, Skill: def, Targets: targets(true)})
			want := []Resisted{{TargetName: "First", SkillID: 1380, SkillLevel: 7}, {TargetName: "Second", SkillID: 1380, SkillLevel: 7}}
			if !ok || !slices.Equal(result.Resisted, want) {
				t.Fatalf("player caster, failed rolls: Resisted = %+v, handled = %t; want %+v", result.Resisted, ok, want)
			}
			if len(result.Messages) != 2 || result.Messages[0] != any(want[0]) || result.Messages[1] != any(want[1]) {
				t.Fatalf("player caster, failed rolls: Messages = %+v, want the resists in target order", result.Messages)
			}

			result, _ = NewDefaultRegistry().UseResult(Cast{Caster: &skillTarget{isPlayer: true}, Skill: def, Targets: targets(false)})
			if len(result.Resisted) != 0 || len(result.Messages) != 0 {
				t.Fatalf("player caster, landed rolls: Resisted = %+v, Messages = %+v; want none", result.Resisted, result.Messages)
			}

			result, _ = NewDefaultRegistry().UseResult(Cast{Caster: &skillTarget{}, Skill: def, Targets: targets(true)})
			if len(result.Resisted) != 0 || len(result.Messages) != 0 {
				t.Fatalf("NPC caster, failed rolls: Resisted = %+v, Messages = %+v; want none", result.Resisted, result.Messages)
			}
		})
	}
}

// TestConfusionOnNonNPCTellsPlayerInvalidTarget: CONFUSION rolls nothing
// against a target that is no NPC and tells a player caster the target is
// invalid, in target order among the cast's resists; an NPC caster hears
// nothing.
func TestConfusionOnNonNPCTellsPlayerInvalidTarget(t *testing.T) {
	def := modelskill.Definition{ID: 2, Level: 3, SkillType: "CONFUSION", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}}
	targets := func() (*disablerFake, *disablerFake) {
		player := newDisablerFake(1)
		npc := newDisablerFake(2)
		npc.name = "Monster"
		npc.attackableFlag = true
		npc.failRoll = true
		return player, npc
	}

	player, npc := targets()
	result, ok := NewDefaultRegistry().UseResult(Cast{Caster: &skillTarget{isPlayer: true}, Skill: def, Targets: []Actor{player, npc}})
	resisted := Resisted{TargetName: "Monster", SkillID: 2, SkillLevel: 3}
	if !ok || len(result.Messages) != 2 || result.Messages[0] != any(InvalidTargetMessage{}) || result.Messages[1] != any(resisted) {
		t.Fatalf("Messages = %+v, handled = %t; want invalid target, then %+v", result.Messages, ok, resisted)
	}
	if player.shieldRolls != 1 || len(player.list.All()) != 0 {
		t.Fatalf("non-NPC target: shield rolls %d, effects %d; want its shield rolled and no effect", player.shieldRolls, len(player.list.All()))
	}

	player, npc = targets()
	result, _ = NewDefaultRegistry().UseResult(Cast{Caster: &skillTarget{}, Skill: def, Targets: []Actor{player, npc}})
	if len(result.Messages) != 0 {
		t.Fatalf("NPC caster: Messages = %+v, want none", result.Messages)
	}
}

func TestControlDisablersDoNotReportFailedRollForNPCCaster(t *testing.T) {
	caster := &skillTarget{}
	target := &skillTarget{effects: newTestList(nil), skillSuccessOK: true, skillSuccessChance: chanceOf(0)}
	result, ok := NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: modelskill.Definition{ID: 44, Level: 7, SkillType: "STUN"}, Targets: []Actor{target}})
	if !ok || len(result.Resisted) != 0 {
		t.Fatalf("UseResult() = %+v, %t; want no player-only resist", result.Resisted, ok)
	}
}

func TestReflectedStunReportsResistedCasterAsTarget(t *testing.T) {
	caster := &skillTarget{isPlayer: true, name: "Caster", skillSuccessOK: true, skillSuccessChance: chanceOf(0)}
	target := newDisablerFake(1)
	target.reflects = true
	result, ok := NewDefaultRegistry().UseResult(Cast{
		Caster: caster, Skill: modelskill.Definition{ID: 44, Level: 7, SkillType: "STUN"}, Targets: []Actor{target},
	})
	if !ok || len(result.Resisted) != 1 || result.Resisted[0] != (Resisted{TargetName: "Caster", SkillID: 44, SkillLevel: 1}) {
		t.Fatalf("Resisted = %+v, handled = %t; want reflected caster named at level 1", result.Resisted, ok)
	}
}

// TestReflectedStunUsesOriginalTargetsPreSwapShieldBlock proves the shield
// roll that gates a reflected STUN/ROOT/SLEEP/PARALYZE cast is resolved
// against the original target before the reflect swap, matching
// Disablers.java:64 (calcShldUse against targetCreature, once, ahead of the
// switch) and :80-83 (the reflect reassignment happens after sDef is fixed).
// The original target perfect-blocks and reflects; the caster's own shield
// state must never be consulted for the reflected cast.
func TestReflectedStunUsesOriginalTargetsPreSwapShieldBlock(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)
	target.shield = formulas.ShieldPerfect
	target.reflects = true
	caster := newDisablerFake(2)
	caster.shield = formulas.ShieldFailed // must never be consulted

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "STUN", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{target},
	})

	if caster.lastShield != formulas.ShieldPerfect {
		t.Fatalf("lastShield = %v, want the original target's pre-swap ShieldPerfect", caster.lastShield)
	}
	if len(caster.list.All()) != 0 {
		t.Fatal("original target's perfect block must fail the reflected cast against the caster")
	}
}

// TestReflectedMuteUsesOriginalTargetsPreSwapShieldBlock is
// TestReflectedStunUsesOriginalTargetsPreSwapShieldBlock for the MUTE case
// (Disablers.java:90-93), which resolves shield the same way.
func TestReflectedMuteUsesOriginalTargetsPreSwapShieldBlock(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)
	target.shield = formulas.ShieldPerfect
	target.reflects = true
	caster := newDisablerFake(2)
	caster.shield = formulas.ShieldFailed // must never be consulted

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "MUTE", Effects: []modelskill.EffectTemplate{{Name: "Mute", Time: 10}}},
		Targets: []Actor{target},
	})

	if caster.lastShield != formulas.ShieldPerfect {
		t.Fatalf("lastShield = %v, want the original target's pre-swap ShieldPerfect", caster.lastShield)
	}
	if len(caster.list.All()) != 0 {
		t.Fatal("original target's perfect block must fail the reflected cast against the caster")
	}
}

func TestControlDisablersApplyToHostileNPC(t *testing.T) {
	registry := NewDefaultRegistry()
	tests := []struct {
		skillType  string
		effectName string
	}{
		{skillType: "STUN", effectName: "Stun"},
		{skillType: "ROOT", effectName: "Root"},
		{skillType: "SLEEP", effectName: "Sleep"},
		{skillType: "PARALYZE", effectName: "Paralyze"},
	}

	for _, tt := range tests {
		t.Run(tt.skillType, func(t *testing.T) {
			target := newTestHostile(t, 100, 0)

			registry.Use(Cast{
				Caster: &bssCasterFake{},
				Skill: modelskill.Definition{
					SkillType:     tt.skillType,
					EffectType:    tt.skillType,
					BaseLandRate:  100,
					IgnoreResists: true,
					Effects: []modelskill.EffectTemplate{{
						Name: tt.effectName,
						Time: 10,
					}},
				},
				Targets: []Actor{target},
			})

			if len(target.EffectList().All()) != 1 {
				t.Fatalf("%s should apply its effect to a hostile NPC target", tt.skillType)
			}
		})
	}
}

func TestCancelDebuffStripsOnlyDispellableDebuffsUpToLimit(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)

	// Distinct skill ids keep the effect list from treating these as
	// duplicate applications of "the same" effect and silently dropping
	// one (List.Add's identical-effect collision handling).
	a, _ := effect.New(effect.Skill{ID: 1, Debuff: true, CanBeDispelled: true}, modelskill.EffectTemplate{Name: "Debuff"})
	b, _ := effect.New(effect.Skill{ID: 2, Debuff: true, CanBeDispelled: true}, modelskill.EffectTemplate{Name: "Debuff"})
	notDispellable, _ := effect.New(effect.Skill{ID: 3, Debuff: true, CanBeDispelled: false}, modelskill.EffectTemplate{Name: "Debuff"})
	notDebuff, _ := effect.New(effect.Skill{ID: 4, Debuff: false, CanBeDispelled: true}, modelskill.EffectTemplate{Name: "Buff"})
	for _, e := range []*effect.Effect{a, b, notDispellable, notDebuff} {
		e.Effected = target
		target.list.Add(e)
	}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "CANCEL_DEBUFF", MaxNegatedEffects: 1},
		Targets: []Actor{target},
	})

	remaining := target.list.All()
	if len(remaining) != 3 {
		t.Fatalf("expected exactly 1 debuff stripped (limit=1), got %d effects remaining", len(remaining))
	}
	if !hasEffect(target.list, notDispellable) {
		t.Error("a non-dispellable debuff must never be stripped")
	}
	if !hasEffect(target.list, notDebuff) {
		t.Error("a non-debuff effect must never be stripped by CANCEL_DEBUFF")
	}
	if hasEffect(target.list, a) && hasEffect(target.list, b) {
		t.Error("exactly one of the two dispellable debuffs should have been stripped (limit=1)")
	}
}

func TestNegateByIDStripsMatchingEffect(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)

	targeted, _ := effect.New(effect.Skill{ID: 42}, modelskill.EffectTemplate{Name: "Buff"})
	untouched, _ := effect.New(effect.Skill{ID: 43}, modelskill.EffectTemplate{Name: "Buff"})
	targeted.Effected, untouched.Effected = target, target
	target.list.Add(targeted)
	target.list.Add(untouched)

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "NEGATE", NegateIDs: []int{42}},
		Targets: []Actor{target},
	})

	if hasEffect(target.list, targeted) {
		t.Error("NEGATE should strip the effect matching its negate id list")
	}
	if !hasEffect(target.list, untouched) {
		t.Error("NEGATE should not strip an effect outside its negate id list")
	}
}

func TestAggRemoveSkipsNonAttackableAndRaidRelatedTargets(t *testing.T) {
	registry := NewDefaultRegistry()

	notAttackable := newDisablerFake(1)
	notAttackable.aggro.AddDamage(newDisablerFake(9), 50, 50)

	raidRelated := newDisablerFake(2)
	raidRelated.attackableFlag = true
	raidRelated.raidRelated = true
	raidRelated.aggro.AddDamage(newDisablerFake(9), 50, 50)

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "AGGREMOVE"},
		Targets: []Actor{notAttackable, raidRelated},
	})

	if notAttackable.aggro.IsEmpty() {
		t.Error("a non-attackable target's aggro should be untouched")
	}
	if raidRelated.aggro.IsEmpty() {
		t.Error("a raid-related target's aggro should be untouched")
	}
}

func TestAggDamageNotifiesAttackableTargetAndAppliesEffectsUnconditionally(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newDisablerFake(9)
	target := newDisablerFake(1)
	target.attackableFlag = true
	target.level = 43
	target.successOK = false // AGGDAMAGE never rolls; effects apply regardless

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "AGGDAMAGE", Power: 100, Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{target},
	})

	if target.aggressionSource != caster {
		t.Fatalf("aggression source = %v, want caster", target.aggressionSource)
	}
	// power/(targetLevel+7)*150 = 100/(43+7)*150 = 300.
	if target.aggressionPower != 300 {
		t.Fatalf("aggression power = %d, want 300", target.aggressionPower)
	}
	if len(target.list.All()) != 1 {
		t.Fatalf("AGGDAMAGE must apply its effects unconditionally, got %d effects", len(target.list.All()))
	}
}

func TestAggDamageSkipsAggroNotificationForNonAttackableTarget(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newDisablerFake(9)
	target := newDisablerFake(1) // not attackable

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "AGGDAMAGE", Power: 100, Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{target},
	})

	if target.aggressionSource != nil {
		t.Fatalf("a non-attackable target must never receive an aggro notification, got source %v", target.aggressionSource)
	}
	if len(target.list.All()) != 1 {
		t.Fatalf("AGGDAMAGE must still apply its effects to a non-attackable target, got %d effects", len(target.list.All()))
	}
}

func TestAggRemoveClearsBothTablesOnSuccess(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)
	target.attackableFlag = true
	attacker := newDisablerFake(9)
	target.aggro.AddDamage(attacker, 50, 50)
	target.hate.Add(attacker, 50)

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "AGGREMOVE"},
		Targets: []Actor{target},
	})

	if !target.aggro.IsEmpty() || !target.hate.IsEmpty() {
		t.Fatal("AGGREMOVE should clear both hate tables on a guaranteed-success roll")
	}
}

// bssCasterFake exposes a fixed blessed-spiritshot charge state for tests
// asserting checkSkillSuccess resolves it from the caster.
type bssCasterFake struct {
	neutralCreature
	world.Presence
	fakeActor
	bss bool
}

func (c *bssCasterFake) BlessedSpiritshotCharged() bool { return c.bss }

// newTestHostile builds a real Monster-kind NPC, for the cases that are
// about what a handler does to an actor rather than about one narrow
// surface of it. pAtk is the one stat a physical-skill damage roll needs
// from its caster; a target can leave it at 0.
func newTestHostile(t testing.TB, id int32, pAtk float64) *npc.Hostile {
	t.Helper()
	live, err := creature.NewLive(location.Location{}, 100, disablerHostileGeo{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	live.SetQueue(idleQueue())
	h, err := npc.NewHostile(&npc.Instance{
		ObjectID: id,
		Kind:     "Monster",
		Template: &npc.Template{
			ID:              int(id),
			Type:            "Monster",
			Level:           1,
			CON:             40,
			MEN:             40,
			HPMax:           1000,
			PAtk:            pAtk,
			PDef:            1,
			MAtk:            1,
			MDef:            1,
			BaseAttackRange: 40,
			CanMove:         true,
		},
	}, live, disablerHostileMove{}, disablerHostileAttack{})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

type disablerHostileGeo struct{}

func (disablerHostileGeo) CanMove(_, _, _, _, _, _ int) bool { return true }
func (disablerHostileGeo) Height(_, _, _ int) int16          { return 0 }
func (disablerHostileGeo) FindPath(_, _ location.Location) ([]location.Location, bool) {
	return nil, false
}
func (disablerHostileGeo) Walkable(int, int, int) bool { return true }
func (disablerHostileGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

type disablerHostileMove struct{}

func (disablerHostileMove) MaybeStartOffensiveFollow(attackable.Combatant, int) (bool, error) {
	return false, nil
}
func (disablerHostileMove) MoveHome(location.Location) error { return nil }
func (disablerHostileMove) Stop()                            {}
func (disablerHostileMove) CancelFollow()                    {}

type disablerHostileAttack struct{}

func (disablerHostileAttack) BowCoolingDown() bool                { return false }
func (disablerHostileAttack) AttackingNow() bool                  { return false }
func (disablerHostileAttack) CanAttack(attackable.Combatant) bool { return false }
func (disablerHostileAttack) DoAttack(attackable.Combatant)       {}
func (disablerHostileAttack) Stop()                               {}

func TestCheckSkillSuccessFailsOnPerfectShieldBlockDespiteGuaranteedRate(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)
	target.shield = formulas.ShieldPerfect

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "STUN", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{target},
	})

	if len(target.list.All()) != 0 {
		t.Fatal("a perfect shield block must fail the roll even though the target reports a guaranteed-success rate")
	}
	if target.lastShield != formulas.ShieldPerfect {
		t.Fatalf("lastShield = %v, want ShieldPerfect", target.lastShield)
	}
}

func TestCheckSkillSuccessUsesLivePlayerShieldDefense(t *testing.T) {
	registry := NewDefaultRegistry()
	items := liveShieldItems()
	caster := liveShieldCharacter(t, 1, items)
	unblocked := liveShieldCharacter(t, 2, items)
	blocked := liveShieldCharacter(t, 3, items, &item.Instance{
		ObjectID: 30, TemplateID: 3, Location: item.LocationPaperdoll, LocationData: itemcontainer.LHand,
	})
	caster.SetLastKnownPosition(location.Location{X: 80, Y: 0, Z: 0}, 0)
	for _, target := range []*player.Character{unblocked, blocked} {
		target.SetLastKnownPosition(location.Location{X: 0, Y: 0, Z: 0}, 0)
		owner := effect.ModOwnerSkill(modelskill.Ref{ID: 1, Level: 1})
		target.AddStatFuncs([]effect.Mod{
			{Stat: stat.ShieldRate, Op: effect.OpSet, Value: 20, Owner: owner},
			{Stat: stat.ShieldDefenceAngle, Op: effect.OpSet, Value: 120, Owner: owner},
		})
	}
	blocked.SetRollSource(func(n int) int {
		if n != 100 {
			t.Fatalf("shield roll bound = %d, want 100", n)
		}
		return 0
	})

	skill := modelskill.Definition{
		SkillType:     "STUN",
		EffectType:    "STUN",
		BaseLandRate:  100,
		IgnoreResists: true,
		Effects:       []modelskill.EffectTemplate{{Name: "Stun", Time: 10}},
	}
	tests := []struct {
		name   string
		target *player.Character
		want   int
	}{
		{name: "unblocked", target: unblocked, want: 1},
		{name: "perfect shield blocked", target: blocked, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry.Use(Cast{Caster: caster, Skill: skill, Targets: []Actor{tt.target}})
			if got := len(tt.target.EffectList().All()); got != tt.want {
				t.Fatalf("target effects = %d, want %d", got, tt.want)
			}
		})
	}
}

type liveShieldGeo struct{}

func (liveShieldGeo) CanMove(_, _, _, _, _, _ int) bool { return true }
func (liveShieldGeo) Height(_, _, _ int) int16          { return 0 }
func (liveShieldGeo) FindPath(_, _ location.Location) ([]location.Location, bool) {
	return nil, false
}
func (liveShieldGeo) Walkable(int, int, int) bool { return true }
func (liveShieldGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

func liveShieldItems() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{Type: item.WeaponFist}},
		{ID: 3, Kind: item.KindArmor, Slot: item.SlotLHand, Armor: &item.ArmorDetail{Type: item.ArmorShield}},
	})
}

func liveShieldTemplate() *player.Template {
	return &player.Template{
		ID: 0, FistsItemID: 1,
		STR: 40, CON: 43, DEX: 30, INT: 21, WIT: 11, MEN: 25,
		PAtk: 5, PDef: 50, MAtk: 25, MDef: 40,
		CollisionRadius: 9, CollisionHeight: 23,
		HPTable: []float64{100}, MPTable: []float64{30}, CPTable: []float64{0},
	}
}

func liveShieldCharacter(t *testing.T, id int32, items *item.Table, equipped ...*item.Instance) *player.Character {
	t.Helper()
	tmpl := liveShieldTemplate()
	c := &player.Character{
		ID: id, Name: "char", ClassID: tmpl.ID, BaseClassID: tmpl.ID,
		Race: player.RaceHuman, Sex: player.SexMale, CharLevel: 1,
		Location: location.Location{X: int(id) * 100, Y: 0, Z: 0},
	}
	c.SetResourceValues(player.Resources{MaxHP: 100, CurrentHP: 100, MaxMP: 30, CurrentMP: 30})
	c.AttachRuntime(tmpl, itemcontainer.RestorePlayerInventory(c.ID, items, equipped))
	live, err := creature.NewLive(c.Location, 0, liveShieldGeo{}, c)
	if err != nil {
		t.Fatal(err)
	}
	live.SetQueue(idleQueue())
	c.Live = live
	c.SetRollSource(func(int) int { return 99 })
	c.Configure(player.Runtime{Rules: player.Rules{PerfectShieldBlockRate: 5}})
	return c
}

func TestCheckSkillSuccessResolvesCasterBlessedSpiritshotCharge(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)
	caster := &bssCasterFake{bss: true}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "STUN", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{target},
	})

	if !target.lastBss {
		t.Fatal("checkSkillSuccess should have resolved the caster's blessed-spiritshot charge as true")
	}
}

// ---- from extractable_test.go ----
type extractableFakeCaster struct {
	neutralCreature
	world.Presence
	fakeActor
	granted  map[int32]int
	capacity bool
}

func (c *extractableFakeCaster) AddItem(itemID int32, count int) {
	if c.granted == nil {
		c.granted = make(map[int32]int)
	}
	c.granted[itemID] += count
}

func (c *extractableFakeCaster) HasCapacityFor(itemIDs []int32) bool { return c.capacity }

func TestExtractableGrantsTheOnlyGuaranteedProduct(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &extractableFakeCaster{capacity: true}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "EXTRACTABLE", ExtractableItems: "57,10,100.0"},
		Targets: []Actor{},
	})

	if caster.granted[57] != 10 {
		t.Fatalf("granted = %v, want {57: 10}", caster.granted)
	}
}

func TestExtractableFullInventoryGrantsNothing(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &extractableFakeCaster{capacity: false}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "EXTRACTABLE", ExtractableItems: "57,10,100.0"},
		Targets: []Actor{},
	})

	if len(caster.granted) != 0 {
		t.Fatalf("granted = %v, want none when inventory is full", caster.granted)
	}
}

func TestExtractableNoDataIsNoop(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &extractableFakeCaster{capacity: true}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "EXTRACTABLE_FISH"},
		Targets: []Actor{},
	})
	if len(caster.granted) != 0 {
		t.Fatalf("granted = %v, want none without extractable data", caster.granted)
	}
}

// ---- from fusion_test.go ----
// battleForce is the FUSION-skillType caster skill (id 426), triggering
// Battle Force (id 5104) at its own level.
func battleForce(level int) modelskill.Definition {
	return modelskill.Definition{
		ID: 426, Level: level, SkillType: "FUSION",
		TriggeredID: 5104, TriggeredLevel: level,
	}
}

func battleForceDefs() continuousDefinitions {
	defs := continuousDefinitions{}
	for level := 1; level <= 3; level++ {
		defs[modelskill.Ref{ID: 5104, Level: level}] = modelskill.Definition{
			ID: 5104, Level: level, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: "Fusion", Time: 600}},
		}
	}
	return defs
}

func TestFusionHandlerAppliesTriggeredSkillFreshWhenTargetHasNone(t *testing.T) {
	target := newContinuousFake(1)
	registry := NewDefaultRegistryWithDefinitions(battleForceDefs())

	registry.Use(Cast{Caster: newContinuousFake(2), Skill: battleForce(1), Targets: []Actor{target}})

	e := firstEffectByID(target.list, 5104)
	if e == nil {
		t.Fatal("no fusion effect applied to a target with no prior one")
	}
	if e.Level != 1 {
		t.Fatalf("fresh fusion effect Level = %d, want 1", e.Level)
	}
}

func TestFusionHandlerRecastGrowsExistingEffectInPlace(t *testing.T) {
	target := newContinuousFake(1)
	registry := NewDefaultRegistryWithDefinitions(battleForceDefs())
	cast := Cast{Caster: newContinuousFake(2), Skill: battleForce(1), Targets: []Actor{target}}

	registry.Use(cast)
	first := firstEffectByID(target.list, 5104)

	registry.Use(cast)
	second := firstEffectByID(target.list, 5104)

	if second == first {
		t.Fatal("IncreaseEffect removes and reapplies a fresh instance, not the same pointer")
	}
	if second == nil || second.Level != 2 {
		t.Fatalf("Level after recast growth = %v, want 2", second)
	}
	if len(target.list.All()) != 1 {
		t.Fatalf("effect list has %d effects, want exactly 1 (no duplicate)", len(target.list.All()))
	}
}

func TestFusionHandlerCapsGrowthAtMaxLevel(t *testing.T) {
	target := newContinuousFake(1)
	registry := NewDefaultRegistryWithDefinitions(battleForceDefs())
	cast := Cast{Caster: newContinuousFake(2), Skill: battleForce(1), Targets: []Actor{target}}

	registry.Use(cast) // level 1
	registry.Use(cast) // level 2
	registry.Use(cast) // level 3 (max)
	registry.Use(cast) // no-op past max

	e := firstEffectByID(target.list, 5104)
	if e == nil || e.Level != 3 {
		t.Fatalf("Level at/past cap = %v, want 3", e)
	}
	if len(target.list.All()) != 1 {
		t.Fatalf("effect list has %d effects, want exactly 1", len(target.list.All()))
	}
}

func TestDecreaseFusionShrinksTriggeredEffectWhenChannelEnds(t *testing.T) {
	target := newContinuousFake(1)
	defs := battleForceDefs()
	registry := NewDefaultRegistryWithDefinitions(defs)
	cast := Cast{Caster: newContinuousFake(2), Skill: battleForce(1), Targets: []Actor{target}}

	registry.Use(cast)
	registry.Use(cast)
	DecreaseFusion(defs, cast.Caster, target, cast.Skill)

	e := firstEffectByID(target.list, 5104)
	if e == nil || e.Level != 1 {
		t.Fatalf("fusion level after channel end = %v, want level 1", e)
	}
}

func TestDecreaseFusionRemovesLevelOneTriggeredEffect(t *testing.T) {
	target := newContinuousFake(1)
	defs := battleForceDefs()
	registry := NewDefaultRegistryWithDefinitions(defs)
	cast := Cast{Caster: newContinuousFake(2), Skill: battleForce(1), Targets: []Actor{target}}

	registry.Use(cast)
	DecreaseFusion(defs, cast.Caster, target, cast.Skill)

	if e := firstEffectByID(target.list, 5104); e != nil {
		t.Fatalf("fusion effect after level-one channel end = %v, want removed", e)
	}
}

// ---- from handler_test.go ----
type recordingHandler struct {
	types []string
	uses  int
}

func (h *recordingHandler) Types() []string { return h.types }

func (h *recordingHandler) Use(Cast) { h.uses++ }

func TestRegistryDispatchesBySkillType(t *testing.T) {
	h := &recordingHandler{types: []string{"HEAL_PERCENT", "MANAHEAL_PERCENT"}}
	registry := NewRegistry(h)

	if _, ok := registry.Handler("heal_percent"); !ok {
		t.Fatal("Handler() did not normalize skill type keys")
	}
	if !registry.Use(Cast{Skill: modelskill.Definition{SkillType: "MANAHEAL_PERCENT"}}) {
		t.Fatal("Use() returned false for a registered skill type")
	}
	if h.uses != 1 {
		t.Fatalf("handler uses = %d, want 1", h.uses)
	}
	if registry.Use(Cast{Skill: modelskill.Definition{SkillType: "NOT_REGISTERED"}}) {
		t.Fatal("Use() returned true for an unregistered skill type")
	}
}

func TestRegistryReportsAttackFailedForPhysicalSkillWithNoDamage(t *testing.T) {
	registry := NewDefaultRegistry()
	target := &skillTarget{
		physicalOK: true,
		physicalInput: formulas.PhysicalSkillInput{
			AttackPower: -1, Defence: 1,
			RandomMul: 1, RaceMul: 1, PvPMul: 1, ElementalMul: 1, WeaponVulnMul: 1,
		},
	}

	result, ok := registry.UseResult(Cast{
		Skill:   modelskill.Definition{SkillType: "PDAM"},
		Targets: []Actor{target},
	})
	if !ok {
		t.Fatal("UseResult() handled = false, want true for PDAM")
	}
	if result.AttackFailed != 1 {
		t.Fatalf("AttackFailed = %d, want 1", result.AttackFailed)
	}
}

func TestDefaultRegistryHasRepresentativeHandlers(t *testing.T) {
	registry := NewDefaultRegistry()

	for _, skillType := range []string{
		"PDAM", "FATAL", "MDAM", "DEATHLINK", "BLOW", "MANADAM",
		"HEAL", "HEAL_STATIC", "HEAL_PERCENT", "MANAHEAL_PERCENT", "MANAHEAL", "MANARECHARGE",
		"COMBATPOINTHEAL", "BALANCE_LIFE", "REAL_DAMAGE", "GIVE_SP",
		"CPDAMPERCENT", "DUMMY", "BEAST_FEED",
		"SUMMON_CREATURE", "SUMMON_FRIEND", "SUMMON_PARTY", "ERASE",
	} {
		if _, ok := registry.Handler(skillType); !ok {
			t.Fatalf("default registry missing %s", skillType)
		}
	}
}

type skillTarget struct {
	neutralPlayer
	world.Presence
	fakeActor
	hp, maxHP float64
	mp, maxMP float64
	cp, maxCP float64

	dead         bool
	alikeDead    bool
	invulnerable bool
	cursed       bool

	sp       int
	diedBy   any
	recharge float64

	healAmount        float64
	healMAtk          int
	healScaling       formulas.HealShotScaling
	healEffectiveness float64
	healOK            bool
	// effectsAtHeal is the effect count AddHP last saw.
	effectsAtHeal int

	physicalInput formulas.PhysicalSkillInput
	physicalOK    bool
	magicInput    formulas.MagicDamageInput
	magicOK       bool
	// magicFailures records the switch the last MagicDamageInput call saw.
	magicFailures  *bool
	skillSuccessOK bool
	// skillSuccessChance overrides SkillSuccessInput's BaseChance; nil keeps
	// the default guaranteed-success 100, a pointer to 0 forces a
	// deterministic effect-landing failure regardless of shield/rnd.
	skillSuccessChance *float64
	lastShield         formulas.ShieldDefense
	blowInput          formulas.BlowInput
	blowOK             bool
	manaInput          formulas.ManaDamageInput
	manaOK             bool
	lethalInput        formulas.LethalInput
	lethalOK           bool
	lethalPlayer       bool

	raidRelated  bool
	lethalImmune bool

	lethalOutcomes []formulas.LethalOutcome

	effects *effect.List
	shots   []item.ShotKind
	// shotFlags parallels shots with the charged flag each write carried.
	shotFlags []bool
	charged   map[item.ShotKind]bool

	castBreakDamage []float64
	// hitLog records, in order, an early cast-break roll and the HP loss
	// that follows it, with the effects the target held at that moment.
	hitLog []string

	isPlayer     bool
	name         string
	noticeKind   string
	noticeName   string
	noticeAmount int
	noticeOther  bool

	// reflects makes every reflectable skill bounce off this target.
	reflects bool
	// resistNotices records the S1_RESISTED_YOUR_S2 notices sent to this
	// actor directly rather than through a cast Result.
	resistNotices []Resisted
}

func (t *skillTarget) SkillReflectInput(modelskill.Definition) formulas.SkillReflectInput {
	if !t.reflects {
		return formulas.SkillReflectInput{}
	}
	return formulas.SkillReflectInput{CanBeReflected: true, Magic: true, ReflectChance: 100}
}

func (t *skillTarget) NotifyResistedSkill(name string, id modelskill.ID, level int) {
	t.resistNotices = append(t.resistNotices, Resisted{TargetName: name, SkillID: id, SkillLevel: level})
}

func (t *skillTarget) BreakCastOnDamage(damage float64) {
	t.castBreakDamage = append(t.castBreakDamage, damage)
	t.hitLog = append(t.hitLog, "cast break")
}

func (t *skillTarget) ReduceHPWithoutCastBreak(v float64, _ attackable.Combatant, _ modelskill.Definition) {
	t.hitLog = append(t.hitLog, fmt.Sprintf("hp -%v with %d effects", v, len(t.effects.All())))
	t.hp -= v
}

func (t *skillTarget) EffectList() *effect.List { return t.effects }

func (t *skillTarget) AlikeDead() bool { return t.dead || t.alikeDead }
func (t *skillTarget) Dead() bool      { return t.dead }

func (t *skillTarget) Invulnerable() bool { return t.invulnerable }

func (t *skillTarget) CursedWeaponEquipped() bool { return t.cursed }

func (t *skillTarget) RaidRelated() bool { return t.raidRelated }

func (t *skillTarget) Lethalable() bool { return !t.lethalImmune }

func (t *skillTarget) CanBeHealed() bool {
	return !t.dead && !t.invulnerable && !t.cursed
}

func (t *skillTarget) IsPlayer() bool { return t.isPlayer }

// Kind follows the fake's isPlayer flag, so the player-only handler paths
// resolve exactly when the test asks for a player.
func (t *skillTarget) Kind() actor.Kind {
	if t.isPlayer {
		return actor.KindPlayer
	}
	return actor.KindNPC
}

func (t *skillTarget) CharacterName() string { return t.name }
func (t *skillTarget) NotifyHPRestored(name string, amount int, other bool) {
	t.noticeKind, t.noticeName, t.noticeAmount, t.noticeOther = "hp", name, amount, other
}

func (t *skillTarget) NotifyMPRestored(name string, amount int, other bool) {
	t.noticeKind, t.noticeName, t.noticeAmount, t.noticeOther = "mp", name, amount, other
}

func (t *skillTarget) NotifyCPRestored(name string, amount int, other bool) {
	t.noticeKind, t.noticeName, t.noticeAmount, t.noticeOther = "cp", name, amount, other
}

// HealInput resolves healAmount as the power term; healMAtk and
// healScaling feed the spiritshot terms.
func (t *skillTarget) HealInput(skill modelskill.Definition) (formulas.HealInput, bool) {
	return formulas.HealInput{
		Power:   t.healAmount,
		Static:  skillTypeKey(skill.SkillType) == "HEAL_STATIC",
		MAtk:    t.healMAtk,
		Scaling: t.healScaling,
	}, t.healOK
}

func (t *skillTarget) SpiritshotCharged() bool { return t.charged[item.ShotSpirit] }

func (t *skillTarget) BlessedSpiritshotCharged() bool { return t.charged[item.ShotBlessedSpirit] }

func (t *skillTarget) HealEffectiveness() float64 {
	if t.healEffectiveness == 0 {
		return 100
	}
	return t.healEffectiveness
}

func (t *skillTarget) HP() float64         { return t.hp }
func (t *skillTarget) MaxHPValue() float64 { return t.maxHP }

func (t *skillTarget) SetHP(v float64) { t.hp = v }

func (t *skillTarget) AddHP(v float64) float64 {
	if t.effects != nil {
		t.effectsAtHeal = len(t.effects.All())
	}
	if t.hp+v > t.maxHP {
		v = t.maxHP - t.hp
	}
	if v == 0 {
		return 0
	}
	t.hp += v
	return v
}

func (t *skillTarget) MaxMPValue() float64 { return t.maxMP }
func (t *skillTarget) MPValue() float64    { return t.mp }

func (t *skillTarget) AddMP(v float64) float64 {
	if t.mp+v > t.maxMP {
		v = t.maxMP - t.mp
	}
	if v == 0 {
		return 0
	}
	t.mp += v
	return v
}

func (t *skillTarget) ReduceMP(v float64) float64 {
	if t.mp-v < 0 {
		v = t.mp
	}
	if v == 0 {
		return 0
	}
	t.mp -= v
	return v
}

func (t *skillTarget) RechargeMP(v float64) float64 { return v * t.recharge }
func (t *skillTarget) BroadcastStatus()             {}

func (t *skillTarget) CP() float64         { return t.cp }
func (t *skillTarget) MaxCPValue() float64 { return t.maxCP }
func (t *skillTarget) SetCP(v float64) {
	if v < 0 {
		v = 0
	}
	if v > t.maxCP {
		v = t.maxCP
	}
	t.cp = v
}

func (t *skillTarget) AddExpAndSp(_ int64, sp int) { t.sp += sp }

func (t *skillTarget) Die(killer attackable.Combatant) {
	t.dead = true
	t.diedBy = killer
}

func (t *skillTarget) ReduceHP(v float64, attacker attackable.Combatant, skill modelskill.Definition) {
	t.hp -= v
}

func (t *skillTarget) SetChargedShot(kind item.ShotKind, charged bool) {
	t.shots = append(t.shots, kind)
	t.shotFlags = append(t.shotFlags, charged)
}

func (t *skillTarget) ChargedShot(kind item.ShotKind) bool { return t.charged[kind] }

func (t *skillTarget) PhysicalSkillInput(caster creature.FormulaActor, skill modelskill.Definition) (formulas.PhysicalSkillInput, bool) {
	return t.physicalInput, t.physicalOK
}

func (t *skillTarget) MagicDamageInput(caster creature.FormulaActor, skill modelskill.Definition, magicFailures bool) (formulas.MagicDamageInput, bool) {
	t.magicFailures = &magicFailures
	return t.magicInput, t.magicOK
}

func (t *skillTarget) SkillSuccessInput(_ creature.FormulaActor, _ modelskill.Definition, _ bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	t.lastShield = shield
	chance := 100.0
	if t.skillSuccessChance != nil {
		chance = *t.skillSuccessChance
	}
	return formulas.SkillSuccessInput{IgnoreResists: true, BaseChance: chance, Shield: shield}, t.skillSuccessOK
}

func (t *skillTarget) BlowInput(caster creature.FormulaActor, skill modelskill.Definition) (formulas.BlowInput, bool) {
	return t.blowInput, t.blowOK
}

func (t *skillTarget) ManaDamageInput(caster creature.FormulaActor, skill modelskill.Definition) (formulas.ManaDamageInput, bool) {
	return t.manaInput, t.manaOK
}

func (t *skillTarget) LethalInput(caster creature.FormulaActor, skill modelskill.Definition) (formulas.LethalInput, bool) {
	in := t.lethalInput
	in.Chance1 = skill.LethalChance1
	in.Chance2 = skill.LethalChance2
	in.MagicLevel = skill.MagicLevel
	return in, t.lethalOK
}

func (t *skillTarget) ApplyLethalOutcome(outcome formulas.LethalOutcome, caster attackable.Combatant, skill modelskill.Definition) {
	t.lethalOutcomes = append(t.lethalOutcomes, outcome)
	switch outcome {
	case formulas.LethalFull:
		t.hp = 1
		if t.lethalPlayer {
			t.cp = 1
		}
	case formulas.LethalHalf:
		if t.lethalPlayer {
			t.cp = 1
		} else {
			t.hp -= t.hp / 2
		}
	}
}

func almost(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

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

// playerActor marks a skillTarget as a world player, satisfying
// worldPlayerTarget so it is treated like a Player.Character for
// player-gated system messages.
type playerActor struct{ skillTarget }

func (*playerActor) Kind() actor.Kind { return actor.KindPlayer }

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

// resistedIconTemplate is an effect template whose landing roll always
// resists: skillTarget implements skillSuccessSource (the skill's own
// effect-success roll, controlled by skillSuccessChance) but not
// effectSuccessSource (the per-template roll inside applyEffectsWithLanding),
// so any EffectPowerSet template with an icon is force-counted as resisted
// there regardless of rnd.
var resistedIconTemplate = []modelskill.EffectTemplate{{Name: "Buff", Time: 10, EffectPowerSet: true, EffectPower: 100, Icon: true}}

func chanceOf(v float64) *float64 { return &v }

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

func TestPdamAndMdamDischargeTheirChargedShots(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{}
	target := &skillTarget{
		hp: 1000,
		physicalInput: formulas.PhysicalSkillInput{
			AttackPower: 100, SkillPower: 50, Defence: 50,
			RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, ElementalMul: 1,
		},
		physicalOK: true,
		magicInput: formulas.MagicDamageInput{
			MAtk: 100, MDef: 50, SkillPower: 20, PvPMul: 1, ElementalMul: 1,
		},
		magicOK: true,
	}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "PDAM"}, Targets: []Actor{target}})
	caster.charged = map[item.ShotKind]bool{item.ShotBlessedSpirit: true}
	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "MDAM"}, Targets: []Actor{target}})

	if got, want := caster.shots, []item.ShotKind{item.ShotSoul, item.ShotBlessedSpirit}; !slices.Equal(got, want) {
		t.Fatalf("discharged shots = %v, want %v", got, want)
	}
}

// TestNPCCasterSpendsItsSpiritshot drives HEAL and MDAM from a real
// *npc.Hostile, which exposes its charge through SpiritshotCharged and has
// no ChargedShot: the cast spends the NPC's spiritshot, and a static-reuse
// cast writes the bit back set, as Npc.setChargedShot does for any caster.
func TestNPCCasterSpendsItsSpiritshot(t *testing.T) {
	magicTarget := func() *skillTarget {
		return &skillTarget{
			hp:         1000,
			magicInput: formulas.MagicDamageInput{MAtk: 100, MDef: 50, SkillPower: 20, PvPMul: 1, ElementalMul: 1},
			magicOK:    true,
		}
	}
	for _, tc := range []struct {
		name        string
		skill       modelskill.Definition
		charged     bool
		target      func() *skillTarget
		wantCharged bool
	}{
		{"heal spends the charge", modelskill.Definition{SkillType: "HEAL", Power: 20}, true, func() *skillTarget {
			return &skillTarget{hp: 10, maxHP: 1000, healEffectiveness: 100}
		}, false},
		{"mdam spends the charge", modelskill.Definition{SkillType: "MDAM", Power: 20}, true, magicTarget, false},
		{"static-reuse heal writes the charge", modelskill.Definition{SkillType: "HEAL", Power: 20, StaticReuse: true}, false, func() *skillTarget {
			return &skillTarget{hp: 10, maxHP: 1000, healEffectiveness: 100}
		}, true},
		{"static-reuse mdam writes the charge", modelskill.Definition{SkillType: "MDAM", Power: 20, StaticReuse: true}, false, magicTarget, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caster := newTestHostile(t, 20001, 0)
			caster.SetChargedShot(item.ShotSpirit, tc.charged)
			target := tc.target()
			if !NewDefaultRegistry().Use(Cast{Caster: caster, Skill: tc.skill, Targets: []Actor{target}}) {
				t.Fatalf("Use() returned false for %s", tc.skill.SkillType)
			}
			if got := caster.SpiritshotCharged(); got != tc.wantCharged {
				t.Fatalf("NPC spiritshot charged = %v after %s, want %v", got, tc.name, tc.wantCharged)
			}
		})
	}
}

// TestNonDamageHandlersDischargeChargedShots pins the shot each non-damage
// handler spends after its target loop, and the static-reuse flag it writes
// back: CPDAMPERCENT spends the soulshot; HEAL, MANAHEAL, RESURRECT and the
// CANCEL family spend the blessed spiritshot when one is charged, otherwise
// the plain spiritshot; a heal also spends it in its BUFF pass first, so a
// static heal spends it once there and a potion leaves it alone.
// The continuous handler spends the spiritshot on every cast but a potion or
// a toggle; the disablers handler spends it unconditionally.
func TestNonDamageHandlersDischargeChargedShots(t *testing.T) {
	healTarget := func() *skillTarget { return &skillTarget{hp: 10, maxHP: 100, mp: 10, maxMP: 100, recharge: 1} }
	tests := []struct {
		name      string
		skill     modelskill.Definition
		blessed   bool
		healOK    bool
		cubic     bool
		targets   func() []Actor
		wantShots []item.ShotKind
	}{
		{
			name:      "cpdampercent with no targets",
			skill:     modelskill.Definition{SkillType: "CPDAMPERCENT", Power: 50},
			targets:   func() []Actor { return nil },
			wantShots: []item.ShotKind{item.ShotSoul},
		},
		{
			name:      "cpdampercent skips a non-player target",
			skill:     modelskill.Definition{SkillType: "CPDAMPERCENT", Power: 50},
			blessed:   true,
			targets:   func() []Actor { return []Actor{&skillTarget{cp: 100, maxCP: 100}} },
			wantShots: []item.ShotKind{item.ShotSoul},
		},
		{
			name:      "cpdampercent static reuse",
			skill:     modelskill.Definition{SkillType: "CPDAMPERCENT", Power: 50, StaticReuse: true},
			targets:   func() []Actor { return []Actor{&skillTarget{isPlayer: true, cp: 100, maxCP: 100}} },
			wantShots: []item.ShotKind{item.ShotSoul},
		},
		{
			name:      "heal plain spiritshot",
			skill:     modelskill.Definition{SkillType: "HEAL", Power: 20},
			healOK:    true,
			targets:   func() []Actor { return []Actor{healTarget()} },
			wantShots: []item.ShotKind{item.ShotSpirit, item.ShotSpirit},
		},
		{
			name:      "heal blessed spiritshot static reuse",
			skill:     modelskill.Definition{SkillType: "HEAL", Power: 20, StaticReuse: true},
			blessed:   true,
			healOK:    true,
			targets:   func() []Actor { return []Actor{healTarget()} },
			wantShots: []item.ShotKind{item.ShotBlessedSpirit, item.ShotBlessedSpirit},
		},
		{
			name:      "heal without a resolvable amount",
			skill:     modelskill.Definition{SkillType: "HEAL", Power: 20},
			targets:   func() []Actor { return []Actor{healTarget()} },
			wantShots: []item.ShotKind{item.ShotSpirit, item.ShotSpirit},
		},
		{
			name:      "heal static spends only in the buff pass",
			skill:     modelskill.Definition{SkillType: "HEAL_STATIC", Power: 20},
			blessed:   true,
			healOK:    true,
			targets:   func() []Actor { return []Actor{healTarget()} },
			wantShots: []item.ShotKind{item.ShotBlessedSpirit},
		},
		{
			name:    "heal potion keeps shot",
			skill:   modelskill.Definition{SkillType: "HEAL", Power: 20, Potion: true},
			healOK:  true,
			targets: func() []Actor { return []Actor{healTarget()} },
		},
		{
			name:      "manaheal plain spiritshot",
			skill:     modelskill.Definition{SkillType: "MANAHEAL", Power: 20},
			targets:   func() []Actor { return []Actor{healTarget()} },
			wantShots: []item.ShotKind{item.ShotSpirit},
		},
		{
			name:      "manarecharge blessed spiritshot",
			skill:     modelskill.Definition{SkillType: "MANARECHARGE", Power: 20, StaticReuse: true},
			blessed:   true,
			targets:   func() []Actor { return []Actor{healTarget()} },
			wantShots: []item.ShotKind{item.ShotBlessedSpirit},
		},
		{
			name:    "manaheal potion keeps shot",
			skill:   modelskill.Definition{SkillType: "MANAHEAL", Power: 20, Potion: true},
			blessed: true,
			targets: func() []Actor { return []Actor{healTarget()} },
		},
		{
			name:      "resurrect by a caster without revive power",
			skill:     modelskill.Definition{SkillType: "RESURRECT", Power: 20},
			blessed:   true,
			targets:   func() []Actor { return nil },
			wantShots: []item.ShotKind{item.ShotBlessedSpirit},
		},
		{
			name:      "cancel with no targets",
			skill:     modelskill.Definition{SkillType: "CANCEL", Power: 20},
			targets:   func() []Actor { return nil },
			wantShots: []item.ShotKind{item.ShotSpirit},
		},
		{
			name:      "mage bane skips a dead target",
			skill:     modelskill.Definition{SkillType: "MAGE_BANE", Power: 20, StaticReuse: true},
			blessed:   true,
			targets:   func() []Actor { return []Actor{&skillTarget{dead: true}} },
			wantShots: []item.ShotKind{item.ShotBlessedSpirit},
		},
		{
			name:      "warrior bane with no targets",
			skill:     modelskill.Definition{SkillType: "WARRIOR_BANE", Power: 20},
			targets:   func() []Actor { return nil },
			wantShots: []item.ShotKind{item.ShotSpirit},
		},
		{
			name:      "buff plain spiritshot",
			skill:     modelskill.Definition{SkillType: "BUFF"},
			targets:   func() []Actor { return []Actor{&skillTarget{}} },
			wantShots: []item.ShotKind{item.ShotSpirit},
		},
		{
			name:      "debuff blessed spiritshot static reuse after a failed landing",
			skill:     modelskill.Definition{SkillType: "DEBUFF", Debuff: true, Offensive: true, StaticReuse: true},
			blessed:   true,
			targets:   func() []Actor { return []Actor{&skillTarget{}} },
			wantShots: []item.ShotKind{item.ShotBlessedSpirit},
		},
		{
			name:      "hot with no targets",
			skill:     modelskill.Definition{SkillType: "HOT"},
			blessed:   true,
			targets:   func() []Actor { return nil },
			wantShots: []item.ShotKind{item.ShotBlessedSpirit},
		},
		{
			name:    "buff potion keeps shot",
			skill:   modelskill.Definition{SkillType: "BUFF", Potion: true},
			blessed: true,
			targets: func() []Actor { return []Actor{&skillTarget{}} },
		},
		{
			name:    "toggle keeps shot",
			skill:   modelskill.Definition{SkillType: "CONT", Activation: modelskill.ActivationToggle},
			targets: func() []Actor { return []Actor{&skillTarget{}} },
		},
		{
			name:      "stun plain spiritshot",
			skill:     modelskill.Definition{SkillType: "STUN", Offensive: true},
			targets:   func() []Actor { return []Actor{&skillTarget{}} },
			wantShots: []item.ShotKind{item.ShotSpirit},
		},
		{
			name:      "sleep blessed spiritshot static reuse skips a dead target",
			skill:     modelskill.Definition{SkillType: "SLEEP", Offensive: true, StaticReuse: true},
			blessed:   true,
			targets:   func() []Actor { return []Actor{&skillTarget{dead: true}} },
			wantShots: []item.ShotKind{item.ShotBlessedSpirit},
		},
		{
			name:    "cubic poison keeps the owner's shot",
			skill:   modelskill.Definition{SkillType: "POISON", Debuff: true, Offensive: true},
			blessed: true,
			cubic:   true,
			targets: func() []Actor { return []Actor{&skillTarget{}} },
		},
		{
			name:    "cubic stun keeps the owner's shot",
			skill:   modelskill.Definition{SkillType: "STUN", Offensive: true},
			blessed: true,
			cubic:   true,
			targets: func() []Actor { return []Actor{&skillTarget{}} },
		},
		{
			name:      "disabler potion still spends",
			skill:     modelskill.Definition{SkillType: "CANCEL_DEBUFF", Potion: true},
			blessed:   true,
			targets:   func() []Actor { return nil },
			wantShots: []item.ShotKind{item.ShotBlessedSpirit},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caster := &skillTarget{
				healAmount: 10, healOK: tt.healOK,
				charged: map[item.ShotKind]bool{item.ShotBlessedSpirit: tt.blessed, item.ShotSpirit: !tt.blessed, item.ShotSoul: true},
			}
			NewDefaultRegistry().Use(Cast{Caster: caster, Skill: tt.skill, Targets: tt.targets(), Cubic: tt.cubic})
			if !slices.Equal(caster.shots, tt.wantShots) {
				t.Fatalf("discharged shots = %v, want %v", caster.shots, tt.wantShots)
			}
			for i, flag := range caster.shotFlags {
				if flag != tt.skill.StaticReuse {
					t.Fatalf("shot write %d charged = %v, want static-reuse flag %v", i, flag, tt.skill.StaticReuse)
				}
			}
		})
	}
}

// TestCpDamPercentAlikeDeadCasterKeepsSoulshot pins the one early exit that
// precedes the soulshot discharge.
func TestCpDamPercentAlikeDeadCasterKeepsSoulshot(t *testing.T) {
	caster := &skillTarget{alikeDead: true}
	NewDefaultRegistry().Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "CPDAMPERCENT", Power: 50}})
	if len(caster.shots) != 0 {
		t.Fatalf("discharged shots = %v, want none from an alike-dead caster", caster.shots)
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

// ---- from manor_test.go ----
type manorFakeTarget struct {
	neutralNPC
	world.Presence
	fakeActor
	dead  bool
	level int
	state *npc.SeedState
}

func (m *manorFakeTarget) Kind() actor.Kind          { return actor.KindNPC }
func (m *manorFakeTarget) Dead() bool                { return m.dead }
func (m *manorFakeTarget) Level() int                { return m.level }
func (m *manorFakeTarget) SeedState() *npc.SeedState { return m.state }

// sownState is a seed state already sown by sowerID, carrying a crop that
// matures into matureID.
func sownState(sowerID int32, matureID int) *npc.SeedState {
	state := &npc.SeedState{}
	state.Sow(sowerID, manor.Seed{MatureID: matureID})
	return state
}

type manorFakeItem struct {
	seed manor.Seed
	ok   bool
}

func (i manorFakeItem) Seed() (manor.Seed, bool) { return i.seed, i.ok }

type manorFakeCaster struct {
	neutralCreature
	world.Presence
	fakeActor
	id    int32
	level int
	items map[int32]int
}

func (c *manorFakeCaster) ObjectID() int32 { return c.id }
func (c *manorFakeCaster) Level() int      { return c.level }
func (c *manorFakeCaster) AddEarnedItem(itemID int32, count int) {
	if c.items == nil {
		c.items = make(map[int32]int)
	}
	c.items[itemID] += count
}

func TestSowEventuallySucceedsAndMarksSeeded(t *testing.T) {
	// Seed/target/player levels all equal give a 90% sow success rate — not
	// a certainty, so the roll can't be forced deterministically. Retrying
	// drives the false-negative chance for this assertion to effectively
	// zero (0.1^300) without depending on a specific random outcome.
	registry := NewDefaultRegistry()
	caster := &manorFakeCaster{id: 7, level: 40}
	item := manorFakeItem{seed: manor.Seed{Level: 40, Alternative: false}, ok: true}

	for i := 0; i < 300; i++ {
		target := &manorFakeTarget{level: 40, state: &npc.SeedState{}}
		if !registry.Use(Cast{
			Caster:  caster,
			Item:    item,
			Skill:   modelskill.Definition{SkillType: "SOW"},
			Targets: []Actor{target},
		}) {
			t.Fatal("Use() returned false for SOW")
		}
		if target.state.Seeded() {
			if !target.state.AllowedToHarvest(7) {
				t.Fatal("sown state does not record the casting player as its sower")
			}
			return
		}
	}
	t.Fatal("SOW never succeeded in 300 attempts at a 90% success rate")
}

func TestSowAlreadySeededIsNoop(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &manorFakeCaster{id: 7, level: 40}
	target := &manorFakeTarget{level: 40, state: sownState(3, 0)}
	item := manorFakeItem{seed: manor.Seed{Level: 40}, ok: true}

	registry.Use(Cast{Caster: caster, Item: item, Skill: modelskill.Definition{SkillType: "SOW"}, Targets: []Actor{target}})
	if !target.state.AllowedToHarvest(3) {
		t.Fatal("already-seeded target should keep its original sower")
	}
}

func TestHarvestRewardsAllowedHarvester(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &manorFakeCaster{id: 7, level: 40}
	target := &manorFakeTarget{level: 40, state: sownState(7, 5001)}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "HARVEST"}, Targets: []Actor{target}})

	if !target.state.Harvested() {
		t.Error("target should be marked harvested")
	}
	// Assert against the crop the seed state itself reports rather than a
	// literal, so this still fails if the handler stops threading the count
	// through and survives #240 changing what the count is.
	wantID, wantCount := target.state.HarvestedCrop()
	if wantID != 5001 {
		t.Fatalf("sown state crop id = %d, want 5001", wantID)
	}
	if caster.items[wantID] != wantCount {
		t.Fatalf("caster earned items = %v, want {%d: %d}", caster.items, wantID, wantCount)
	}
}

func TestHarvestDisallowedHarvesterGetsNothing(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &manorFakeCaster{id: 7, level: 40}
	target := &manorFakeTarget{level: 40, state: sownState(3, 5001)}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "HARVEST"}, Targets: []Actor{target}})

	if target.state.Harvested() {
		t.Error("a disallowed harvester should not mark the target harvested")
	}
	if len(caster.items) != 0 {
		t.Fatalf("caster earned items = %v, want none", caster.items)
	}
}

func TestHarvestAlreadyHarvestedIsNoop(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &manorFakeCaster{id: 7, level: 40}
	target := &manorFakeTarget{level: 40, state: sownState(7, 5001)}
	target.state.MarkHarvested()

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "HARVEST"}, Targets: []Actor{target}})
	if len(caster.items) != 0 {
		t.Fatalf("caster earned items = %v, want none", caster.items)
	}
}

// ---- from resurrect_test.go ----
type reviveFakeCaster struct {
	neutralCreature
	world.Presence
	fakeActor
	kind    actor.Kind
	wit     int
	refused []event.ReviveRefusal
}

func (c *reviveFakeCaster) WIT() int              { return c.wit }
func (c *reviveFakeCaster) Kind() actor.Kind      { return c.kind }
func (c *reviveFakeCaster) CharacterName() string { return "Healer" }
func (c *reviveFakeCaster) NotifyReviveRefused(r event.ReviveRefusal) {
	c.refused = append(c.refused, r)
}

type reviveFakeTarget struct {
	world.Presence
	neutralPlayer
	fakeActor
	restoredPercent float64
	offers          []reviveOffer
}

type reviveOffer struct {
	reviver string
	power   float64
	isPet   bool
}

func (t *reviveFakeTarget) ReviveRestoringExp(restorePercent float64) bool {
	t.restoredPercent = restorePercent
	return true
}

func (t *reviveFakeTarget) ReviveRequest(reviver player.Reviver, power float64, isPet bool) {
	t.offers = append(t.offers, reviveOffer{reviver.CharacterName(), power, isPet})
}
func (*reviveFakeTarget) Kind() actor.Kind { return actor.KindPlayer }

// TestResurrectByPlayerOffersEveryTarget: a player caster's resurrection
// asks each dead player first, carrying the WIT-scaled revive power, and
// revives nobody outright.
func TestResurrectByPlayerOffersEveryTarget(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &reviveFakeCaster{kind: actor.KindPlayer, wit: 30}
	a := &reviveFakeTarget{}
	b := &reviveFakeTarget{}

	if !registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "RESURRECT", Power: 40},
		Targets: []Actor{a, b, &placedFakeActor{}},
	}) {
		t.Fatal("Use() returned false for RESURRECT")
	}

	want := reviveOffer{"Healer", formulas.RevivePower(statbonus.WITBonus[30], 40), false}
	for i, target := range []*reviveFakeTarget{a, b} {
		if len(target.offers) != 1 || target.offers[0] != want {
			t.Fatalf("target %d offers = %+v, want [%+v]", i, target.offers, want)
		}
		if target.restoredPercent != 0 {
			t.Fatalf("target %d revived outright at %v, want only the offer", i, target.restoredPercent)
		}
	}
}

// TestResurrectByNonPlayerRevivesOutright: any other caster revives a dead
// player without asking, restoring the revive power's share of lost exp.
func TestResurrectByNonPlayerRevivesOutright(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &reviveFakeCaster{kind: actor.KindNPC, wit: 30}
	a := &reviveFakeTarget{}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "RESURRECT", Power: 40},
		Targets: []Actor{a},
	})
	if want := formulas.RevivePower(statbonus.WITBonus[30], 40); a.restoredPercent != want || len(a.offers) != 0 {
		t.Fatalf("restore percent = %v, offers = %+v; want %v and no offer", a.restoredPercent, a.offers, want)
	}
}

// reviveFakeSummon records how a resurrection reached a summon.
type reviveFakeSummon struct {
	world.Presence
	fakeActor
	outright []float64
	direct   []float64
}

func (*reviveFakeSummon) Kind() actor.Kind { return actor.KindSummon }
func (s *reviveFakeSummon) ResurrectOutright(power float64) {
	s.outright = append(s.outright, power)
}

func (s *reviveFakeSummon) ReviveRestoringExp(power float64) bool {
	s.direct = append(s.direct, power)
	return true
}

// TestResurrectByNonPlayerRevivesSummonOnItsQueue: any other caster hands a
// dead summon its revive, at the WIT-scaled power, through the summon's own
// queued resurrection (which drops the decay first), and asks nobody.
func TestResurrectByNonPlayerRevivesSummonOnItsQueue(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &reviveFakeCaster{kind: actor.KindNPC, wit: 30}
	s := &reviveFakeSummon{}
	p := &reviveFakeTarget{}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "RESURRECT", Power: 40},
		Targets: []Actor{s, p},
	})
	want := formulas.RevivePower(statbonus.WITBonus[30], 40)
	if len(s.outright) != 1 || s.outright[0] != want {
		t.Fatalf("summon outright resurrections = %v, want [%v]", s.outright, want)
	}
	if len(s.direct) != 0 {
		t.Fatalf("summon revived off its queue at %v, want only the queued resurrection", s.direct)
	}
	if len(p.offers) != 0 || p.restoredPercent != want {
		t.Fatalf("player offers = %+v, restore percent = %v; want no offer and %v", p.offers, p.restoredPercent, want)
	}
}

func TestResurrectWithoutCasterIsNoop(t *testing.T) {
	registry := NewDefaultRegistry()
	a := &reviveFakeTarget{}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "RESURRECT"},
		Targets: []Actor{a},
	})
	if a.restoredPercent != 0 || len(a.offers) != 0 {
		t.Fatalf("restore percent = %v, offers = %+v; want untouched", a.restoredPercent, a.offers)
	}
}

// ---- from seed_test.go ----
func seedOfFire() modelskill.Definition {
	return modelskill.Definition{
		ID:        1285,
		Level:     1,
		SkillType: "SEED",
		Effects:   []modelskill.EffectTemplate{{Name: "Seed", Time: 5}},
	}
}

func TestSeedHandlerAppliesFreshEffectWhenTargetHasNone(t *testing.T) {
	target := newContinuousFake(1)
	cast := Cast{Caster: newContinuousFake(2), Skill: seedOfFire(), Targets: []Actor{target}}

	seedHandler{}.Use(cast)

	e := firstEffectByID(target.list, 1285)
	if e == nil {
		t.Fatal("no seed effect applied to a target with no prior seed")
	}
	if e.Level != 1 {
		t.Fatalf("fresh seed effect Level = %d, want 1", e.Level)
	}
}

func TestSeedHandlerRecastGrowsExistingEffectInPlaceInsteadOfDuplicating(t *testing.T) {
	target := newContinuousFake(1)
	cast := Cast{Caster: newContinuousFake(2), Skill: seedOfFire(), Targets: []Actor{target}}

	seedHandler{}.Use(cast)
	first := firstEffectByID(target.list, 1285)

	seedHandler{}.Use(cast)
	second := firstEffectByID(target.list, 1285)

	if second != first {
		t.Fatal("recasting the same seed skill must grow the existing instance, not replace it")
	}
	if second.Level != 2 {
		t.Fatalf("Level after recast = %d, want 2", second.Level)
	}
	if len(target.list.All()) != 1 {
		t.Fatalf("effect list has %d effects, want exactly 1 (no duplicate seed)", len(target.list.All()))
	}
}

// Recasting one seed skill must not disturb an unrelated seed already
// active on the same target — no reschedule, no level change. That the
// recast also leaves the recast seed's own deadline unmoved is verified in
// the effect package's own tests, which have access to the unexported
// schedule state.
func TestSeedHandlerRecastLeavesOtherActiveSeedsInPlace(t *testing.T) {
	target := newContinuousFake(1)
	fire := Cast{Caster: newContinuousFake(2), Skill: seedOfFire(), Targets: []Actor{target}}
	water := Cast{Caster: fire.Caster, Skill: modelskill.Definition{
		ID: 1286, Level: 1, SkillType: "SEED",
		Effects: []modelskill.EffectTemplate{{Name: "Seed", Time: 5}},
	}, Targets: []Actor{target}}

	seedHandler{}.Use(fire)
	seedHandler{}.Use(water)
	seedHandler{}.Use(fire)

	waterEffect := firstEffectByID(target.list, 1286)
	if waterEffect == nil {
		t.Fatal("recasting fire must not remove the unrelated water seed")
	}
	if waterEffect.Level != 1 {
		t.Fatalf("water seed Level = %d, want unchanged 1", waterEffect.Level)
	}
}

// ---- from spoil_test.go ----
type spoilFakeTarget struct {
	world.Presence
	neutralNPC
	fakeActor
	dead  bool
	level int
	pool  *item.SpoilPool
}

func (*spoilFakeTarget) Kind() actor.Kind             { return actor.KindNPC }
func (s *spoilFakeTarget) Dead() bool                 { return s.dead }
func (s *spoilFakeTarget) Level() int                 { return s.level }
func (s *spoilFakeTarget) SpoilPool() *item.SpoilPool { return s.pool }

type spoilFakeCaster struct {
	neutralPlayer
	world.Presence
	fakeActor
	id             int32
	level          int
	inParty        bool
	items          map[int32]int
	distItem       int32
	distCnt        int32
	alreadyNotices int
	notices        []string
	resisted       []Resisted
}

func (c *spoilFakeCaster) ObjectID() int32 { return c.id }
func (*spoilFakeCaster) Kind() actor.Kind  { return actor.KindPlayer }
func (c *spoilFakeCaster) Level() int      { return c.level }
func (c *spoilFakeCaster) AddEarnedItem(itemID int32, count int) {
	if c.items == nil {
		c.items = make(map[int32]int)
	}
	c.items[itemID] += count
}
func (c *spoilFakeCaster) InParty() bool { return c.inParty }
func (c *spoilFakeCaster) DistributeItem(itemID, count int32) {
	c.distItem, c.distCnt = itemID, count
}

func (c *spoilFakeCaster) NotifySpoilAlready() {
	c.alreadyNotices++
	c.notices = append(c.notices, "already")
}
func (c *spoilFakeCaster) NotifySpoilSuccess() { c.notices = append(c.notices, "success") }
func (c *spoilFakeCaster) NotifyResistedSkill(name string, id modelskill.ID, level int) {
	c.notices = append(c.notices, "resisted")
	c.resisted = append(c.resisted, Resisted{TargetName: name, SkillID: id, SkillLevel: level})
}

func TestSpoilPreservesPerTargetNoticeOrder(t *testing.T) {
	rolls := []int{0, 9999} // failed roll, then successful roll
	registry := NewRegistry(spoilHandler{roll: func(int) int {
		roll := rolls[0]
		rolls = rolls[1:]
		return roll
	}})
	caster := &spoilFakeCaster{id: 42, level: 1}
	failed := &spoilFakeTarget{level: 100, pool: &item.SpoilPool{}}
	succeeded := &spoilFakeTarget{level: 1, pool: &item.SpoilPool{}}
	result, ok := registry.UseResult(Cast{
		Caster: caster, Skill: modelskill.Definition{ID: 302, Level: 7, SkillType: "SPOIL", MagicLevel: 1},
		Targets: []Actor{failed, succeeded},
	})
	if !ok || failed.pool.IsSpoiled() || !succeeded.pool.IsSpoiled() {
		t.Fatalf("UseResult() handled = %t, spoiled = %t/%t; want false/true", ok, failed.pool.IsSpoiled(), succeeded.pool.IsSpoiled())
	}
	// The network delivers Result messages only after the handler returns.
	for _, message := range result.Messages {
		if _, ok := message.(Resisted); !ok {
			t.Fatalf("unexpected deferred message %T", message)
		}
		caster.notices = append(caster.notices, "resisted")
	}
	if !slices.Equal(caster.notices, []string{"resisted", "success"}) {
		t.Fatalf("caster notices = %v, want resisted then success", caster.notices)
	}
}

func TestSpoilEventuallyMarksTarget(t *testing.T) {
	// Level-equal caster/target still carries a real magic-resist chance
	// (never exactly 100%), so retry instead of asserting a single roll.
	registry := NewDefaultRegistry()
	caster := &spoilFakeCaster{id: 42, level: 40}

	for i := 0; i < 300; i++ {
		target := &spoilFakeTarget{level: 40, pool: &item.SpoilPool{}}
		registry.Use(Cast{
			Caster:  caster,
			Skill:   modelskill.Definition{SkillType: "SPOIL", MagicLevel: 40},
			Targets: []Actor{target},
		})
		if target.pool.IsSpoiled() {
			if !target.pool.IsSpoiler(42) {
				t.Fatal("spoiled pool should be marked by the caster")
			}
			return
		}
	}
	t.Fatal("SPOIL never succeeded in 300 attempts")
}

func TestSpoilReportsFailedMagicRollAtLevelOne(t *testing.T) {
	registry := NewRegistry(spoilHandler{roll: func(int) int { return 0 }})
	caster := &spoilFakeCaster{id: 42, level: 1}
	def := modelskill.Definition{ID: 254, Level: 7, SkillType: "SPOIL", MagicLevel: 1}
	target := &spoilFakeTarget{level: 100, pool: &item.SpoilPool{}}
	result, ok := registry.UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
	if !ok || target.pool.IsSpoiled() {
		t.Fatalf("UseResult() handled = %t, spoiled = %t; want failed SPOIL", ok, target.pool.IsSpoiled())
	}
	if len(caster.resisted) != 1 || caster.resisted[0] != (Resisted{SkillID: 254, SkillLevel: 1}) || len(result.Resisted) != 0 {
		t.Fatalf("direct resists = %+v, deferred resists = %+v; want one direct level-1 report", caster.resisted, result.Resisted)
	}
}

func TestSpoilAlreadySpoiledIsSkipped(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &spoilFakeCaster{id: 42, level: 40}
	target := &spoilFakeTarget{level: 40, pool: &item.SpoilPool{}}
	target.pool.Mark(99)

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "SPOIL", MagicLevel: 40}, Targets: []Actor{target}})
	if !target.pool.IsSpoiler(99) {
		t.Fatal("an already-spoiled pool should keep its original spoiler")
	}
	if caster.alreadyNotices != 1 {
		t.Fatalf("already-spoiled notices = %d, want 1", caster.alreadyNotices)
	}
}

func TestSweepDistributesPooledItemsAndClearsPool(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &spoilFakeCaster{id: 1}
	target := &spoilFakeTarget{pool: &item.SpoilPool{}}
	target.pool.Mark(1)
	target.pool.Add(57, 10)

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "SWEEP"}, Targets: []Actor{target}})

	if caster.items[57] != 10 {
		t.Fatalf("caster earned items = %v, want {57: 10}", caster.items)
	}
	if target.pool.IsSpoiled() || target.pool.Sweepable() {
		t.Fatal("sweeping should fully clear the pool, spoiler marker included")
	}
}

func TestSweepDistributesThroughParty(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &spoilFakeCaster{id: 1, inParty: true}
	target := &spoilFakeTarget{pool: &item.SpoilPool{}}
	target.pool.Mark(1)
	target.pool.Add(57, 10)

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "SWEEP"}, Targets: []Actor{target}})

	if caster.distItem != 57 || caster.distCnt != 10 {
		t.Fatalf("party distribution = (%d, %d), want (57, 10)", caster.distItem, caster.distCnt)
	}
	if len(caster.items) != 0 {
		t.Fatalf("caster should not also receive a direct reward: %v", caster.items)
	}
}

func TestSweepEmptyPoolIsNoop(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &spoilFakeCaster{id: 1}
	target := &spoilFakeTarget{pool: &item.SpoilPool{}}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "SWEEP"}, Targets: []Actor{target}})
	if len(caster.items) != 0 {
		t.Fatalf("nothing to sweep should reward nothing, got %v", caster.items)
	}
}

// ---- from teleport_test.go ----
type jumpFakeTarget struct {
	world.Presence
	fakeActor
	heading, x, y, z int
}

func (t *jumpFakeTarget) Heading() int              { return t.heading }
func (t *jumpFakeTarget) Position() (int, int, int) { return t.x, t.y, t.z }
func (t *jumpFakeTarget) X() int                    { return t.x }
func (t *jumpFakeTarget) Y() int                    { return t.y }
func (t *jumpFakeTarget) Z() int                    { return t.z }

type jumpFakeCaster struct {
	neutralCreature
	world.Presence
	fakeActor
	aborted     bool
	broadcasted bool
	x, y, z     int
}

func (c *jumpFakeCaster) AbortAll(force bool) { c.aborted = true }
func (c *jumpFakeCaster) SetXYZ(x, y, z int)  { c.x, c.y, c.z = x, y, z }
func (c *jumpFakeCaster) BroadcastPosition()  { c.broadcasted = true }

func TestInstantJumpRepositionsBehindTarget(t *testing.T) {
	registry := NewDefaultRegistry()
	// Heading 0 faces due "east"; +180 degrees puts the jump point due
	// west of the target, 25 units out: cos(pi) = -1, sin(pi) = 0.
	target := &jumpFakeTarget{heading: 0, x: 100, y: 100, z: 50}
	caster := &jumpFakeCaster{}

	if !registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "INSTANT_JUMP"},
		Targets: []Actor{target},
	}) {
		t.Fatal("Use() returned false for INSTANT_JUMP")
	}
	if !caster.aborted {
		t.Error("caster should abort its current action before jumping")
	}
	if !caster.broadcasted {
		t.Error("caster should broadcast its new position")
	}
	if caster.x != 75 || caster.y != 100 || caster.z != 50 {
		t.Errorf("caster position = (%d,%d,%d), want (75,100,50)", caster.x, caster.y, caster.z)
	}
}

func TestInstantJumpNoTargetsIsNoop(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &jumpFakeCaster{}
	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "INSTANT_JUMP"}})
	if caster.aborted {
		t.Error("caster should not act without a target")
	}
}

type getPlayerFakeCaster struct {
	neutralCreature
	world.Presence
	fakeActor
	x, y, z int
}

func (c *getPlayerFakeCaster) AlikeDead() bool           { return false }
func (c *getPlayerFakeCaster) Position() (int, int, int) { return c.x, c.y, c.z }

type getPlayerFakeTarget struct {
	world.Presence
	fakeActor
	dead       bool
	teleported bool
	tx, ty, tz int
}

func (t *getPlayerFakeTarget) AlikeDead() bool { return t.dead }
func (t *getPlayerFakeTarget) Dead() bool      { return t.dead }
func (t *getPlayerFakeTarget) TeleportTo(x, y, z int) {
	t.teleported = true
	t.tx, t.ty, t.tz = x, y, z
}

func TestGetPlayerPullsLivingTargetsToCaster(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &getPlayerFakeCaster{x: 1, y: 2, z: 3}
	target := &getPlayerFakeTarget{}
	deadTarget := &getPlayerFakeTarget{dead: true}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "GET_PLAYER"},
		Targets: []Actor{target, deadTarget},
	})

	if !target.teleported || target.tx != 1 || target.ty != 2 || target.tz != 3 {
		t.Fatalf("target not pulled to caster position: %+v", target)
	}
	if deadTarget.teleported {
		t.Fatal("dead target should not be teleported")
	}
}

func (*effectLandingFake) Kind() actor.Kind { return actor.KindNPC }

func (*positionedFakeActor) Kind() actor.Kind { return actor.KindNPC }

func (fakeActor) Kind() actor.Kind { return actor.KindNPC }

func (*disablerFake) Kind() actor.Kind { return actor.KindNPC }

func (disablerHostileMove) CanMoveTo(location.Location) bool { return true }

func (disablerHostileMove) MoveToLocation(location.Location) (bool, error) { return false, nil }

var (
	_ Creature = (*effectLandingFake)(nil)
	_ Creature = (*positionedFakeActor)(nil)
	_ Creature = (*damagePermissionFake)(nil)
)

// newTestList returns a list whose owner runs on its own inline queue, with
// the clock reading the wall time at creation.
func newTestList(owner effect.StatOwner) *effect.List {
	l := effect.NewList(owner)
	l.SetQueue(sim.NewInline(time.Now()).NewQueue("test"))
	return l
}

func idleQueue() *sim.Queue { return sim.NewInline(time.Unix(0, 0)).NewQueue("test") }

// guardedSkillTarget overrides the invulnerable/paralyzed state the damage
// feedback reads.
type guardedSkillTarget struct {
	*skillTarget
	invul, paralyzed bool
}

func (t *guardedSkillTarget) Invul() bool     { return t.invul }
func (t *guardedSkillTarget) Paralyzed() bool { return t.paralyzed }

type summonSkillCaster struct {
	*skillTarget
	owner int32
	pet   bool
}

func (*summonSkillCaster) Kind() actor.Kind { return actor.KindSummon }
func (s *summonSkillCaster) OwnerID() int32 { return s.owner }
func (s *summonSkillCaster) IsPet() bool    { return s.pet }

func damageMessages(t *testing.T, messages []any) []Damage {
	t.Helper()
	var out []Damage
	for _, m := range messages {
		if d, ok := m.(Damage); ok {
			out = append(out, d)
		}
	}
	return out
}

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

// TestDrainAbsorbMatchesReference pins the DRAIN absorb amount against
// literal outputs of a probe that replays L2SkillDrain.useSkill's absorb
// block (L2SkillDrain.java:60-79) with the reference's own types: int casts
// of the target's CP and HP, and absorbAbs + absorbPart * drain evaluated
// in float before widening to addHp's double. Probe run on
// eclipse-temurin:21; the want column is Double.doubleToLongBits.
func TestDrainAbsorbMatchesReference(t *testing.T) {
	for _, tc := range []struct {
		playable, targetPlayer bool
		cp, hp                 float64
		damage, abs            int
		part                   float32
		want                   uint64
	}{
		{true, false, 0, 5000, 7, 0, 0.8, 4617991057798332416},
		{true, false, 0, 5000, 123, 0, 0.8, 4636624701471326208},
		{true, false, 0, 5000, 333, 0, 0.35, 4637901893748654080},
		{true, false, 0, 5000, 1001, 0, 0.2, 4641247927749050368},
		{true, false, 0, 5000, 77, 0, 0.4, 4629362647286939648},
		{true, false, 0, 99.9, 150, 0, 0.8, 4635273621797863424},
		{true, false, 0, 5000, 50, 105, 0, 4637089135075524608},
		{true, false, 0, 30, 50, 260, 0, 4643281584563159040},
		{true, true, 100.7, 900, 60, 0, 0.8, 0},
		{true, true, 100.7, 900, 100, 0, 0.8, 0},
		{true, true, 100.7, 900, 257, 0, 0.8, 4638538731098210304},
		{true, true, 0.4, 900, 257, 0, 0.8, 4641437923680452608},
		{true, true, 0, 120.5, 257, 0, 0.8, 4636455816377925632},
		{false, true, 100.7, 900, 257, 0, 0.8, 4641437923680452608},
		{false, true, 100.7, 200.9, 257, 0, 1, 4641240890982006784},
		{false, false, 0, 8000, 4321, 0, 0.2, 4650812799516147712},
		{true, false, 0, 16777217, 16777217, 0, 1, 4715268809856909312},
		{true, false, 0, 5000, 3, 7, 0.1, 4619905087962087424},
	} {
		caster := &skillTarget{fakeActor: fakeActor{objectID: 1}, isPlayer: tc.playable}
		target := &skillTarget{fakeActor: fakeActor{objectID: 2}, isPlayer: tc.targetPlayer, cp: tc.cp, hp: tc.hp}
		cast := Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "DRAIN", AbsorbAbs: tc.abs, AbsorbPart: tc.part}}
		if got := drainAbsorb(cast, target, tc.damage); math.Float64bits(got) != tc.want {
			t.Errorf("drainAbsorb(%+v) = %v, want %v", tc, got, math.Float64frombits(tc.want))
		}
	}
}

func drainFixture() (caster, target *skillTarget, def modelskill.Definition) {
	caster = &skillTarget{
		fakeActor: fakeActor{objectID: 1}, hp: 100, maxHP: 5000, isPlayer: true, name: "Caster",
		effects: newTestList(nil), charged: map[item.ShotKind]bool{item.ShotBlessedSpirit: true},
	}
	target = &skillTarget{
		fakeActor: fakeActor{objectID: 2}, hp: 5000, maxHP: 5000, name: "Target",
		effects:    newTestList(nil),
		magicInput: formulas.MagicDamageInput{MAtk: 400, MDef: 50, SkillPower: 20, PvPMul: 1, ElementalMul: 1},
		magicOK:    true, skillSuccessOK: true,
	}
	def = modelskill.Definition{ID: 1090, Level: 1, SkillType: "DRAIN", Target: modelskill.TargetOne, AbsorbPart: 0.8}
	return caster, target, def
}

// TestDrainDamagesTargetAndFeedsCaster pins the DRAIN hit on a live target:
// the caster regains 80% of the damage, the target's cast break is rolled
// before the damage report and its effects, the HP loss comes last and
// skips a second break, and the charged blessed spiritshot is spent with
// the static-reuse flag.
func TestDrainDamagesTargetAndFeedsCaster(t *testing.T) {
	caster, target, def := drainFixture()
	def.Effects = []modelskill.EffectTemplate{{Name: "Debuff", Time: 9}}
	damage := int(formulas.MagicDamage(target.magicInput))

	result, ok := NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
	if !ok {
		t.Fatal("UseResult() handled = false, want true for DRAIN")
	}
	if want := 100 + float64(float32(0.8)*float32(damage)); caster.hp != want {
		t.Fatalf("caster hp = %v, want %v", caster.hp, want)
	}
	if want := 5000 - float64(damage); target.hp != want {
		t.Fatalf("target hp = %v, want %v", target.hp, want)
	}
	wantLog := []string{"cast break", fmt.Sprintf("hp -%v with 1 effects", float64(damage))}
	if !slices.Equal(target.hitLog, wantLog) || !slices.Equal(target.castBreakDamage, []float64{float64(damage)}) {
		t.Fatalf("target hits = %q (breaks %v), want %q", target.hitLog, target.castBreakDamage, wantLog)
	}
	if got := damageMessages(t, result.Messages); len(got) != 1 || got[0].Amount != int32(damage) || got[0].RecipientID != 1 {
		t.Fatalf("damage messages = %+v, want one %d-damage report to the caster", got, damage)
	}
	if len(result.Messages) < 2 || result.Messages[0] != any(CasterVitalsChanged{}) {
		t.Fatalf("messages = %+v, want the caster's status change ahead of its damage report", result.Messages)
	}
	if landed := target.effects.All(); len(landed) != 1 || landed[0].Effector != effect.Actor(caster) {
		t.Fatalf("target effects = %+v, want the Debuff from the caster", landed)
	}
	if !slices.Equal(caster.shots, []item.ShotKind{item.ShotBlessedSpirit}) || !slices.Equal(caster.shotFlags, []bool{false}) {
		t.Fatalf("shots = %v %v, want blessed spiritshot spent", caster.shots, caster.shotFlags)
	}
}

// TestDrainAtFullHPReportsNoCasterStatus pins the absorb's status gate: a
// caster already at full HP gains nothing and has no status to report.
func TestDrainAtFullHPReportsNoCasterStatus(t *testing.T) {
	caster, target, def := drainFixture()
	caster.hp = caster.maxHP
	result, _ := NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
	if caster.hp != caster.maxHP || slices.Contains(result.Messages, any(CasterVitalsChanged{})) {
		t.Fatalf("caster hp %v, messages %+v; want full HP and no status change", caster.hp, result.Messages)
	}
}

// TestDrainNetsAPlayerTargetsCP pins the drain basis: a playable caster
// drains only the damage past a player target's CP, and nothing when the CP
// soaks it all.
func TestDrainNetsAPlayerTargetsCP(t *testing.T) {
	for _, tc := range []struct {
		cp   float64
		want float64
	}{{10000, 100}, {100.7, 100 + float64(float32(0.8)*float32(728-100))}} {
		caster, target, def := drainFixture()
		target.isPlayer, target.cp = true, tc.cp
		NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
		if caster.hp != tc.want {
			t.Fatalf("cp %v: caster hp = %v, want %v", tc.cp, caster.hp, tc.want)
		}
	}
}

// TestDrainGates pins which targets a DRAIN skips outright and what a
// corpse drain still does.
func TestDrainGates(t *testing.T) {
	t.Run("alike-dead caster", func(t *testing.T) {
		caster, target, def := drainFixture()
		caster.alikeDead = true
		NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
		if target.hp != 5000 || caster.hp != 100 || len(caster.shots) != 0 {
			t.Fatalf("target hp %v, caster hp %v, shots %v; want nothing done", target.hp, caster.hp, caster.shots)
		}
	})
	t.Run("alike-dead target", func(t *testing.T) {
		caster, target, def := drainFixture()
		target.alikeDead = true
		NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
		if target.hp != 5000 || caster.hp != 100 {
			t.Fatalf("target hp %v, caster hp %v; want the target skipped", target.hp, caster.hp)
		}
		if !slices.Equal(caster.shots, []item.ShotKind{item.ShotBlessedSpirit}) {
			t.Fatalf("shots = %v, want the cast still spends its shot", caster.shots)
		}
	})
	t.Run("invulnerable target", func(t *testing.T) {
		caster, target, def := drainFixture()
		guarded := &guardedSkillTarget{skillTarget: target, invul: true}
		NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{guarded}})
		if target.hp != 5000 || caster.hp != 100 {
			t.Fatalf("target hp %v, caster hp %v; want the target skipped", target.hp, caster.hp)
		}
	})
	t.Run("invulnerable caster on itself", func(t *testing.T) {
		caster, target, def := drainFixture()
		target.fakeActor = caster.fakeActor
		guarded := &guardedSkillTarget{skillTarget: target, invul: true}
		NewDefaultRegistry().UseResult(Cast{Caster: guarded, Skill: def, Targets: []Actor{guarded}})
		if target.hp >= 5000 {
			t.Fatalf("self-target hp = %v, want the drain to land", target.hp)
		}
	})
	t.Run("corpse drain feeds the caster only", func(t *testing.T) {
		caster, target, def := drainFixture()
		target.dead = true
		def.Target, def.AbsorbPart, def.AbsorbAbs = modelskill.TargetCorpseMob, 0, 260
		def.Effects = []modelskill.EffectTemplate{{Name: "Debuff", Time: 9}}
		result, _ := NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
		if caster.hp != 360 {
			t.Fatalf("caster hp = %v, want 360", caster.hp)
		}
		if target.hp != 5000 || len(target.hitLog) != 0 || len(target.effects.All()) != 0 || len(damageMessages(t, result.Messages)) != 0 {
			t.Fatalf("corpse hp %v, hits %q, effects %d, messages %+v; want untouched and unreported", target.hp, target.hitLog, len(target.effects.All()), result.Messages)
		}
	})
	t.Run("corpse-mob drain on a live target lands no effects", func(t *testing.T) {
		caster, target, def := drainFixture()
		target.alikeDead = true
		def.Target = modelskill.TargetCorpseMob
		def.Effects = []modelskill.EffectTemplate{{Name: "Debuff", Time: 9}}
		result, _ := NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
		if target.hp >= 5000 || len(damageMessages(t, result.Messages)) != 1 {
			t.Fatalf("target hp %v, messages %+v; want the hit reported", target.hp, result.Messages)
		}
		if len(target.effects.All()) != 0 {
			t.Fatalf("target effects = %+v, want none from a CORPSE_MOB skill", target.effects.All())
		}
	})
	t.Run("no damage", func(t *testing.T) {
		caster, target, def := drainFixture()
		target.magicOK = false
		NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
		if target.hp != 5000 || caster.hp != 100 || len(target.hitLog) != 0 {
			t.Fatalf("target hp %v, caster hp %v, hits %q; want nothing", target.hp, caster.hp, target.hitLog)
		}
	})
}

// TestDrainEffectRollResistsAtLevelOne pins the unreflected landing: the
// skill-level roll fails and the caster hears it at level 1, the id-only
// addSkillName overload, from the skill's own unconditional send.
func TestDrainEffectRollResistsAtLevelOne(t *testing.T) {
	caster, target, def := drainFixture()
	def.Level = 6
	def.Effects = []modelskill.EffectTemplate{{Name: "Debuff", Time: 9}}
	target.skillSuccessChance = chanceOf(0)
	result, _ := NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
	want := []Resisted{{TargetName: "Target", SkillID: 1090, SkillLevel: 1, Unconditional: true}}
	if !slices.Equal(result.Resisted, want) || len(target.effects.All()) != 0 {
		t.Fatalf("Resisted = %+v, effects %d; want %+v and none landed", result.Resisted, len(target.effects.All()), want)
	}
}

// TestDrainMagicFailureUsesDrainMessages pins calcMagicDam's DRAIN wording:
// a half failure reports DRAIN_HALF_SUCCESFUL instead of ATTACK_FAILED, and
// a player target hears RESISTED_S1_DRAIN instead of RESISTED_S1_MAGIC.
func TestDrainMagicFailureUsesDrainMessages(t *testing.T) {
	caster, target, def := drainFixture()
	target.isPlayer = true
	target.magicInput.Failure = formulas.MagicFailureHalf
	result, _ := NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
	if result.AttackFailed != 0 {
		t.Fatalf("AttackFailed = %d, want 0 for a drain", result.AttackFailed)
	}
	if len(result.Messages) < 2 || result.Messages[0] != any(DrainHalfSucceededMessage{}) || result.Messages[1] != any(MagicResist{TargetID: 2, AttackerName: "Caster", Drain: true}) {
		t.Fatalf("messages = %+v, want DrainHalfSucceeded then a drain MagicResist", result.Messages)
	}
}

// rolledStun is an effect template carrying its own landing roll, so landing
// it goes through the target's per-template success input.
func rolledStun() []modelskill.EffectTemplate {
	return []modelskill.EffectTemplate{{Name: "Stun", Time: 10, EffectType: "STUN", EffectPower: 80, EffectPowerSet: true}}
}

// TestContinuousPassesBlessedShotAndShieldToTemplateLanding pins the inputs
// the continuous handler hands each per-template landing roll: the blessed
// spiritshot sampled at cast start, and the shield outcome it resolved for
// an offensive or debuff skill (no block for anything else).
func TestContinuousPassesBlessedShotAndShieldToTemplateLanding(t *testing.T) {
	for _, tc := range []struct {
		name       string
		def        modelskill.Definition
		bss        bool
		shield     formulas.ShieldDefense
		want       templateLanding
		wantRolls  int
		wantLanded int
	}{
		{
			name:   "debuff with blessed shot and shield block",
			def:    modelskill.Definition{ID: 1, SkillType: "DEBUFF", Debuff: true, Offensive: true, Magic: true},
			bss:    true,
			shield: formulas.ShieldSuccess,
			want:   templateLanding{bss: true, shield: formulas.ShieldSuccess}, wantRolls: 1, wantLanded: 1,
		},
		{
			name:   "debuff without blessed shot",
			def:    modelskill.Definition{ID: 1, SkillType: "DEBUFF", Debuff: true, Magic: true},
			shield: formulas.ShieldSuccess,
			want:   templateLanding{shield: formulas.ShieldSuccess}, wantRolls: 1, wantLanded: 1,
		},
		{
			name:   "buff never rolls the shield",
			def:    modelskill.Definition{ID: 1, SkillType: "BUFF", Magic: true},
			bss:    true,
			shield: formulas.ShieldSuccess,
			want:   templateLanding{bss: true, shield: formulas.ShieldFailed}, wantRolls: 0, wantLanded: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caster := &bssCasterFake{bss: tc.bss}
			target := newDisablerFake(2)
			target.shield = tc.shield
			def := tc.def
			def.Effects = rolledStun()
			continuousHandler{}.UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})

			if target.shieldRolls != tc.wantRolls {
				t.Fatalf("shield rolls = %d, want %d", target.shieldRolls, tc.wantRolls)
			}
			if len(target.templateLandings) != 1 || target.templateLandings[0] != tc.want {
				t.Fatalf("template landing inputs = %+v, want [%+v]", target.templateLandings, tc.want)
			}
			if got := len(target.list.All()); got != tc.wantLanded {
				t.Fatalf("landed effects = %d, want %d", got, tc.wantLanded)
			}
		})
	}
}

// TestDisablersPassBlessedShotAndShieldToTemplateLanding pins the Disablers
// landing inputs: one shield roll per target ahead of the type switch (even
// for a type that never rolls its own landing), reused with the cast-start
// blessed spiritshot by every per-template roll; a perfect block refuses
// every effect, including a type with no landing roll of its own.
func TestDisablersPassBlessedShotAndShieldToTemplateLanding(t *testing.T) {
	for _, tc := range []struct {
		skillType  string
		shield     formulas.ShieldDefense
		attackable bool
		wantInputs []templateLanding
		wantLanded int
	}{
		{skillType: "STUN", shield: formulas.ShieldSuccess, wantInputs: []templateLanding{{bss: true, shield: formulas.ShieldSuccess}}, wantLanded: 1},
		{skillType: "BETRAY", shield: formulas.ShieldSuccess, wantInputs: []templateLanding{{bss: true, shield: formulas.ShieldSuccess}}, wantLanded: 1},
		{skillType: "NEGATE", shield: formulas.ShieldSuccess, wantInputs: []templateLanding{{bss: true, shield: formulas.ShieldSuccess}}, wantLanded: 1},
		{skillType: "AGGDAMAGE", shield: formulas.ShieldSuccess, wantInputs: []templateLanding{{bss: true, shield: formulas.ShieldSuccess}}, wantLanded: 1},
		{skillType: "AGGREDUCE", shield: formulas.ShieldSuccess, attackable: true, wantInputs: []templateLanding{{bss: true, shield: formulas.ShieldSuccess}}, wantLanded: 1},
		{skillType: "FAKE_DEATH", shield: formulas.ShieldPerfect},
		{skillType: "NEGATE", shield: formulas.ShieldPerfect},
		{skillType: "CANCEL_DEBUFF", shield: formulas.ShieldSuccess},
	} {
		t.Run(fmt.Sprintf("%s/shield%d", tc.skillType, tc.shield), func(t *testing.T) {
			caster := &bssCasterFake{bss: true}
			target := newDisablerFake(2)
			target.shield = tc.shield
			target.attackableFlag = tc.attackable
			disablersHandler{}.Use(Cast{
				Caster:  caster,
				Skill:   modelskill.Definition{ID: 1, SkillType: tc.skillType, Magic: true, Effects: rolledStun()},
				Targets: []Actor{target},
			})
			if target.shieldRolls != 1 {
				t.Fatalf("shield rolls = %d, want 1 per target", target.shieldRolls)
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
