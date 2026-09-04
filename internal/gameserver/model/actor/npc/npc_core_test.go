package npc

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestHostileMaxBuffCountIncludesTemplateDivineInspiration(t *testing.T) {
	hostile := newCombatHostile(t, 1, &Template{
		ID:     1,
		Type:   "Monster",
		Skills: map[int]int{int(modelskill.DivineInspirationSkillID): 3},
	})

	if got := hostile.MaxBuffCount(); got != 23 {
		t.Fatalf("MaxBuffCount() = %d, want 23", got)
	}
}

func TestHostileMaxBuffCountUsesConfiguredBase(t *testing.T) {
	hostile := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster"})
	hostile.SetMaxBuffsAmount(2)

	if got := hostile.MaxBuffCount(); got != 2 {
		t.Fatalf("MaxBuffCount() = %d, want 2", got)
	}
}

// ---- from hostile_cancel_vulnerability_test.go ----
// TestHostileCancelVulnerabilityAppliesCancelVulnStat proves CANCEL_VULN
// (Formulas.java:949-951) reaches Hostile.CancelVulnerability through the
// live stat calculator — see the parallel player-package test for the
// downstream formula behavior this plumbing enables.
func TestHostileCancelVulnerabilityAppliesCancelVulnStat(t *testing.T) {
	hostile, err := NewHostile(&Instance{
		ObjectID: 101,
		Template: &Template{ID: 9001, Type: "Monster", Level: 20},
		Kind:     "Monster",
	}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatal(err)
	}

	if got := hostile.CancelVulnerability("CANCEL"); got != 1 {
		t.Fatalf("CancelVulnerability() with no modifier = %v, want 1 (unmodified)", got)
	}

	hostile.AddStatFuncs([]effect.Mod{{Stat: stat.CancelVuln, Op: effect.OpMul, Value: 0.2}})
	if got := hostile.CancelVulnerability("CANCEL"); got != 0.2 {
		t.Fatalf("CancelVulnerability() after 0.2x modifier = %v, want 0.2", got)
	}
}

// ---- from hostile_collision_test.go ----
func TestHostileCollisionRadiusOverride(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", CollisionRadius: 9})

	if got := h.CollisionRadius(); got != 9 {
		t.Fatalf("CollisionRadius() before override = %v, want template value 9", got)
	}

	h.SetCollisionRadius(9 * 1.19)
	if got := h.CollisionRadius(); got != 9*1.19 {
		t.Fatalf("CollisionRadius() after SetCollisionRadius = %v, want %v", got, 9*1.19)
	}

	h.ResetCollisionRadius()
	if got := h.CollisionRadius(); got != 9 {
		t.Fatalf("CollisionRadius() after ResetCollisionRadius = %v, want template value 9", got)
	}
}

// ---- from hostile_conditions_test.go ----
// levelGate is a minimal effect.Condition mirroring skill/effect's
// conditionGate: it resolves effector to a conditions.Actor and requires
// its Level() to meet min. Before #1509, hostileStatActor didn't implement
// conditions.Actor, so this always resolved to false regardless of min — a
// conditional stat func on an NPC-owned skill silently never applied.
type levelGate struct{ min int }

func (g levelGate) Test(effector stat.Actor) bool {
	actor, ok := effector.(conditions.Actor)
	return ok && actor.Level() >= g.min
}

func TestHostileConditionalStatFuncGatesOnRealLevel(t *testing.T) {
	hostile, err := NewHostile(&Instance{
		ObjectID: 101,
		Template: &Template{
			ID:    9001,
			Type:  "Monster",
			Level: 15,
		},
		Kind: "Monster",
	}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatal(err)
	}

	// HealEffectiveness carries no default NPC stat func (see
	// defaultStatFuncs), so its finalized value is driven purely by what
	// AddStatFuncs attaches here.
	hostile.AddStatFuncs([]effect.Mod{
		{Stat: stat.HealEffectiveness, Op: effect.OpAdd, Value: 25, Cond: levelGate{min: 10}},
	})
	hostile.AddStatFuncs([]effect.Mod{
		{Stat: stat.RechargeMPRate, Op: effect.OpAdd, Value: 25, Cond: levelGate{min: 100}},
	})

	if got := hostile.CalcStat(stat.HealEffectiveness, 100); got != 125 {
		t.Errorf("CalcStat(HealEffectiveness) = %v, want 125 (level 15 >= 10 gate should pass)", got)
	}
	if got := hostile.CalcStat(stat.RechargeMPRate, 100); got != 100 {
		t.Errorf("CalcStat(RechargeMPRate) = %v, want 100 unchanged (level 15 >= 100 gate should fail)", got)
	}
}

func TestHostileCalcStatFloorsNonNegativeStatsAtOne(t *testing.T) {
	hostile, err := NewHostile(&Instance{
		ObjectID: 101,
		Template: &Template{ID: 9001, Type: "Monster", Level: 20, PAtk: 100},
		Kind:     "Monster",
	}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatal(err)
	}

	hostile.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpSet, Value: 0}})
	if got := hostile.CalcStat(stat.PowerAttack, 100); got != 1 {
		t.Errorf("CalcStat(PowerAttack, 100) = %v, want 1", got)
	}
}

func TestHostileStatActorImplementsConditionsActor(t *testing.T) {
	hostile, err := NewHostile(&Instance{
		ObjectID: 101,
		Template: &Template{ID: 9001, Type: "Monster", Level: 20, HPMax: 1000},
		Kind:     "Monster",
	}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatal(err)
	}

	var actor conditions.Actor = hostileStatActor{h: hostile}
	if actor.Level() != 20 {
		t.Errorf("Level() = %v, want 20", actor.Level())
	}
	if actor.HPRatio() <= 0 {
		t.Errorf("HPRatio() = %v, want > 0 for a freshly spawned NPC", actor.HPRatio())
	}
	if !actor.IsRunning() {
		t.Error("IsRunning() = false, want true (NPCs always spawn in run stance)")
	}
	if actor.IsRiding() || actor.IsFlying() {
		t.Error("IsRiding()/IsFlying() should default false for an NPC")
	}
	if _, ok := actor.ActiveSkillLevel(1); ok {
		t.Error("ActiveSkillLevel(1) ok = true, want false for an NPC with no active effects")
	}
}

// ---- from hostile_damage_effects_test.go ----
// TestHostileReduceHPStopsSleepAndImmobileUntilAttackedEffects mirrors
// NpcStatus.reduceHp's inherited CreatureStatus.reduceHp non-DOT block
// (CreatureStatus.java:228-248): a non-DOT hit stops SLEEP and
// IMMOBILE_UNTIL_ATTACKED.
func TestHostileReduceHPStopsSleepAndImmobileUntilAttackedEffects(t *testing.T) {
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	addHostileEffect(t, hostile, "Sleep")
	addHostileEffect(t, hostile, "ImmobileUntilAttacked")

	hostile.ReduceHP(10, nil, modelskill.Definition{})

	if hostile.Sleeping() {
		t.Fatal("Sleeping() = true after ReduceHP, want the sleep effect stopped")
	}
	if hostile.ImmobileUntilAttacked() {
		t.Fatal("ImmobileUntilAttacked() = true after ReduceHP, want the effect stopped")
	}
}

func TestHostileMovementDisabledTracksCrowdControl(t *testing.T) {
	h := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	h.Instance.Template.CanMove = true
	if h.MovementDisabled() {
		t.Fatal("MovementDisabled() = true for a mobile NPC with no crowd-control")
	}

	h.Instance.Template.CanMove = false
	if !h.MovementDisabled() {
		t.Fatal("MovementDisabled() = false for canMove=false")
	}
	h.Instance.Template.CanMove = true

	root := addHostileEffect(t, h, "Root")
	if !h.MovementDisabled() {
		t.Fatal("MovementDisabled() = false while rooted")
	}
	h.EffectList().Remove(root)
	if h.MovementDisabled() {
		t.Fatal("MovementDisabled() = true after root was removed")
	}

	addHostileEffect(t, h, "Fear")
	if h.MovementDisabled() {
		t.Fatal("MovementDisabled() = true while afraid; fear does not disable movement")
	}
}

