package skill

import (
	"math"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/worldobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
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
	_ effectListTarget = (*player.Character)(nil)
	_ effectListTarget = (*npc.Hostile)(nil)
	_ effectListTarget = (*summon.Actor)(nil)
	_ effectListTarget = (*npc.EffectPoint)(nil)
	_ continuousTarget = (*player.Character)(nil)
	_ continuousTarget = (*npc.Hostile)(nil)
	_ continuousTarget = (*summon.Actor)(nil)
	_ disablerTarget   = (*player.Character)(nil)
	_ disablerTarget   = (*npc.Hostile)(nil)
	_ disablerTarget   = (*summon.Actor)(nil)
	_ cancelTarget     = (*player.Character)(nil)

	// Damage targets: PDAM/CHARGEDAM, MDAM/DEATHLINK, BLOW, and MANADAM
	// each narrow to one of these before touching HP or MP.
	_ hpDamageTarget      = (*player.Character)(nil)
	_ hpDamageTarget      = (*npc.Hostile)(nil)
	_ hpDamageTarget      = (*summon.Actor)(nil)
	_ physicalSkillTarget = (*player.Character)(nil)
	_ physicalSkillTarget = (*npc.Hostile)(nil)
	_ physicalSkillTarget = (*summon.Actor)(nil)
	_ magicDamageTarget   = (*player.Character)(nil)
	_ magicDamageTarget   = (*npc.Hostile)(nil)
	_ magicDamageTarget   = (*summon.Actor)(nil)
	_ worldPlayerTarget   = (*player.Character)(nil)
	_ blowDamageTarget    = (*player.Character)(nil)
	_ blowDamageTarget    = (*npc.Hostile)(nil)
	_ manaDamageTarget    = (*player.Character)(nil)
	_ manaDamageTarget    = (*npc.Hostile)(nil)

	// Caster-side surfaces resolved from Cast.Caster. cancelTarget above
	// and these three share a Level() int requirement that *player.Character
	// could not meet until its persisted level field was renamed off of
	// Level to make room for the method (see player.Character.CharLevel).
	_ magicCaster   = (*player.Character)(nil)
	_ sowCaster     = (*player.Character)(nil)
	_ harvestCaster = (*player.Character)(nil)

	// Signet: the radius scan hands each found object to the tick as an
	// Actor, and an anti-summon signet narrows that to a dismissable summon.
	_ signetCastTarget    = (*player.Character)(nil)
	_ signetCastTarget    = (*npc.Hostile)(nil)
	_ signetUnsummonable  = (*summon.Actor)(nil)
	_ spoilableTarget     = (*npc.Hostile)(nil)
	_ effectSuccessSource = (*npc.Hostile)(nil)

	// Erase: the servitor surface disableErase reaches through, and the
	// owner-facing notification it fires once erased.
	_ erasableSummon         = (*summon.Actor)(nil)
	_ servitorVanishNotifier = (*player.Character)(nil)

	// SummonFriend/SummonParty: the caster-side gate, the target-side gate,
	// the pending teleport-request/confirm-summon surface, the required-item
	// check, and the teleport itself.
	_ summonFriendCaster       = (*player.Character)(nil)
	_ summonFriendTargetState  = (*player.Character)(nil)
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
	objectID int32
}

func (f fakeActor) ObjectID() int32 { return f.objectID }

func (fakeActor) Dead() bool { return false }

// ---- from apply_test.go ----
type effectLandingFake struct {
	fakeActor
	list *effect.List
}

func (f *effectLandingFake) EffectList() *effect.List { return f.list }

type effectListOnlyFake struct {
	fakeActor
	list *effect.List
}

func (f *effectListOnlyFake) EffectList() *effect.List { return f.list }

func (*effectLandingFake) EffectSuccessInput(_ creature.DeathActor, _ modelskill.Definition, tmpl modelskill.EffectTemplate, _ bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	return formulas.SkillSuccessInput{BaseChance: tmpl.EffectPower, IgnoreResists: true, Shield: shield}, true
}

