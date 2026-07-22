package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

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

	// lastBss and lastShield record the most recent SkillSuccessInput call's
	// resolved caster/target state, for tests asserting checkSkillSuccess
	// threaded them through correctly.
	lastBss    bool
	lastShield formulas.ShieldDefense
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

func (d *disablerFake) SkillSuccessInput(caster any, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	d.lastBss = bss
	d.lastShield = shield
	return formulas.SkillSuccessInput{IgnoreResists: true, BaseChance: 100, Shield: shield}, d.successOK
}

// ShieldDefense reports d's pre-set shield-block outcome, letting tests
// exercise checkSkillSuccess's shield-block threading.
func (d *disablerFake) ShieldDefense(caster any, def modelskill.Definition, isCrit bool) formulas.ShieldDefense {
	return d.shield
}

func (d *disablerFake) Attackable() bool                   { return d.attackableFlag }
func (d *disablerFake) RaidRelated() bool                  { return d.raidRelated }
func (d *disablerFake) Undead() bool                       { return d.undeadFlag }
func (d *disablerFake) AggroList() *attackable.ThreatTable { return d.aggro }
func (d *disablerFake) HateList() *attackable.HateTable    { return d.hate }

func TestDisablersSkipsDeadAndUnparalyzedInvulTargets(t *testing.T) {
	registry := NewDefaultRegistry()
	dead := newDisablerFake(1)
	dead.dead = true
	invul := newDisablerFake(2)
	invul.invul = true

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "FAKE_DEATH", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []any{dead, invul},
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
		Targets: []any{target},
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
		Targets: []any{target},
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
		Targets: []any{target},
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
		Targets: []any{target},
	})
	if len(target.list.All()) != 1 {
		t.Fatal("STUN should apply its effect on a guaranteed-success roll")
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
		Targets: []any{target},
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
		Targets: []any{target},
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
		Targets: []any{notAttackable, raidRelated},
	})

	if notAttackable.aggro.IsEmpty() {
		t.Error("a non-attackable target's aggro should be untouched")
	}
	if raidRelated.aggro.IsEmpty() {
		t.Error("a raid-related target's aggro should be untouched")
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
		Targets: []any{target},
	})

	if !target.aggro.IsEmpty() || !target.hate.IsEmpty() {
		t.Fatal("AGGREMOVE should clear both hate tables on a guaranteed-success roll")
	}
}

// bssCasterFake exposes a fixed blessed-spiritshot charge state for tests
// asserting checkSkillSuccess resolves it from the caster.
type bssCasterFake struct{ bss bool }

func (c *bssCasterFake) BlessedSpiritshotCharged() bool { return c.bss }

func TestCheckSkillSuccessFailsOnPerfectShieldBlockDespiteGuaranteedRate(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)
	target.shield = formulas.ShieldPerfect

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "STUN", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []any{target},
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
		target.AddStatFuncs([]basefunc.Func{
			basefunc.NewSet(target, stat.ShieldRate, 20, nil),
			basefunc.NewSet(target, stat.ShieldDefenceAngle, 120, nil),
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
	registry.Use(Cast{Caster: caster, Skill: skill, Targets: []any{unblocked}})
	if got := len(unblocked.EffectList().All()); got != 1 {
		t.Fatalf("unblocked target effects = %d, want 1", got)
	}

	registry.Use(Cast{Caster: caster, Skill: skill, Targets: []any{blocked}})
	if got := len(blocked.EffectList().All()); got != 0 {
		t.Fatalf("perfect-shield-blocked target effects = %d, want 0", got)
	}
}

type liveShieldGeo struct{}

func (liveShieldGeo) CanMove(_, _, _, _, _, _ int) bool { return true }
func (liveShieldGeo) Height(_, _, _ int) int16          { return 0 }
func (liveShieldGeo) FindPath(_, _ location.Location) ([]location.Location, bool) {
	return nil, false
}
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
	return c
}

func TestCheckSkillSuccessResolvesCasterBlessedSpiritshotCharge(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)
	caster := &bssCasterFake{bss: true}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "STUN", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []any{target},
	})

	if !target.lastBss {
		t.Fatal("checkSkillSuccess should have resolved the caster's blessed-spiritshot charge as true")
	}
}