// TestHostileReduceHPByDOTLeavesEffectsAloneOnRealDOTTick mirrors the
// !isDOT gate on CreatureStatus.reduceHp's whole SLEEP/IMMOBILE/STUN block:
// NpcStatus has no PlayerStatus-style override, so a real DOT tick
// (isDOT=true) skips the block entirely.
func TestHostileReduceHPByDOTLeavesEffectsAloneOnRealDOTTick(t *testing.T) {
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	addHostileEffect(t, hostile, "Sleep")
	addHostileEffect(t, hostile, "ImmobileUntilAttacked")
	addHostileEffect(t, hostile, "Stun")
	hostile.SetRollSource(func(int) int { return 0 })

	hostile.ReduceHPByDOT(10, nil, true)

	if !hostile.Sleeping() {
		t.Fatal("Sleeping() = false after a DOT tick, want the sleep effect untouched")
	}
	if !hostile.ImmobileUntilAttacked() {
		t.Fatal("ImmobileUntilAttacked() = false after a DOT tick, want the effect untouched")
	}
	if !hostile.Stunned() {
		t.Fatal("Stunned() = false after a DOT tick, want the stun effect untouched")
	}
}

// TestHostileReduceHPByDOTAppliesEffectsWhenNotARealDOTTick mirrors
// drowning's WaterTaskManager.reduceCurrentHp(hp, player, false, false,
// null) call: isDOT=false periodic damage still runs the block.
func TestHostileReduceHPByDOTAppliesEffectsWhenNotARealDOTTick(t *testing.T) {
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	addHostileEffect(t, hostile, "Sleep")

	hostile.ReduceHPByDOT(10, nil, false)

	if hostile.Sleeping() {
		t.Fatal("Sleeping() = true after a non-DOT periodic hit, want the sleep effect stopped")
	}
}

// TestHostileTakeDamageStopsSleepAndImmobileUntilAttackedEffects mirrors the
// melee auto-attack path (CreatureAttack.java:263 -> NpcStatus.reduceHp),
// which is always non-DOT.
func TestHostileTakeDamageStopsSleepAndImmobileUntilAttackedEffects(t *testing.T) {
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	addHostileEffect(t, hostile, "Sleep")
	addHostileEffect(t, hostile, "ImmobileUntilAttacked")

	hostile.TakeDamage(10, nil)

	if hostile.Sleeping() {
		t.Fatal("Sleeping() = true after TakeDamage, want the sleep effect stopped")
	}
	if hostile.ImmobileUntilAttacked() {
		t.Fatal("ImmobileUntilAttacked() = true after TakeDamage, want the effect stopped")
	}
}

// TestHostileReduceHPBreaksStunOnOneInTenRollForNonDOTDamage mirrors
// !isDOT && isStunned() && Rnd.get(10) == 0.
func TestHostileReduceHPBreaksStunOnOneInTenRollForNonDOTDamage(t *testing.T) {
	tests := []struct {
		name      string
		roll      int
		wantAfter bool
	}{
		{"winning roll breaks stun", 0, false},
		{"losing roll leaves stun active", 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
			addHostileEffect(t, hostile, "Stun")
			hostile.SetRollSource(func(int) int { return tt.roll })

			hostile.ReduceHP(10, nil, modelskill.Definition{})

			if got := hostile.Stunned(); got != tt.wantAfter {
				t.Fatalf("Stunned() = %v, want %v", got, tt.wantAfter)
			}
		})
	}
}

// ---- from hostile_dot_test.go ----
var (
	_ interface {
		Dead() bool
		HP() float64
		ReduceHPByDOT(float64, effect.Participant, bool)
	} = (*Hostile)(nil)
	_ interface {
		Dead() bool
		MPValue() float64
		ReduceMP(float64) float64
	} = (*Hostile)(nil)
)

func TestDamageOverTimeEffectTargetsHostile(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{HPMax: 100, MPMax: 50})
	e, err := effect.New(effect.Skill{ID: 1}, skill.EffectTemplate{Name: "DamOverTime", Value: 4})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = h
	if !e.ActionTime() {
		t.Fatal("ActionTime() = false, want true")
	}
	if got, want := h.HP(), h.MaxHPValue()-4; got != want {
		t.Fatalf("HP() = %v, want %v", got, want)
	}
}

// TestDamageOverTimeEffectRecordsZeroHateThreat pins Finding 1 of the #1088
// closed-PR review: ReduceHPByDOT must record the caster in the threat
// table at zero hate weight, matching Npc.reduceCurrentHp's unconditional
// addDamageHate(attacker, damage, 0) (Npc.java:390-395) — DOT feeds the
// AggroList's damage/timestamp bookkeeping even though it never raises
// hate above zero (so target selection is unaffected).
func TestDamageOverTimeEffectRecordsZeroHateThreat(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{HPMax: 100, MPMax: 50})
	caster := newCombatHostile(t, 2, &Template{HPMax: 100, MPMax: 50})
	e, err := effect.New(effect.Skill{ID: 1}, skill.EffectTemplate{Name: "DamOverTime", Value: 4})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = h
	e.Effector = caster
	if !e.ActionTime() {
		t.Fatal("ActionTime() = false, want true")
	}
	threat, ok := h.AI().Threats().Get(caster)
	if !ok {
		t.Fatal("Threats().Get(caster) ok = false, want caster recorded")
	}
	if threat.Damage != 4 {
		t.Fatalf("threat.Damage = %v, want 4", threat.Damage)
	}
	if threat.Hate != 0 {
		t.Fatalf("threat.Hate = %v, want 0", threat.Hate)
	}
}

func TestDamageOverTimeEffectQueuesAttackDesire(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{HPMax: 100, MPMax: 50})
	caster := newCombatHostile(t, 2, &Template{HPMax: 100, MPMax: 50})
	e, err := effect.New(effect.Skill{ID: 1}, skill.EffectTemplate{Name: "DamOverTime", Value: 4})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = h
	e.Effector = caster
	if !e.ActionTime() {
		t.Fatal("ActionTime() = false, want true")
	}
	desire, ok := h.AI().Desires().Peek()
	if !ok {
		t.Fatal("Desires().Peek() ok = false, want DOT attack desire")
	}
	if desire.FinalTarget != caster || desire.Weight != 200 {
		t.Fatalf("DOT desire = (%v, %v), want (%v, 200)", desire.FinalTarget, desire.Weight, caster)
	}
}

func TestManaDamageOverTimeEffectTargetsHostile(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{HPMax: 100, MPMax: 50})
	e, err := effect.New(effect.Skill{ID: 1}, skill.EffectTemplate{Name: "ManaDamOverTime", Value: 4})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = h
	if !e.ActionTime() {
		t.Fatal("ActionTime() = false, want true")
	}
	if got, want := h.MPValue(), 46.0; got != want {
		t.Fatalf("MPValue() = %v, want %v", got, want)
	}
}

// ---- from hostile_effects_test.go ----
func TestHostileSkillSuccessInputUsesTemplateStatsAndCasterMagicAttack(t *testing.T) {
	caster := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", Level: 12, MAtk: 200})
	target := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", Level: 10, MEN: 40, MDef: 50})
	def := modelskill.Definition{BaseLandRate: 50, EffectType: "ROOT", Magic: true, LevelDepend: 1}

	without, ok := target.SkillSuccessInput(caster, def, false, formulas.ShieldFailed)
	if !ok {
		t.Fatal("SkillSuccessInput() ok = false")
	}
	with, ok := target.SkillSuccessInput(caster, def, true, formulas.ShieldFailed)
	if !ok {
		t.Fatal("SkillSuccessInput(bss=true) ok = false")
	}

	if without.BaseChance != 50 {
		t.Fatalf("BaseChance = %v, want 50", without.BaseChance)
	}
	if want := math.Max(0, 2-math.Sqrt(statbonus.MENBonus[40])); !closeNPCFloat(without.StatModifier, want) {
		t.Fatalf("StatModifier = %v, want %v", without.StatModifier, want)
	}
	if want := without.MAtkModifier * 2; !closeNPCFloat(with.MAtkModifier, want) {
		t.Fatalf("MAtkModifier with bss = %v, want %v", with.MAtkModifier, want)
	}
	if want := 1.015; !closeNPCFloat(without.LevelModifier, want) {
		t.Fatalf("LevelModifier = %v, want %v", without.LevelModifier, want)
	}
}