func TestApplyEffectsRollsEachConfiguredTemplate(t *testing.T) {
	target := &effectLandingFake{list: effect.NewList(nil)}
	templates := []modelskill.EffectTemplate{
		{Name: "Buff", Time: 60, EffectPower: 100, EffectPowerSet: true},
		{Name: "Buff", Time: 60, EffectPower: 0, EffectPowerSet: true},
	}
	applyEffects(nil, target, modelskill.Definition{}, templates)

	if got := len(target.list.All()); got != 1 {
		t.Fatalf("landed effects = %d, want 1", got)
	}
}

func TestApplyEffectsRejectsPerfectShieldBeforeTemplates(t *testing.T) {
	target := &effectLandingFake{list: effect.NewList(nil)}
	applyEffectsWithLanding(nil, target, modelskill.Definition{}, []modelskill.EffectTemplate{{Name: "Buff", Time: 60, EffectPower: 100, EffectPowerSet: true}}, formulas.ShieldPerfect, false)

	if got := len(target.list.All()); got != 0 {
		t.Fatalf("landed effects after perfect shield = %d, want 0", got)
	}
}

func TestApplyEffectsRejectsConfiguredTemplateWithoutLandingInput(t *testing.T) {
	target := &effectListOnlyFake{list: effect.NewList(nil)}
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

func TestActiveEffectOnATargetWithNoEffectListIsFalse(t *testing.T) {
	if ActiveEffect(fakeActor{}, 288) {
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
	StopEffect(fakeActor{}, 288)
}

// ---- from cancel_test.go ----
type cancelFakeActor struct {
	fakeActor
	dead  bool
	level int
	list  *effect.List
}

func newCancelFakeActor(level int) *cancelFakeActor {
	return &cancelFakeActor{level: level, list: effect.NewList(nil)}
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
	currentTarget     worldobject.Object
	setTargetCalls    []worldobject.Object
	attackTargetCalls []worldobject.Object
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
func (*continuousFake) CharacterName() string            { return "Target" }
func (f *continuousFake) Dead() bool                     { return f.dead }
func (f *continuousFake) Invul() bool                    { return f.invul }
func (f *continuousFake) Playable() bool                 { return f.playable }
func (f *continuousFake) Attackable() bool               { return f.attackableFlag }
func (f *continuousFake) CursedWeaponEquipped() bool     { return f.cursed }
func (f *continuousFake) EffectList() *effect.List       { return f.list }
func (f *continuousFake) BlessedSpiritshotCharged() bool { return f.bss }

func (f *continuousFake) SkillSuccessInput(caster creature.DeathActor, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	if f.recordSuccessInput != nil {
		f.recordSuccessInput(caster, def, bss, shield)
	}
	return f.successInput, f.successOK
}

func (f *continuousFake) SkillReflectInput(modelskill.Definition) formulas.SkillReflectInput {
	return f.skillReflectInput
}

func (f *continuousFake) NotifyAggression(source creature.DeathActor, power int) {
	f.aggressionSource = source
	f.aggressionPower = power
}

func (f *continuousFake) CurrentTarget() worldobject.Object { return f.currentTarget }

func (f *continuousFake) SetTarget(target worldobject.Object) {
	f.setTargetCalls = append(f.setTargetCalls, target)
}

func (f *continuousFake) AttackTarget(target worldobject.Object) {
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

// ---- from cubic_test.go ----
type fakeCubicSummoner struct {
	fakeActor
	added        map[cubic.ID]bool
	givenByOther map[cubic.ID]bool
	nextAdded    bool
	servitor     modelskill.Definition
}

func newFakeCubicSummoner(nextAdded bool) *fakeCubicSummoner {
	return &fakeCubicSummoner{added: map[cubic.ID]bool{}, givenByOther: map[cubic.ID]bool{}, nextAdded: nextAdded}
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
	d := &disablerFake{id: id, list: effect.NewList(nil), successOK: true}
	d.aggro = attackable.NewThreatTable(d)
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

func (d *disablerFake) SkillSuccessInput(caster creature.DeathActor, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	d.lastBss = bss
	d.lastShield = shield
	return formulas.SkillSuccessInput{IgnoreResists: true, BaseChance: 100, Shield: shield}, d.successOK
}

// ShieldDefense reports d's pre-set shield-block outcome, letting tests
// exercise checkSkillSuccess's shield-block threading.
func (d *disablerFake) ShieldDefense(caster creature.DeathActor, def modelskill.Definition, isCrit bool) formulas.ShieldDefense {
	return d.shield
}

// SkillReflectInput reports a guaranteed reflect when d.reflects is set
// (ReflectChance 100 always beats a [0,100) roll), and no reflect otherwise.
func (d *disablerFake) SkillReflectInput(def modelskill.Definition) formulas.SkillReflectInput {
	if !d.reflects {
		return formulas.SkillReflectInput{}
	}
	return formulas.SkillReflectInput{CanBeReflected: true, Magic: true, ReflectChance: 100}
}

func (d *disablerFake) Attackable() bool                   { return d.attackableFlag }
func (d *disablerFake) RaidRelated() bool                  { return d.raidRelated }
func (d *disablerFake) Undead() bool                       { return d.undeadFlag }
func (d *disablerFake) AggroList() *attackable.ThreatTable { return d.aggro }
func (d *disablerFake) HateList() *attackable.HateTable    { return d.hate }
func (d *disablerFake) Level() int                         { return d.level }

func (d *disablerFake) NotifyAggression(source creature.DeathActor, power int) {
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
func (disablerHostileMove) Stop() error                      { return nil }

type disablerHostileAttack struct{}

func (disablerHostileAttack) BowCoolingDown() bool                { return false }
func (disablerHostileAttack) AttackingNow() bool                  { return false }
func (disablerHostileAttack) CanAttack(attackable.Combatant) bool { return false }
func (disablerHostileAttack) DoAttack(attackable.Combatant) error { return nil }

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
	c.Live = live
	c.SetRollSource(func(int) int { return 99 })
	c.SetPerfectShieldBlockRate(5)
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
	healEffectiveness float64
	healOK            bool

	physicalInput formulas.PhysicalSkillInput
	physicalOK    bool
	magicInput    formulas.MagicDamageInput
	magicOK       bool
	blowInput     formulas.BlowInput
	blowOK        bool
	manaInput     formulas.ManaDamageInput
	manaOK        bool
	lethalInput   formulas.LethalInput
	lethalOK      bool
	lethalPlayer  bool

	raidRelated  bool
	lethalImmune bool

	lethalOutcomes []formulas.LethalOutcome

	effects *effect.List
	shots   []modelitem.ShotKind
	charged map[modelitem.ShotKind]bool

	castBreakDamage []float64
}

func (t *skillTarget) BreakCastOnDamage(damage float64) {
	t.castBreakDamage = append(t.castBreakDamage, damage)
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

func (t *skillTarget) HealAmount(skill modelskill.Definition) (float64, bool) {
	return t.healAmount, t.healOK
}

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

func (t *skillTarget) AddExpAndSP(exp, sp int) { t.sp += sp }

func (t *skillTarget) Die(killer creature.DeathActor) {
	t.dead = true
	t.diedBy = killer
}

func (t *skillTarget) ReduceHP(v float64, attacker creature.DeathActor, skill modelskill.Definition) {
	t.hp -= v
}

func (t *skillTarget) SetChargedShot(kind modelitem.ShotKind, _ bool) {
	t.shots = append(t.shots, kind)
}

func (t *skillTarget) ChargedShot(kind modelitem.ShotKind) bool { return t.charged[kind] }

func (t *skillTarget) PhysicalSkillInput(caster creature.DeathActor, skill modelskill.Definition) (formulas.PhysicalSkillInput, bool) {
	return t.physicalInput, t.physicalOK
}

func (t *skillTarget) MagicDamageInput(caster creature.DeathActor, skill modelskill.Definition) (formulas.MagicDamageInput, bool) {
	return t.magicInput, t.magicOK
}

func (t *skillTarget) BlowInput(caster creature.DeathActor, skill modelskill.Definition) (formulas.BlowInput, bool) {
	return t.blowInput, t.blowOK
}

func (t *skillTarget) ManaDamageInput(caster creature.DeathActor, skill modelskill.Definition) (formulas.ManaDamageInput, bool) {
	return t.manaInput, t.manaOK
}

func (t *skillTarget) LethalInput(caster creature.DeathActor, skill modelskill.Definition) (formulas.LethalInput, bool) {
	in := t.lethalInput
	in.Chance1 = skill.LethalChance1
	in.Chance2 = skill.LethalChance2
	in.MagicLevel = skill.MagicLevel
	return in, t.lethalOK
}

func (t *skillTarget) ApplyLethalOutcome(outcome formulas.LethalOutcome, caster creature.DeathActor, skill modelskill.Definition) {
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
		Targets: []Actor{target, dead, fakeActor{}},
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
		Targets: []Actor{target, dead, fakeActor{}},
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
	target := &skillTarget{cp: 80, maxCP: 100}
	dead := &skillTarget{cp: 1, maxCP: 100, dead: true}
	invulnerable := &skillTarget{cp: 1, maxCP: 100, invulnerable: true}

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

func TestCPDamagePercentReducesCurrentCP(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{}
	target := &skillTarget{cp: 80, maxCP: 100}
	dead := &skillTarget{cp: 80, maxCP: 100, dead: true}
	invulnerable := &skillTarget{cp: 80, maxCP: 100, invulnerable: true}

	if !registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "CPDAMPERCENT", Power: 35},
		Targets: []Actor{target, dead, invulnerable, fakeActor{}},
	}) {
		t.Fatal("Use() returned false for CPDAMPERCENT")
	}
	if target.cp != 52 {
		t.Fatalf("CPDAMPERCENT cp = %v, want 52", target.cp)
	}
	if dead.cp != 80 || invulnerable.cp != 80 {
		t.Fatalf("invalid target cp changed: dead=%v invulnerable=%v", dead.cp, invulnerable.cp)
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
	caster.charged = map[modelitem.ShotKind]bool{modelitem.ShotBlessedSpirit: true}
	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "MDAM"}, Targets: []Actor{target}})

	if got, want := caster.shots, []modelitem.ShotKind{modelitem.ShotSoul, modelitem.ShotBlessedSpirit}; !slices.Equal(got, want) {
		t.Fatalf("discharged shots = %v, want %v", got, want)
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
	hp := &skillTarget{hp: 50, maxHP: 100, effects: effect.NewList(noopStatOwner{})}
	cp := &skillTarget{cp: 50, maxCP: 100, effects: effect.NewList(noopStatOwner{})}
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
		effects: effect.NewList(noopStatOwner{}),
	}
	for _, name := range []string{"Sleep", "ImmobileUntilAttacked"} {
		e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: name})
		if err != nil {
			t.Fatalf("build %s effect: %v", name, err)
		}
		target.effects.Add(e)
	}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "MANADAM"}, Targets: []Actor{target}})

	if all := target.effects.All(); len(all) != 0 {
		t.Fatalf("MANADAM drain left effects = %v, want none (Sleep and ImmobileUntilAttacked should be stopped)", all)
	}
}

