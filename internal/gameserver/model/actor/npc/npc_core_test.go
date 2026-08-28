package npc

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

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