func TestHostileInactiveRegionStopsAllEffects(t *testing.T) {
	hostile := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster"})
	hostile.EffectList().Add(&effect.Effect{Skill: effect.Skill{ID: 1}, Template: modelskill.EffectTemplate{Name: "test"}})

	hostile.OnInactiveRegion()

	if got := hostile.EffectList().All(); len(got) != 0 {
		t.Fatalf("effects after region deactivation = %d, want 0", len(got))
	}
}

func TestHostileSkillSuccessInputAllowsIgnoreResistsWithoutCasterStats(t *testing.T) {
	target := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster"})

	in, ok := target.SkillSuccessInput(nil, modelskill.Definition{
		BaseLandRate:  100,
		IgnoreResists: true,
	}, false, formulas.ShieldPerfect)
	if !ok {
		t.Fatal("SkillSuccessInput(ignore resists) ok = false")
	}
	if !in.IgnoreResists || in.BaseChance != 100 || in.Shield != formulas.ShieldPerfect {
		t.Fatalf("SkillSuccessInput(ignore resists) = %+v, want base chance, ignore flag, and shield preserved", in)
	}
}

func closeNPCFloat(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// ---- from hostile_lethal_test.go ----
type deniedLethalCaster struct{ *Hostile }

func (deniedLethalCaster) CanGiveDamage() bool { return false }

func TestHostileLethalSurfaceBuildsInputAndAppliesOutcomes(t *testing.T) {
	caster := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", Level: 40, HPMax: 500})
	target := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", Level: 45, HPMax: 500})
	skill := modelskill.Definition{LethalChance1: 30, LethalChance2: 10, MagicLevel: 40}

	in, ok := target.LethalInput(caster, skill)
	if !ok {
		t.Fatal("LethalInput() ok = false")
	}
	if in.Chance1 != 30 || in.Chance2 != 10 || in.MagicLevel != 40 || in.AttackerLevel != 40 || in.TargetLevel != 45 || in.LethalMul != 1 {
		t.Fatalf("LethalInput() = %+v, want skill fields and 40/45/1 actor values", in)
	}

	hp := target.MaxHPValue()
	target.SetHP(hp)
	target.ApplyLethalOutcome(formulas.LethalHalf, caster, skill)
	if got := target.HP(); got != hp/2 {
		t.Fatalf("half lethal HP = %v, want %v", got, hp/2)
	}

	target.SetHP(hp)
	target.ApplyLethalOutcome(formulas.LethalFull, caster, skill)
	if got := target.HP(); got != 1 {
		t.Fatalf("full lethal HP = %v, want 1", got)
	}
}

func TestHostileLethalInputRejectsGuardedDamage(t *testing.T) {
	caster := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", Level: 40, HPMax: 500})
	target := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", Level: 45, HPMax: 500})
	skill := modelskill.Definition{LethalChance1: 30}
	target.SetInvul(true)
	if _, ok := target.LethalInput(caster, skill); ok {
		t.Fatal("LethalInput accepted an invulnerable hostile")
	}
	target.SetInvul(false)
	if _, ok := target.LethalInput(deniedLethalCaster{caster}, skill); ok {
		t.Fatal("LethalInput accepted an attacker without damage permission")
	}
}

func TestHostileLethalableExcludesReferenceExceptions(t *testing.T) {
	for _, tt := range []struct {
		id   int
		want bool
	}{
		{id: 1, want: true},
		{id: 22215}, {id: 22216}, {id: 22217}, {id: 35062},
		{id: 35410}, {id: 35368}, {id: 35375}, {id: 35629},
	} {
		t.Run("npc", func(t *testing.T) {
			h := newCombatHostile(t, 1, &Template{ID: tt.id, Type: "Monster", HPMax: 100})
			if got := h.Lethalable(); got != tt.want {
				t.Fatalf("Lethalable() = %v, want %v for NPC %d", got, tt.want, tt.id)
			}
		})
	}
}

// ---- from hostile_stats_golden_test.go ----
// goldenHostileScenarios is npc.Hostile's half of the stat pipeline parity
// oracle described in issue #1527: same-order funcs attached in different
// sequences (float addition's insertion order is load-bearing), a Set
// rebase, and attach/detach round-tripping, each running through the
// shared builtin finalize step at order 10.
func goldenHostileScenarios(t testing.TB) map[string]float64 {
	t.Helper()
	out := make(map[string]float64)

	tpl := func() *Template {
		return &Template{ID: 1, Type: "Monster", Level: 20, STR: 40, CON: 21, DEX: 30, INT: 20, WIT: 43, MEN: 20,
			PAtk: 100, PDef: 50, MAtk: 64, MDef: 40, HPMax: 500, MPMax: 200}
	}

	{
		h1 := newCombatHostile(t, 1, tpl())
		h1.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 1e16}})
		h1.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpSub, Value: 1e16}})
		h1.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 1}})
		out["order30_forward"] = h1.PDef()

		h2 := newCombatHostile(t, 2, tpl())
		h2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 1}})
		h2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpSub, Value: 1e16}})
		h2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 1e16}})
		out["order30_reverse"] = h2.PDef()
	}

	{
		h := newCombatHostile(t, 3, tpl())
		h.AddStatFuncs([]effect.Mod{
			{Stat: stat.MagicDefence, Op: effect.OpSet, Value: 500},
			{Stat: stat.MagicDefence, Op: effect.OpBaseMul, Value: 0.5},
		})
		out["set_rebase_mdef"] = h.MDef()
	}

	{
		h := newCombatHostile(t, 4, tpl())
		base := h.PAtk()
		owner := effect.ModOwnerEffect(&effect.Effect{})
		h.AddStatFuncs([]effect.Mod{
			{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 7, Owner: owner},
			{Stat: stat.PowerAttack, Op: effect.OpMul, Value: 1.25, Owner: owner},
		})
		out["attach_detach_before"] = base
		out["attach_detach_during"] = h.PAtk()
		h.RemoveStatsByOwner(owner)
		out["attach_detach_after"] = h.PAtk()
	}

	return out
}

func TestGoldenHostileStatPipelineCapture(t *testing.T) {
	if os.Getenv("ACIS_CAPTURE_GOLDEN") == "" {
		t.Skip("set ACIS_CAPTURE_GOLDEN=1 to (re)capture the golden fixture from the current implementation")
	}
	got := goldenHostileScenarios(t)
	writeHostileGolden(t, "testdata/golden_stats.json", got)
}

func TestGoldenHostileStatPipelineParity(t *testing.T) {
	want := readHostileGolden(t, "testdata/golden_stats.json")
	got := goldenHostileScenarios(t)
	compareHostileGolden(t, want, got)
}

func writeHostileGolden(t testing.TB, path string, values map[string]float64) {
	t.Helper()
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	bitsMap := make(map[string]uint64, len(values))
	for _, k := range keys {
		bitsMap[k] = math.Float64bits(values[k])
	}
	data, err := json.MarshalIndent(bitsMap, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write golden fixture %s: %v", path, err)
	}
}

func readHostileGolden(t testing.TB, path string) map[string]float64 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden fixture %s: %v (capture it first with ACIS_CAPTURE_GOLDEN=1)", path, err)
	}
	var bitsMap map[string]uint64
	if err := json.Unmarshal(data, &bitsMap); err != nil {
		t.Fatalf("unmarshal golden fixture %s: %v", path, err)
	}
	out := make(map[string]float64, len(bitsMap))
	for k, v := range bitsMap {
		out[k] = math.Float64frombits(v)
	}
	return out
}

func compareHostileGolden(t testing.TB, want, got map[string]float64) {
	t.Helper()
	for k, w := range want {
		g, ok := got[k]
		if !ok {
			t.Errorf("golden case %q missing from current run", k)
			continue
		}
		if math.Float64bits(g) != math.Float64bits(w) {
			t.Errorf("golden case %q = %v (bits %x), want %v (bits %x)", k, g, math.Float64bits(g), w, math.Float64bits(w))
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("golden case %q present in current run but not in fixture", k)
		}
	}
}