// ---- from manor_test.go ----
type manorFakeSeedState struct {
	seeded, harvested bool
	allowed           bool
	sownBy            int32
	sownSeed          manor.Seed
	cropID            int32
	cropCount         int
}

func (s *manorFakeSeedState) Seeded() bool                         { return s.seeded }
func (s *manorFakeSeedState) Harvested() bool                      { return s.harvested }
func (s *manorFakeSeedState) MarkHarvested()                       { s.harvested = true }
func (s *manorFakeSeedState) AllowedToHarvest(playerID int32) bool { return s.allowed }
func (s *manorFakeSeedState) HarvestedCrop() (int32, int)          { return s.cropID, s.cropCount }
func (s *manorFakeSeedState) Sow(sowerID int32, seed manor.Seed) {
	s.seeded = true
	s.sownBy = sowerID
	s.sownSeed = seed
}

type manorFakeTarget struct {
	fakeActor
	dead  bool
	level int
	state *manorFakeSeedState
}

func (m *manorFakeTarget) Dead() bool           { return m.dead }
func (m *manorFakeTarget) Level() int           { return m.level }
func (m *manorFakeTarget) SeedState() seedState { return m.state }

type manorFakeItem struct {
	seed manor.Seed
	ok   bool
}

func (i manorFakeItem) Seed() (manor.Seed, bool) { return i.seed, i.ok }

