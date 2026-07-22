package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
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
			target := newDisablerHostile(t, 100)

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
				Targets: []any{target},
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

func newDisablerHostile(t testing.TB, id int32) *npc.Hostile {
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
			HPMax:           100,
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
func (disablerHostileGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

type disablerHostileMove struct{}

func (disablerHostileMove) MaybeStartOffensiveFollow(attackable.Combatant, int) bool { return false }
func (disablerHostileMove) MoveHome(location.Location)                               {}
func (disablerHostileMove) Stop()                                                    {}

type disablerHostileAttack struct{}

func (disablerHostileAttack) BowCoolingDown() bool                { return false }
func (disablerHostileAttack) AttackingNow() bool                  { return false }
func (disablerHostileAttack) CanAttack(attackable.Combatant) bool { return false }
func (disablerHostileAttack) DoAttack(attackable.Combatant)       {}

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