// ---- from instance_test.go ----
var supportedKindsOracle = []InstanceKind{
	"Adventurer", "Auctioneer", "BabyPet", "CastleBlacksmith", "CastleChamberlain",
	"CastleDoorman", "CastleGatekeeper", "CastleMagician", "CastleWarehouseKeeper", "Chest",
	"ChristmasTree", "ClanHallDoorman", "ClanHallManagerNpc", "ClassMaster", "Cubic",
	"DawnPriest", "DerbyTrackManagerNpc", "Door", "Doorman", "DungeonGatekeeper",
	"DuskPriest", "EffectPoint", "FeedableBeast", "Fence", "FestivalGuide", "FestivalMonster",
	"Fisherman", "FlameTower", "Folk", "FriendlyMonster", "Gatekeeper", "GrandBoss", "Guard",
	"HalishaChest", "HolyThing", "LifeTower", "ManorManagerNpc", "MercenaryManagerNpc", "Merchant",
	"Monster", "MutedFolk", "OlympiadManagerNpc", "Pet", "RaidBoss", "SchemeBuffer", "Servitor",
	"SiegeFlag", "SiegeGuard", "SiegeNpc", "SiegeSummon", "SignsPriest", "StaticObject", "SymbolMaker",
	"TamedBeast", "Trainer", "VillageMaster", "VillageMasterDElf", "VillageMasterDwarf", "VillageMasterFighter",
	"VillageMasterMystic", "VillageMasterOrc", "VillageMasterPriest", "WarehouseKeeper", "WeddingManagerNpc", "WyvernManagerNpc",
}

func TestNewInstance_AllSupportedKinds(t *testing.T) {
	if len(supportedKindsOracle) != 65 {
		t.Fatalf("oracle has %d kinds, want 65", len(supportedKindsOracle))
	}

	for _, kind := range supportedKindsOracle {
		t.Run(string(kind), func(t *testing.T) {
			got, err := NewInstance(101, &Template{ID: 9001, Type: string(kind)})
			if err != nil {
				t.Fatalf("NewInstance() error: %v", err)
			}
			if got.ObjectID != 101 || got.Template.ID != 9001 || got.Kind != kind {
				t.Fatalf("instance = %+v", got)
			}
		})
	}
}

func TestNewInstance_RejectsInvalidTemplate(t *testing.T) {
	for _, tpl := range []*Template{nil, {Type: ""}, {Type: "NotAType"}} {
		if _, err := NewInstance(1, tpl); err == nil {
			t.Fatalf("NewInstance(%+v) error = nil", tpl)
		}
	}
}

// ---- from race_test.go ----
func TestRaceBySecondarySkillID(t *testing.T) {
	for skillID, want := range map[int]Race{
		4295: RaceHumanoid,
		4296: RaceSpirit,
		4297: RaceAngel,
		4298: RaceDemon,
	} {
		if got := RaceBySecondarySkillID(skillID); got != want {
			t.Fatalf("RaceBySecondarySkillID(%d) = %v, want %v", skillID, got, want)
		}
	}
	if got := RaceBySecondarySkillID(4290); got != RaceUndead {
		t.Fatalf("RaceBySecondarySkillID(4290) = %v, want RaceUndead", got)
	}
	if got := RaceBySecondarySkillID(4302); got != RaceFairy {
		t.Fatalf("RaceBySecondarySkillID(4302) = %v, want RaceFairy", got)
	}
	if got := RaceBySecondarySkillID(4416); got != RaceDummy {
		t.Fatalf("RaceBySecondarySkillID(4416) = %v, want RaceDummy (not a secondary marker)", got)
	}
	if got := RaceBySecondarySkillID(1); got != RaceDummy {
		t.Fatalf("RaceBySecondarySkillID(1) = %v, want RaceDummy", got)
	}
}

func TestRaceByOrdinal(t *testing.T) {
	got, ok := RaceByOrdinal(13)
	if !ok || got != RaceFairy {
		t.Fatalf("RaceByOrdinal(13) = %v, %v, want RaceFairy, true", got, ok)
	}
	if _, ok := RaceByOrdinal(-1); ok {
		t.Fatal("RaceByOrdinal(-1) ok = true, want false")
	}
	if _, ok := RaceByOrdinal(len(raceNames)); ok {
		t.Fatal("RaceByOrdinal(len) ok = true, want false")
	}
}

// ---- from template_test.go ----
func TestNewPrivateEntryKeepsIDInFieldError(t *testing.T) {
	for _, missing := range []string{"weight", "respawn"} {
		t.Run(missing, func(t *testing.T) {
			set := commons.NewStatSet()
			set.Set("id", 123)
			set.Set("weight", 1)
			set.Set("respawn", "1sec")
			set.Unset(missing)

			_, err := NewPrivateEntry(set)
			if err == nil || !strings.Contains(err.Error(), "npc: private entry 123") {
				t.Fatalf("NewPrivateEntry() error = %v, want entry id", err)
			}
		})
	}
}

func TestTable_All(t *testing.T) {
	table := NewTable([]*Template{
		{ID: 30, Name: "c"},
		{ID: 10, Name: "a"},
		{ID: 20, Name: "b"},
	})

	all := table.All()
	if len(all) != table.Len() {
		t.Fatalf("All() returned %d templates, Len() = %d", len(all), table.Len())
	}

	var ids []int
	for _, tpl := range all {
		ids = append(ids, tpl.ID)
	}
	if !sort.IntsAreSorted(ids) {
		t.Fatalf("All() not sorted ascending by ID: %v", ids)
	}
	if ids[0] != 10 || ids[len(ids)-1] != 30 {
		t.Fatalf("All() ids = %v, want [10 20 30]", ids)
	}
}

func TestInTerritoryIsStrict3D(t *testing.T) {
	// Spawn.isInMyTerritory uses Location.isIn3DRadius: distance3D < MAX_DRIFT_RANGE.
	// Axis-aligned integer offsets make sqrt(d²) == d, so 199 / 200 / 201 are
	// the exact representable neighbors of the boundary. Pure-Z cases fail
	// if InTerritory drops to 2D.
	home := location.Location{X: 100, Y: 0, Z: 0}
	cases := []struct {
		axis   string
		offset int
		want   bool
	}{
		{axis: "x", offset: defaultDriftRange - 1, want: true},
		{axis: "x", offset: defaultDriftRange, want: false},
		{axis: "x", offset: defaultDriftRange + 1, want: false},
		{axis: "z", offset: defaultDriftRange - 1, want: true},
		{axis: "z", offset: defaultDriftRange, want: false},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s=%d", tc.axis, tc.offset), func(t *testing.T) {
			hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
			hostile.Instance.HasHome = true
			hostile.Instance.Home = home
			x, y, z := home.X, home.Y, home.Z
			switch tc.axis {
			case "x":
				x += tc.offset
			case "z":
				z += tc.offset
			}
			world.New().Spawn(hostile, x, y, z, 0)

			if got := hostile.InTerritory(); got != tc.want {
				t.Fatalf("InTerritory() = %v at 3D %s distance %d, want %v", got, tc.axis, tc.offset, tc.want)
			}
		})
	}
}

func TestReturnHomeAtExactTerritoryBoundaryWalksBack(t *testing.T) {
	home := location.Location{X: 100, Y: 0, Z: 0}
	movement := &hostileMove{}
	hostile := newTestHostile(t, movement, &hostileAttack{})
	hostile.Instance.Kind = "Monster"
	hostile.Instance.HasHome = true
	hostile.Instance.Home = home
	world.New().Spawn(hostile, home.X+defaultDriftRange, home.Y, home.Z, 0)

	if hostile.InTerritory() {
		t.Fatal("InTerritory() = true at 3D distance 200, want false")
	}
	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false at same-Z offset 200, want walk-back")
	}
	if movement.home != home {
		t.Fatalf("MoveHome destination = %#v, want %#v", movement.home, home)
	}
}