type manorFakeCaster struct {
	fakeActor
	id    int32
	level int
	items map[int32]int
}

func (c manorFakeCaster) ObjectID() int32 { return c.id }
func (c manorFakeCaster) Level() int      { return c.level }
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
	caster := manorFakeCaster{id: 7, level: 40}
	item := manorFakeItem{seed: manor.Seed{Level: 40, Alternative: false}, ok: true}

	for i := 0; i < 300; i++ {
		target := &manorFakeTarget{level: 40, state: &manorFakeSeedState{}}
		if !registry.Use(Cast{
			Caster:  caster,
			Item:    item,
			Skill:   modelskill.Definition{SkillType: "SOW"},
			Targets: []Actor{target},
		}) {
			t.Fatal("Use() returned false for SOW")
		}
		if target.state.seeded {
			if target.state.sownBy != 7 {
				t.Fatalf("sown by = %d, want 7", target.state.sownBy)
			}
			return
		}
	}
	t.Fatal("SOW never succeeded in 300 attempts at a 90% success rate")
}

func TestSowAlreadySeededIsNoop(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := manorFakeCaster{id: 7, level: 40}
	target := &manorFakeTarget{level: 40, state: &manorFakeSeedState{seeded: true, sownBy: 3}}
	item := manorFakeItem{seed: manor.Seed{Level: 40}, ok: true}

	registry.Use(Cast{Caster: caster, Item: item, Skill: modelskill.Definition{SkillType: "SOW"}, Targets: []Actor{target}})
	if target.state.sownBy != 3 {
		t.Fatalf("already-seeded target should be untouched, sownBy = %d", target.state.sownBy)
	}
}