func TestReturnHomeDriftRangeIsStrict2D(t *testing.T) {
	// Point2D.isIn2DRadius is distance2D < radius. Axis-aligned integer
	// offsets make hypot(d, 0) == d, so d-1 / d / d+1 are the exact
	// representable neighbors of the boundary.
	//
	// Ordinary Attackable and Guard also gate on 3D territory first. Lift
	// Z so that check is already false and the 2D drift predicate is the
	// one under test. SiegeGuard skips territory.
	const aboveTerritory = 1000
	home := location.Location{X: 100, Y: 0, Z: 0}

	cases := []struct {
		kind     InstanceKind
		offset   int
		z        int
		wantHome bool
	}{
		{kind: "Monster", offset: defaultDriftRange - 1, z: aboveTerritory, wantHome: false},
		{kind: "Monster", offset: defaultDriftRange, z: aboveTerritory, wantHome: true},
		{kind: "Monster", offset: defaultDriftRange + 1, z: aboveTerritory, wantHome: true},
		{kind: "Guard", offset: 19, z: aboveTerritory, wantHome: false},
		{kind: "Guard", offset: 20, z: aboveTerritory, wantHome: true},
		{kind: "Guard", offset: 21, z: aboveTerritory, wantHome: true},
		{kind: "SiegeGuard", offset: 19, z: 0, wantHome: false},
		{kind: "SiegeGuard", offset: 20, z: 0, wantHome: true},
		{kind: "SiegeGuard", offset: 21, z: 0, wantHome: true},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/%d", tc.kind, tc.offset), func(t *testing.T) {
			movement := &hostileMove{}
			hostile := newTestHostile(t, movement, &hostileAttack{})
			hostile.Instance.Kind = tc.kind
			hostile.Instance.HasHome = true
			hostile.Instance.Home = home
			world.New().Spawn(hostile, home.X+tc.offset, home.Y, tc.z, 0)

			got := hostile.ReturnHome()
			if got != tc.wantHome {
				t.Fatalf("ReturnHome() = %v, want %v", got, tc.wantHome)
			}
			if tc.wantHome {
				if movement.home != home {
					t.Fatalf("MoveHome destination = %#v, want %#v", movement.home, home)
				}
				return
			}
			if movement.home != (location.Location{}) {
				t.Fatalf("MoveHome destination = %#v, want no walk-back", movement.home)
			}
		})
	}
}

func TestSiegeGuardReturnHomeBypassesTerritoryGate(t *testing.T) {
	movement := &hostileMove{}
	hostile := newTestHostile(t, movement, &hostileAttack{})
	hostile.Instance.Kind = "SiegeGuard"
	hostile.Instance.HasHome = true
	hostile.Instance.Home = location.Location{}
	world.New().Spawn(hostile, 100, 0, 0, 0)

	if !hostile.InTerritory() {
		t.Fatal("InTerritory() = false at 100 units, want true for the global 200-unit territory")
	}
	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false, want SiegeGuard to return outside its 20-unit drift range")
	}
	if got := movement.home; got != hostile.Instance.Home {
		t.Fatalf("MoveHome destination = %#v, want %#v", got, hostile.Instance.Home)
	}
}

func TestReturnHomeForceWalkStanceBroadcast(t *testing.T) {
	movement := &hostileMove{}
	hostile := newTestHostile(t, movement, &hostileAttack{})
	hostile.SetFrameBuilder(serverpackets.NpcFrameBuilder{})
	w := world.New()
	observer := &frameReceiver{trackedID: 999}
	w.Spawn(hostile, 100, 0, 0, 0)
	w.Spawn(observer, 50, 0, 0, 0)
	hostile.SetWorld(w)
	hostile.Instance.HasHome = true
	hostile.Instance.Home = location.Location{X: 100, Y: 0, Z: 0}
	hostile.Instance.Template.RunSpeed = 120
	hostile.Instance.Template.WalkSpeed = 60
	hostile.SetXYZ(100, 500, 0)

	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false, want true outside drift range")
	}
	if hostile.Running() {
		t.Fatal("Running() = true after ordinary ReturnHome, want walk stance")
	}
	if len(observer.frames) != 1 {
		t.Fatalf("observer frame count = %d, want 1 ChangeMoveType", len(observer.frames))
	}
	assertChangeMoveTypeFrame(t, observer.frames[0], hostile.ObjectID(), false)
}

func TestReturnHomeRechecksWanderBehindActor(t *testing.T) {
	movement := &hostileMove{moved: make(chan location.Location, 1)}
	hostile := newTestHostile(t, movement, &hostileAttack{})
	hostile.Instance.HasHome = true
	hostile.Instance.Home = location.Location{X: 100, Y: 0, Z: 0}
	hostile.Instance.Template.WalkSpeed = 100
	hostile.Instance.Template.CollisionRadius = 30
	hostile.roll = func(int) int { return 0 }
	world.New().Spawn(hostile, 100, 500, 0, 0)
	hostile.SetHeading(0)
	hostile.AI().SetWander()

	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false, want true outside drift range")
	}

	select {
	case got := <-movement.moved:
		if want := (location.Location{X: 50, Y: 500, Z: 0}); got != want {
			t.Fatalf("wander recheck target = %#v, want %#v", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wander recheck did not move behind the actor")
	}
}

func TestGrandBossReturnHomeNeverWalksBack(t *testing.T) {
	// GrandBoss.returnHome is unconditionally false. A boss spawned
	// outside drift range must not MoveHome or take the Attackable
	// delayed backward nudge.
	movement := &hostileMove{moved: make(chan location.Location, 1)}
	hostile := newTestHostile(t, movement, &hostileAttack{})
	hostile.Instance.Kind = "GrandBoss"
	hostile.Instance.HasHome = true
	hostile.Instance.Home = location.Location{X: 100, Y: 0, Z: 0}
	hostile.Instance.Template.WalkSpeed = 100
	hostile.roll = func(int) int { return 0 }
	world.New().Spawn(hostile, 100, 500, 0, 0)
	hostile.AI().SetWander()

	if hostile.ReturnHome() {
		t.Fatal("ReturnHome() = true, want false for GrandBoss")
	}
	if movement.home != (location.Location{}) {
		t.Fatalf("MoveHome destination = %#v, want no walk-back", movement.home)
	}
	select {
	case <-movement.moved:
		t.Fatal("GrandBoss wander recheck moved behind the actor")
	case <-time.After(2 * time.Second):
	}
}

func TestSiegeGuardReturnHomeDoesNotRecheckWander(t *testing.T) {
	movement := &hostileMove{moved: make(chan location.Location, 1)}
	hostile := newTestHostile(t, movement, &hostileAttack{})
	hostile.Instance.Kind = "SiegeGuard"
	hostile.Instance.HasHome = true
	hostile.Instance.Home = location.Location{X: 100, Y: 0, Z: 0}
	hostile.Instance.Template.RunSpeed = 100
	hostile.roll = func(int) int { return 0 }
	world.New().Spawn(hostile, 100, 500, 0, 0)
	hostile.AI().SetWander()

	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false, want true outside drift range")
	}
	select {
	case <-movement.moved:
		t.Fatal("SiegeGuard wander recheck moved behind the actor")
	case <-time.After(2 * time.Second):
	}
}

func TestReturnHomeScalesWanderRecheckDelayForFastNPC(t *testing.T) {
	movement := &hostileMove{moved: make(chan location.Location, 1)}
	hostile := newTestHostile(t, movement, &hostileAttack{})
	hostile.Instance.HasHome = true
	hostile.Instance.Home = location.Location{X: 100, Y: 0, Z: 0}
	hostile.Instance.Template.WalkSpeed = 200
	hostile.roll = func(int) int { return 0 }
	world.New().Spawn(hostile, 100, 500, 0, 0)
	hostile.AI().SetWander()

	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false, want true outside drift range")
	}
	select {
	case <-movement.moved:
		t.Fatal("wander recheck fired before the scaled delay")
	case <-time.After(100 * time.Millisecond):
	}
	select {
	case <-movement.moved:
	case <-time.After(time.Second):
		t.Fatal("wander recheck did not fire after the scaled delay")
	}
}

func TestSiegeGuardReturnHomeForceRunStanceBroadcast(t *testing.T) {
	movement := &hostileMove{}
	hostile := newTestHostile(t, movement, &hostileAttack{})
	hostile.SetFrameBuilder(serverpackets.NpcFrameBuilder{})
	hostile.Instance.Kind = "SiegeGuard"
	w := world.New()
	observer := &frameReceiver{trackedID: 999}
	w.Spawn(hostile, 100, 0, 0, 0)
	w.Spawn(observer, 50, 0, 0, 0)
	hostile.SetWorld(w)
	hostile.Instance.HasHome = true
	hostile.Instance.Home = location.Location{X: 100, Y: 0, Z: 0}
	hostile.Instance.Template.RunSpeed = 120
	hostile.Instance.Template.WalkSpeed = 60
	hostile.SetRunning(false)
	hostile.SetXYZ(100, 50, 0)

	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false, want true outside SiegeGuard drift range")
	}
	if !hostile.Running() {
		t.Fatal("Running() = false after SiegeGuard ReturnHome, want run stance")
	}
	if len(observer.frames) != 1 {
		t.Fatalf("observer frame count = %d, want 1 ChangeMoveType", len(observer.frames))
	}
	assertChangeMoveTypeFrame(t, observer.frames[0], hostile.ObjectID(), true)
}

func assertChangeMoveTypeFrame(t *testing.T, frame []byte, objectID int32, running bool) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeChangeMoveType {
		t.Fatalf("opcode = %#x, want ChangeMoveType (%#x)", frame[0], serverpackets.OpcodeChangeMoveType)
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != objectID {
		t.Fatalf("ChangeMoveType object id = %d, want %d", got, objectID)
	}
	wantRun := int32(0)
	if running {
		wantRun = 1
	}
	if got := r.ReadInt32(); got != wantRun {
		t.Fatalf("ChangeMoveType running = %d, want %d", got, wantRun)
	}
	if got := r.ReadInt32(); got != 0 {
		t.Fatalf("ChangeMoveType swimming = %d, want 0", got)
	}
}

func TestNewHostileAppliesTemplatePassivesBeforeHPSeed(t *testing.T) {
	baseTpl := &Template{ID: 9001, Type: "Monster", Level: 20, HPMax: 1000, CON: 40}
	base, err := NewHostile(&Instance{ObjectID: 1, Template: baseTpl, Kind: "Monster"}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatal(err)
	}

	passive := modelskill.Ref{ID: 99, Level: 1}
	table := modelskill.NewTable([]modelskill.Definition{{
		ID:         passive.ID,
		Level:      passive.Level,
		Activation: modelskill.ActivationActive,
		Funcs:      []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "maxHp", Value: 250}},
	}})
	tpl := &Template{ID: 9001, Type: "Monster", Level: 20, HPMax: 1000, CON: 40, Passives: []modelskill.Ref{passive}}
	got, err := NewHostile(&Instance{ObjectID: 2, Template: tpl, Kind: "Monster"}, newHostileLive(t), &hostileMove{}, &hostileAttack{}, table)
	if err != nil {
		t.Fatal(err)
	}

	wantMax := base.MaxHP() + 250
	if got.MaxHP() != wantMax {
		t.Fatalf("MaxHP() = %d, want %d (base %d + 250 add after CON mul)", got.MaxHP(), wantMax, base.MaxHP())
	}
	if got.CurrentHP() != wantMax {
		t.Fatalf("CurrentHP() = %d, want %d (seed after funcs attach)", got.CurrentHP(), wantMax)
	}
}

func TestNewHostileRejectsFolkEvenWithTemplatePassives(t *testing.T) {
	table := modelskill.NewTable([]modelskill.Definition{{
		ID:         99,
		Level:      1,
		Activation: modelskill.ActivationActive,
		Funcs:      []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "maxHp", Value: 250}},
	}})
	_, err := NewHostile(&Instance{
		ObjectID: 1,
		Template: &Template{
			ID:       3001,
			Type:     "Folk",
			HPMax:    1000,
			Passives: []modelskill.Ref{{ID: 99, Level: 1}},
		},
	}, newHostileLive(t), &hostileMove{}, &hostileAttack{}, table)
	if err == nil {
		t.Fatal("NewHostile() error = nil, want rejection of Folk so template passives never attach")
	}
}

func TestNewHostileFailsOnTemplatePassiveBuildError(t *testing.T) {
	table := modelskill.NewTable([]modelskill.Definition{{
		ID:    99,
		Level: 1,
		Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "notAStat", Value: 1}},
	}})
	_, err := NewHostile(&Instance{
		ObjectID: 1,
		Template: &Template{
			ID:       9001,
			Type:     "Monster",
			HPMax:    1000,
			Passives: []modelskill.Ref{{ID: 99, Level: 1}},
		},
		Kind: "Monster",
	}, newHostileLive(t), &hostileMove{}, &hostileAttack{}, table)
	if err == nil {
		t.Fatal("NewHostile() error = nil, want template-passive build error")
	}
}

func partyAI(partyType, loyalty int) *commons.StatSet {
	set := commons.NewStatSet()
	set.Set("Party_Type", partyType)
	if loyalty != 0 {
		set.Set("Party_Loyalty", loyalty)
	}
	return set
}

func partyHostile(t *testing.T, id int32, partyType int, move ai.MoveController) *Hostile {
	t.Helper()
	tpl := &Template{
		ID: int(id), Type: "Monster", HPMax: 1000, CanMove: true, RunSpeed: 120,
		AIParams: partyAI(partyType, 0),
	}
	h, err := NewHostile(&Instance{ObjectID: id, Template: tpl, Kind: "Monster"}, newHostileLive(t), move, &hostileAttack{})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestMinionAssistsWhenMasterTakesDamage(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	minion := partyHostile(t, 2, 1, &hostileMove{})
	attacker := partyHostile(t, 3, 0, &hostileMove{})
	master.AddMinion(minion)
	minion.SetMaster(master)

	master.TakeDamage(40, attacker)

	d, ok := minion.AI().Desires().Peek()
	if !ok || d.Kind != ai.IntentionAttack || d.FinalTarget != attacker {
		t.Fatalf("minion desire = (%v, %v), want attack on attacker", ok, d)
	}
	if got := d.Weight; got != 40 {
		t.Fatalf("minion attack weight = %v, want 40 (damage * party weight 1)", got)
	}
	if !d.MoveToTarget {
		t.Fatal("moving party assist MoveToTarget = false, want true")
	}
}

func TestMasterDoesNotGainPartyDesireWhenMinionTakesDamage(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	minion := partyHostile(t, 2, 1, &hostileMove{})
	attacker := partyHostile(t, 3, 0, &hostileMove{})
	master.AddMinion(minion)
	minion.SetMaster(master)

	minion.TakeDamage(40, attacker)

	if got := master.AI().Desires().Len(); got != 0 {
		t.Fatalf("master desires = %d, want 0 (Party_Type 2 without a master does not assist)", got)
	}
	d, ok := minion.AI().Desires().Peek()
	if !ok || d.Kind != ai.IntentionAttack {
		t.Fatalf("damaged minion desire = (%v, %v), want its own attack desire", ok, d)
	}
}

func TestSiblingMinionAssistsWhenPartyMemberTakesDamage(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	one := partyHostile(t, 2, 1, &hostileMove{})
	two := partyHostile(t, 3, 1, &hostileMove{})
	attacker := partyHostile(t, 4, 0, &hostileMove{})
	master.AddMinion(one)
	master.AddMinion(two)
	one.SetMaster(master)
	two.SetMaster(master)

	one.TakeDamage(25, attacker)

	d, ok := two.AI().Desires().Peek()
	if !ok || d.Kind != ai.IntentionAttack || d.FinalTarget != attacker {
		t.Fatalf("sibling desire = (%v, %v), want attack on attacker", ok, d)
	}
}

func TestLoyaltyTwoMinionAssistsOnlyWhenMasterIsCaller(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	loyal := partyHostile(t, 2, 1, &hostileMove{})
	sibling := partyHostile(t, 3, 1, &hostileMove{})
	attacker := partyHostile(t, 4, 0, &hostileMove{})
	loyal.Instance.Template.AIParams = partyAI(1, 2)
	master.AddMinion(loyal)
	master.AddMinion(sibling)
	loyal.SetMaster(master)
	sibling.SetMaster(master)

	sibling.TakeDamage(25, attacker)
	if got := loyal.AI().Desires().Len(); got != 0 {
		t.Fatalf("loyalty-2 minion desires after sibling hit = %d, want 0", got)
	}

	master.TakeDamage(25, attacker)
	d, ok := loyal.AI().Desires().Peek()
	if !ok || d.Kind != ai.IntentionAttack || d.FinalTarget != attacker {
		t.Fatalf("loyalty-2 minion desire after master hit = (%v, %v), want attack on attacker", ok, d)
	}
}

func TestNotifyAggressionFansOutToMinions(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	minion := partyHostile(t, 2, 1, &hostileMove{})
	attacker := partyHostile(t, 3, 0, &hostileMove{})
	master.AddMinion(minion)
	minion.SetMaster(master)

	master.NotifyAggression(attacker, 80)

	d, ok := minion.AI().Desires().Peek()
	if !ok || d.Kind != ai.IntentionAttack || d.FinalTarget != attacker {
		t.Fatalf("minion desire after NotifyAggression = (%v, %v), want attack on attacker", ok, d)
	}
}

func spawnPartyWorld(t *testing.T, actors ...*Hostile) *world.State {
	t.Helper()
	state := world.New()
	for i, actor := range actors {
		actor.SetWorld(state)
		state.Spawn(actor, i*100, 0, 0, 0)
	}
	return state
}

func TestStationaryMinionHoldsAttackWhenPlayableInRange(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	minion := partyHostile(t, 2, 1, &hostileMove{})
	minion.Instance.Template.AIParams.Set("MovingAttack", 0)
	minion.Instance.Template.AggroRange = 500
	master.AddMinion(minion)
	minion.SetMaster(master)
	state := world.New()
	master.SetWorld(state)
	minion.SetWorld(state)
	state.Spawn(master, 0, 0, 0, 0)
	state.Spawn(minion, 10, 0, 0, 0)
	attacker := &hostileTarget{id: 99}
	state.Spawn(attacker, 20, 0, 0, 0)

	master.TakeDamage(40, attacker)

	d, ok := minion.AI().Desires().Peek()
	if !ok || d.Kind != ai.IntentionAttack || d.FinalTarget.ObjectID() != attacker.ObjectID() {
		t.Fatalf("minion desire = (%v, %+v), want hold attack on playable", ok, d)
	}
	if d.MoveToTarget {
		t.Fatal("stationary party assist MoveToTarget = true, want false")
	}
	if got := d.Weight; got != 40 {
		t.Fatalf("hold attack weight = %v, want 40", got)
	}
}

func TestStationaryMinionDropsAttackWhenPlayableOutOfRangeAndIsTopDesire(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	minion := partyHostile(t, 2, 1, &hostileMove{})
	minion.Instance.Template.AIParams.Set("MovingAttack", 0)
	minion.Instance.Template.AggroRange = 20
	master.AddMinion(minion)
	minion.SetMaster(master)
	state := world.New()
	master.SetWorld(state)
	minion.SetWorld(state)
	state.Spawn(master, 0, 0, 0, 0)
	state.Spawn(minion, 0, 0, 0, 0)
	attacker := &hostileTarget{id: 99}
	state.Spawn(attacker, 400, 0, 0, 0)

	minion.AddCombatDamageHate(attacker, 10)
	if got := minion.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() before assist = %v, want Attack so the playable is top desire", got)
	}

	master.TakeDamage(40, attacker)

	if got := minion.AI().Desires().Len(); got != 0 {
		t.Fatalf("desires after out-of-range hold assist = %d, want 0 (top desire dropped)", got)
	}
}