func TestHarvestRewardsAllowedHarvester(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &manorFakeCaster{id: 7, level: 40}
	target := &manorFakeTarget{level: 40, state: &manorFakeSeedState{seeded: true, allowed: true, cropID: 5001, cropCount: 12}}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "HARVEST"}, Targets: []Actor{target}})

	if !target.state.harvested {
		t.Error("target should be marked harvested")
	}
	if caster.items[5001] != 12 {
		t.Fatalf("caster earned items = %v, want {5001: 12}", caster.items)
	}
}

func TestHarvestDisallowedHarvesterGetsNothing(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &manorFakeCaster{id: 7, level: 40}
	target := &manorFakeTarget{level: 40, state: &manorFakeSeedState{seeded: true, allowed: false, cropID: 5001, cropCount: 12}}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "HARVEST"}, Targets: []Actor{target}})

	if target.state.harvested {
		t.Error("a disallowed harvester should not mark the target harvested")
	}
	if len(caster.items) != 0 {
		t.Fatalf("caster earned items = %v, want none", caster.items)
	}
}

func TestHarvestAlreadyHarvestedIsNoop(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &manorFakeCaster{id: 7, level: 40}
	target := &manorFakeTarget{level: 40, state: &manorFakeSeedState{seeded: true, harvested: true, allowed: true, cropID: 5001, cropCount: 12}}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "HARVEST"}, Targets: []Actor{target}})
	if len(caster.items) != 0 {
		t.Fatalf("caster earned items = %v, want none", caster.items)
	}
}

// ---- from resurrect_test.go ----
type reviveFakeCaster struct {
	fakeActor
	wit float64
}

func (c reviveFakeCaster) WITBonus() float64 { return c.wit }