func TestMovingMinionTeleportsToTargetAfterGeoPathFails(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	minion := partyHostile(t, 2, 1, &hostileMove{})
	attacker := partyHostile(t, 3, 0, &hostileMove{})
	master.AddMinion(minion)
	minion.SetMaster(master)
	spawnPartyWorld(t, master, minion, attacker)

	minion.AddCombatDamageHate(attacker, 10)
	if got := minion.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() before assist = %v, want Attack", got)
	}
	minion.SetHP(minion.MaxHPValue() / 2)
	for i := 0; i < 11; i++ {
		minion.AddGeoPathFailCount()
	}

	master.TakeDamage(40, attacker)

	mx, my, _ := minion.Position()
	ax, ay, _ := attacker.Position()
	if mx != ax || my != ay {
		t.Fatalf("minion position = (%d,%d), want attacker (%d,%d) after geo-fail teleport", mx, my, ax, ay)
	}
	if got := minion.GeoPathFailCount(); got != 0 {
		t.Fatalf("GeoPathFailCount() after teleport = %d, want 0", got)
	}
}

func TestMovingMinionRootedRetryRequeuesAttack(t *testing.T) {
	move := &hostileMove{}
	master := partyHostile(t, 1, 2, &hostileMove{})
	minion := partyHostile(t, 2, 1, move)
	attacker := partyHostile(t, 3, 0, &hostileMove{})
	master.AddMinion(minion)
	minion.SetMaster(master)
	spawnPartyWorld(t, master, minion, attacker)

	minion.AddCombatDamageHate(attacker, 10)
	if got := minion.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() before assist = %v, want Attack", got)
	}
	addHostileEffect(t, minion, "Root")
	if !minion.Rooted() {
		t.Fatal("Rooted() = false after Root effect, want true")
	}

	master.TakeDamage(40, attacker)

	d, ok := minion.AI().Desires().Peek()
	if !ok || d.Kind != ai.IntentionAttack || d.FinalTarget != attacker {
		t.Fatalf("minion desire after rooted retry = (%v, %v), want requeued attack", ok, d)
	}
	if got := d.Weight; got != 40 {
		t.Fatalf("rooted retry weight = %v, want 40 (cleared then requeued at party damage)", got)
	}
	if move.stopCount == 0 {
		t.Fatal("move.Stop() count = 0, want stop when dropping the out-of-range top desire")
	}
}

func TestMinionThinkFollowMovesToEscortSlot(t *testing.T) {
	masterMove := &hostileMove{}
	minionMove := &hostileMove{}
	master := partyHostile(t, 1, 2, masterMove)
	minion := partyHostile(t, 2, 1, minionMove)
	state := world.New()
	master.SetWorld(state)
	minion.SetWorld(state)
	state.Spawn(master, 1000, 1000, 0, 0)
	state.Spawn(minion, 0, 0, 0, 0)
	master.AddMinion(minion)
	minion.SetMaster(master)
	minion.roll = func(n int) int { return 0 }

	if minion.ThinkFollow(master, false) {
		t.Fatal("ThinkFollow() clearDesire = true, want follow to continue")
	}
	if len(minionMove.locations) != 1 {
		t.Fatalf("escort MoveToLocation count = %d, want 1", len(minionMove.locations))
	}
	got := minionMove.locations[0]
	if math.Abs(got.Distance2D(location.Location{X: 1000, Y: 1000, Z: 0})-150) > 1 {
		t.Fatalf("escort dest %v is not 150 from master", got)
	}
}

func TestMinionThinkFollowLooseMovesTowardNonMaster(t *testing.T) {
	move := &hostileMove{}
	follower := partyHostile(t, 1, 1, move)
	target := partyHostile(t, 2, 0, &hostileMove{})
	state := world.New()
	follower.SetWorld(state)
	target.SetWorld(state)
	state.Spawn(follower, 0, 0, 0, 0)
	state.Spawn(target, 400, 0, 0, 0)
	n := 0
	follower.roll = func(bound int) int {
		n++
		switch n {
		case 1:
			return 51
		case 2:
			return 250000
		case 3:
			return 0
		default:
			t.Fatalf("unexpected roll call %d bound %d", n, bound)
			return 0
		}
	}

	if follower.ThinkFollow(target, false) {
		t.Fatal("ThinkFollow() clearDesire = true, want follow to continue")
	}
	if len(move.locations) != 1 {
		t.Fatalf("loose-follow MoveToLocation count = %d, want 1", len(move.locations))
	}
	got := move.locations[0]
	want := location.Location{X: 550, Y: 0, Z: 0}
	if got != want {
		t.Fatalf("loose-follow dest = %v, want %v (sqrt(0.25)*300 at angle 0 from target)", got, want)
	}
}