type reviveFakeTarget struct {
	fakeActor
	percent float64
}

func (t *reviveFakeTarget) Revive(percent float64) bool { t.percent = percent; return true }

// reviveFakeExpTarget additionally implements expRestorer, matching
// player.Character, to verify Resurrect wires both calls.
type reviveFakeExpTarget struct {
	reviveFakeTarget
	restoredPercent float64
}

func (t *reviveFakeExpTarget) RestoreExp(restorePercent float64) { t.restoredPercent = restorePercent }

func TestResurrectRevivesEveryTarget(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := reviveFakeCaster{wit: 1.5}
	a := &reviveFakeTarget{}
	b := &reviveFakeTarget{}

	if !registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "RESURRECT", Power: 40},
		Targets: []Actor{a, b, fakeActor{}},
	}) {
		t.Fatal("Use() returned false for RESURRECT")
	}

	want := formulas.RevivePower(1.5, 40)
	if a.percent != want || b.percent != want {
		t.Fatalf("revive percent = %v/%v, want %v", a.percent, b.percent, want)
	}
}

// TestResurrectRestoresExpOnExpRestorerTargets matches
// Player.doRevive(double) (Player.java:6008-6012): restoreExp runs with the
// same revive-power percent as the HP revive.
func TestResurrectRestoresExpOnExpRestorerTargets(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := reviveFakeCaster{wit: 1.5}
	a := &reviveFakeExpTarget{}

	if !registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "RESURRECT", Power: 40},
		Targets: []Actor{a},
	}) {
		t.Fatal("Use() returned false for RESURRECT")
	}

	want := formulas.RevivePower(1.5, 40)
	if a.percent != want {
		t.Fatalf("revive percent = %v, want %v", a.percent, want)
	}
	if a.restoredPercent != want {
		t.Fatalf("restored exp percent = %v, want %v", a.restoredPercent, want)
	}
}

func TestResurrectWithoutCasterInterfaceIsNoop(t *testing.T) {
	registry := NewDefaultRegistry()
	a := &reviveFakeTarget{}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "RESURRECT"},
		Targets: []Actor{a},
	})
	if a.percent != 0 {
		t.Fatalf("revive percent = %v, want unchanged 0", a.percent)
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
	fakeActor
	dead  bool
	level int
	pool  *item.SpoilPool
}

func (s *spoilFakeTarget) Dead() bool                 { return s.dead }
func (s *spoilFakeTarget) Level() int                 { return s.level }
func (s *spoilFakeTarget) SpoilPool() *item.SpoilPool { return s.pool }

type spoilFakeCaster struct {
	fakeActor
	id             int32
	level          int
	inParty        bool
	items          map[int32]int
	distItem       int32
	distCnt        int32
	alreadyNotices int
}

func (c spoilFakeCaster) ObjectID() int32 { return c.id }
func (c spoilFakeCaster) Level() int      { return c.level }
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
func (c *spoilFakeCaster) NotifySpoilAlready() { c.alreadyNotices++ }
func (*spoilFakeCaster) NotifySpoilSuccess()   {}

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
	fakeActor
	heading, x, y, z int
}

func (t jumpFakeTarget) Heading() int { return t.heading }
func (t jumpFakeTarget) X() int       { return t.x }
func (t jumpFakeTarget) Y() int       { return t.y }
func (t jumpFakeTarget) Z() int       { return t.z }

type jumpFakeCaster struct {
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
	target := jumpFakeTarget{heading: 0, x: 100, y: 100, z: 50}
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
	fakeActor
	x, y, z int
}

func (c getPlayerFakeCaster) AlikeDead() bool           { return false }
func (c getPlayerFakeCaster) Position() (int, int, int) { return c.x, c.y, c.z }

type getPlayerFakeTarget struct {
	fakeActor
	dead       bool
	teleported bool
	tx, ty, tz int
}

func (t *getPlayerFakeTarget) AlikeDead() bool { return t.dead }
func (t *getPlayerFakeTarget) TeleportTo(x, y, z int) {
	t.teleported = true
	t.tx, t.ty, t.tz = x, y, z
}

func TestGetPlayerPullsLivingTargetsToCaster(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := getPlayerFakeCaster{x: 1, y: 2, z: 3}
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

// ---- from unlock_test.go ----
type doorFake struct {
	fakeActor
	unlockable, opened bool
}

func (d *doorFake) Unlockable() bool { return d.unlockable }
func (d *doorFake) Opened() bool     { return d.opened }
func (d *doorFake) Open()            { d.opened = true }

func TestUnlockDoorSpecialGuaranteedSuccess(t *testing.T) {
	registry := NewDefaultRegistry()
	door := &doorFake{unlockable: false}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "UNLOCK_SPECIAL", Power: 150},
		Targets: []Actor{door},
	})
	if !door.opened {
		t.Fatal("UNLOCK_SPECIAL with power >= 100 should always open, even an unlockable=false door")
	}
}

func TestUnlockDoorLevelZeroNeverOpens(t *testing.T) {
	registry := NewDefaultRegistry()
	door := &doorFake{unlockable: true}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "UNLOCK", Level: 0},
		Targets: []Actor{door},
	})
	if door.opened {
		t.Fatal("level 0 unlock should never open a door")
	}
}

func TestUnlockDoorNotUnlockableIsSkipped(t *testing.T) {
	registry := NewDefaultRegistry()
	door := &doorFake{unlockable: false}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "UNLOCK", Level: 4},
		Targets: []Actor{door},
	})
	if door.opened {
		t.Fatal("a non-unlockable door should not open via a regular UNLOCK")
	}
}

type chestFake struct {
	fakeActor
	dead, interacted, box bool
	level                 int

	died, deleted          bool
	desireAdded, hateAdded bool
}

func (c *chestFake) Dead() bool                     { return c.dead }
func (c *chestFake) Interacted() bool               { return c.interacted }
func (c *chestFake) SetInteracted()                 { c.interacted = true }
func (c *chestFake) Box() bool                      { return c.box }
func (c *chestFake) Level() int                     { return c.level }
func (c *chestFake) Die(killer creature.DeathActor) { c.died = true }
func (c *chestFake) DeleteMe()                      { c.deleted = true }

func (c *chestFake) AddAttackDesire(attacker creature.DeathActor, weight float64) {
	c.desireAdded = true
}
func (c *chestFake) AddDamageHate(attacker creature.DeathActor, damage, hate float64) {
	c.hateAdded = true
}

func TestUnlockChestNotBoxAddsAttackDesire(t *testing.T) {
	registry := NewDefaultRegistry()
	chest := &chestFake{box: false}

	registry.Use(Cast{Skill: modelskill.Definition{SkillType: "UNLOCK"}, Targets: []Actor{chest}})
	if !chest.desireAdded {
		t.Fatal("expected an attack desire for a non-box chest")
	}
	if chest.interacted {
		t.Fatal("a non-box chest should not be marked interacted")
	}
}

func TestUnlockChestDeluxeKeyExactLevelMatchGuaranteedOpen(t *testing.T) {
	registry := NewDefaultRegistry()
	chest := &chestFake{box: true, level: 100}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "DELUXE_KEY_UNLOCK", ID: 9999, Level: 10},
		Targets: []Actor{chest},
	})
	if !chest.interacted {
		t.Fatal("chest should be marked interacted")
	}
	if !chest.died || !chest.hateAdded {
		t.Fatalf("exact-level deluxe key should always open: died=%v hateAdded=%v", chest.died, chest.hateAdded)
	}
}

func TestUnlockChestAboveBracketTooLowSkillGuaranteedFail(t *testing.T) {
	registry := NewDefaultRegistry()
	chest := &chestFake{box: true, level: 70}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "UNLOCK", Level: 5},
		Targets: []Actor{chest},
	})
	if chest.died {
		t.Fatal("a level 5 unlock skill should never open a level-70 chest")
	}
	if !chest.deleted {
		t.Fatal("a failed chest unlock should delete the chest")
	}
}