func TestMinionThinkFollowTeleportsAfterGeoPathFails(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	minion := partyHostile(t, 2, 1, &hostileMove{})
	state := world.New()
	master.SetWorld(state)
	minion.SetWorld(state)
	state.Spawn(master, 500, 0, 0, 0)
	state.Spawn(minion, 0, 0, 0, 0)
	master.AddMinion(minion)
	minion.SetMaster(master)
	for i := 0; i < 10; i++ {
		minion.AddGeoPathFailCount()
	}

	if minion.ThinkFollow(master, true) {
		t.Fatal("ThinkFollow() clearDesire = true after geo fail, want follow kept")
	}
	x, y, _ := minion.Position()
	if x == 0 && y == 0 {
		t.Fatal("minion still at origin after geo-fail teleport, want near master")
	}
	if got := minion.GeoPathFailCount(); got != 0 {
		t.Fatalf("GeoPathFailCount() after teleport = %d, want 0", got)
	}
}

func TestIdlePartyPrivateQueuesFollowOnThink(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	minion := partyHostile(t, 2, 1, &hostileMove{})
	master.AddMinion(minion)
	minion.SetMaster(master)

	if err := minion.Think(); err != nil {
		t.Fatal(err)
	}
	if got := minion.AI().CurrentIntention(); got != ai.IntentionFollow {
		t.Fatalf("CurrentIntention() = %v, want %v", got, ai.IntentionFollow)
	}
}

type overhitActor int32

func (a overhitActor) ObjectID() int32 { return int32(a) }

type overhitSummon struct {
	id    int32
	owner creature.DeathActor
}

func (s overhitSummon) ObjectID() int32 { return s.id }
func (s overhitSummon) ActingPlayer() creature.DeathActor {
	return s.owner
}

func TestOverhitBonusExpOracle(t *testing.T) {
	attacker := overhitActor(1)
	other := overhitActor(2)

	t.Run("no overhit", func(t *testing.T) {
		var s overhitState
		if s.valid(attacker) {
			t.Fatal("valid() = true with no overhit, want false")
		}
	})

	t.Run("valid overhit", func(t *testing.T) {
		var s overhitState
		s.set(true)
		s.test(attacker, 100, 110)
		if !s.valid(attacker) {
			t.Fatal("valid(attacker) = false, want true")
		}
		// excess 10 / maxHP 200 = 5% of 1000 = 50
		if got := s.bonusExp(1000, 200); got != 50 {
			t.Fatalf("bonusExp() = %d, want 50", got)
		}
	})

	t.Run("attacker mismatch", func(t *testing.T) {
		var s overhitState
		s.set(true)
		s.test(attacker, 100, 110)
		if s.valid(other) {
			t.Fatal("valid(other) = true, want false")
		}
	})

	t.Run("25 percent cap", func(t *testing.T) {
		var s overhitState
		s.set(true)
		s.test(attacker, 100, 200)
		// excess 100 / maxHP 200 = 50% capped at 25% of 1000 = 250
		if got := s.bonusExp(1000, 200); got != 250 {
			t.Fatalf("bonusExp() = %d, want 250", got)
		}
	})

	t.Run("half-unit rounds up", func(t *testing.T) {
		var s overhitState
		s.set(true)
		s.test(attacker, 10, 11)
		// excess 1 / maxHP 200 = 0.5% of 100 = 0.5 → 1
		if got := s.bonusExp(100, 200); got != 1 {
			t.Fatalf("bonusExp() = %d, want 1", got)
		}
	})

	t.Run("non-lethal clears", func(t *testing.T) {
		var s overhitState
		s.set(true)
		s.test(attacker, 100, 40)
		if s.valid(attacker) {
			t.Fatal("valid() = true after a non-lethal hit, want false")
		}
	})

	t.Run("zero damage clears", func(t *testing.T) {
		var s overhitState
		s.set(true)
		s.test(attacker, 100, 0)
		if s.valid(attacker) {
			t.Fatal("valid() = true after zero damage, want false")
		}
	})

	t.Run("summon acting player", func(t *testing.T) {
		var s overhitState
		s.set(true)
		s.test(overhitSummon{id: 9, owner: attacker}, 100, 110)
		if !s.valid(attacker) {
			t.Fatal("valid(owner) = false for a summon overhit, want true")
		}
		if s.valid(other) {
			t.Fatal("valid(other) = true for a summon overhit, want false")
		}
	})
}

type hitNight bool

func (n hitNight) IsNight() bool { return bool(n) }

func TestMakeAttackHitAppliesFacingAndNight(t *testing.T) {
	t.Cleanup(func() { creature.SetNightSource(nil) })

	tpl := &Template{ID: 1, Type: "Monster"}
	place := func(t *testing.T, ax, ay int) (*Hostile, *Hostile) {
		t.Helper()
		state := world.New()
		target := newCombatHostile(t, 1, tpl)
		attacker := newCombatHostile(t, 2, tpl)
		state.Spawn(target, 0, 0, 0, 0)
		state.Spawn(attacker, ax, ay, 0, 0)
		return attacker, target
	}

	attacker, target := place(t, 100, 0)
	acc := int(attacker.calcStat(stat.AccuracyCombat, 0))
	eva := target.Evasion()
	frontRate := formulas.HitRate(acc, eva, 0, false, false, true)
	behindRate := formulas.HitRate(acc, eva, 0, false, true, false)
	nightRate := formulas.HitRate(acc, eva, 0, true, false, true)
	if frontRate >= behindRate {
		t.Fatalf("need positional rate gap, front=%d behind=%d", frontRate, behindRate)
	}
	if nightRate >= frontRate {
		t.Fatalf("need night rate gap, night=%d front=%d", nightRate, frontRate)
	}
	posRoll := (frontRate + behindRate) / 2
	nightRoll := (nightRate + frontRate) / 2

	tests := []struct {
		name     string
		ax, ay   int
		night    bool
		roll     int
		wantMiss bool
	}{
		{"front day misses between front and behind rates", 100, 0, false, posRoll, true},
		{"behind day hits between front and behind rates", -100, 0, false, posRoll, false},
		{"side day hits between front and behind rates", 0, 100, false, posRoll, false},
		{"front night misses between night and front rates", 100, 0, true, nightRoll, true},
		{"front day hits the night-gap roll", 100, 0, false, nightRoll, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creature.SetNightSource(hitNight(tt.night))
			attacker, target := place(t, tt.ax, tt.ay)
			attacker.SetRollSource(func(int) int { return tt.roll })
			hit := attacker.MakeAttackHit(target, false)
			if hit.Miss != tt.wantMiss {
				t.Fatalf("Miss = %v, want %v (roll %d acc %d eva %d)", hit.Miss, tt.wantMiss, tt.roll, acc, eva)
			}
		})
	}
}

func TestMakeAttackHitAppliesRacePosAndIgnoresPvP(t *testing.T) {
	tpl := &Template{ID: 1, Type: "Monster", Race: RaceBeast, PAtk: 100, PDef: 50, CritRate: 0, DEX: 30}
	state := world.New()
	target := newCombatHostile(t, 1, tpl)
	attacker := newCombatHostile(t, 2, tpl)
	state.Spawn(target, 0, 0, 0, 0)
	state.Spawn(attacker, -100, 0, 0, 0)
	attacker.SetRollSource(func(bound int) int {
		if bound == 1000 {
			return 0
		}
		return (bound - 1) / 2
	})
	attacker.AddStatFuncs([]effect.Mod{
		{Stat: stat.PAtkBeasts, Op: effect.OpSet, Value: 50, Owner: effect.ModOwnerEffect(&effect.Effect{})},
		{Stat: stat.PvPPhysicalDmg, Op: effect.OpMul, Value: 3, Owner: effect.ModOwnerEffect(&effect.Effect{})},
	})

	hit := attacker.MakeAttackHit(target, false)
	if hit.Miss || hit.Crit {
		t.Fatalf("hit miss=%v crit=%v, want connected non-crit", hit.Miss, hit.Crit)
	}
	want := int(formulas.PhysicalAttackDamage(formulas.PhysicalAttackInput{
		AttackPower: attacker.PAtk(), Defence: creature.Positive(target.PDef()),
		PosMul: 1.2, ElementalMul: 1, RandomMul: 1, RaceMul: target.RaceMultiplier(attacker),
		WeaponVulnMul: 1, PvPMul: 1,
	}))
	if hit.Damage != want {
		t.Fatalf("damage = %d, want %d (race %v)", hit.Damage, want, target.RaceMultiplier(attacker))
	}
	if target.RaceMultiplier(attacker) != 1.49 {
		t.Fatalf("RaceMultiplier = %v, want 1.49", target.RaceMultiplier(attacker))
	}
}
