package npc

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

func TestHostileMaxBuffCountIncludesTemplateDivineInspiration(t *testing.T) {
	hostile := newCombatHostile(t, 1, &Template{
		ID:     1,
		Type:   "Monster",
		Skills: map[int]int{int(skill.DivineInspirationSkillID): 3},
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

// TestHostileHealInputScalesShotsAsNPC pins an NPC healer's side of a HEAL:
// its M.Atk and heal proficiency, and the NPC shot scaling that quadruples
// the M.Atk term under either spiritshot.
func TestHostileHealInputScalesShotsAsNPC(t *testing.T) {
	hostile, err := NewHostile(&Instance{
		ObjectID: 101,
		Template: &Template{ID: 9001, Type: "Monster", Level: 20, MAtk: 64},
		Kind:     "Monster",
	}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatal(err)
	}
	hostile.AddStatFuncs([]effect.Mod{{Stat: stat.HealProficiency, Op: effect.OpAdd, Value: 5}})

	in, ok := hostile.HealInput(skill.Definition{SkillType: "HEAL", Power: 30})
	if !ok {
		t.Fatal("HealInput() ok = false")
	}
	if in.Power != 30 || in.Proficiency != 5 || in.Static || in.MAtk != int(hostile.MAtk()) || in.Scaling != formulas.HealShotScalingNPC {
		t.Fatalf("HealInput() = %+v, want power 30, proficiency 5, M.Atk %d, NPC scaling", in, int(hostile.MAtk()))
	}
	static, _ := hostile.HealInput(skill.Definition{SkillType: "HEAL_STATIC", Power: 30})
	if got := formulas.HealAmount(static); !static.Static || got != 35 {
		t.Fatalf("HEAL_STATIC input %+v amount %v, want static 35", static, got)
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

// ---- from hostile_stats_golden_test.go ----
// rawPAtk, rawPDef and rawMDef read the finalized stat before the getters
// truncate it, so the pipeline golden stays sensitive to sub-integer float
// drift (func insertion order, association) that truncation would hide.
func rawPAtk(h *Hostile) float64 { return h.calcStat(stat.PowerAttack, h.Instance.Template.PAtk) }

func rawPDef(h *Hostile) float64 { return h.calcStat(stat.PowerDefence, h.Instance.Template.PDef) }

func rawMDef(h *Hostile) float64 {
	return h.calcStat(stat.MagicDefence, positiveStat(h.Instance.Template.MDef))
}

// goldenHostileScenarios is npc.Hostile's half of the stat pipeline parity
// oracle described in issue #1527: same-order funcs attached in different
// sequences (float addition's insertion order is load-bearing), a Set
// rebase, and attach/detach round-tripping, each running through the
// shared builtin finalize step at order 10.
func goldenHostileScenarios(t testing.TB) map[string]float64 {
	t.Helper()
	out := make(map[string]float64)

	tpl := func() *Template {
		return &Template{
			ID: 1, Type: "Monster", Level: 20, STR: 40, CON: 21, DEX: 30, INT: 20, WIT: 43, MEN: 20,
			PAtk: 100, PDef: 50, MAtk: 64, MDef: 40, HPMax: 500, MPMax: 200,
		}
	}

	{
		h1 := newCombatHostile(t, 1, tpl())
		h1.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 1e16}})
		h1.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpSub, Value: 1e16}})
		h1.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 1}})
		out["order30_forward"] = rawPDef(h1)

		h2 := newCombatHostile(t, 2, tpl())
		h2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 1}})
		h2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpSub, Value: 1e16}})
		h2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 1e16}})
		out["order30_reverse"] = rawPDef(h2)
	}

	{
		h := newCombatHostile(t, 3, tpl())
		h.AddStatFuncs([]effect.Mod{
			{Stat: stat.MagicDefence, Op: effect.OpSet, Value: 500},
			{Stat: stat.MagicDefence, Op: effect.OpBaseMul, Value: 0.5},
		})
		out["set_rebase_mdef"] = rawMDef(h)
	}

	{
		h := newCombatHostile(t, 4, tpl())
		base := rawPAtk(h)
		owner := effect.ModOwnerEffect(&effect.Effect{})
		h.AddStatFuncs([]effect.Mod{
			{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 7, Owner: owner},
			{Stat: stat.PowerAttack, Op: effect.OpMul, Value: 1.25, Owner: owner},
		})
		out["attach_detach_before"] = base
		out["attach_detach_during"] = rawPAtk(h)
		h.RemoveStatsByOwner(owner)
		out["attach_detach_after"] = rawPAtk(h)
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

func TestNewHostileAppliesTemplatePassivesBeforeHPSeed(t *testing.T) {
	baseTpl := &Template{ID: 9001, Type: "Monster", Level: 20, HPMax: 1000, CON: 40}
	base, err := NewHostile(&Instance{ObjectID: 1, Template: baseTpl, Kind: "Monster"}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatal(err)
	}

	passive := skill.Ref{ID: 99, Level: 1}
	table := skill.NewTable([]skill.Definition{{
		ID:         passive.ID,
		Level:      passive.Level,
		Activation: skill.ActivationActive,
		Funcs:      []skill.FuncTemplate{{Op: skill.FuncAdd, Stat: "maxHp", Value: 250}},
	}})
	tpl := &Template{ID: 9001, Type: "Monster", Level: 20, HPMax: 1000, CON: 40, Passives: []skill.Ref{passive}}
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
	table := skill.NewTable([]skill.Definition{{
		ID:         99,
		Level:      1,
		Activation: skill.ActivationActive,
		Funcs:      []skill.FuncTemplate{{Op: skill.FuncAdd, Stat: "maxHp", Value: 250}},
	}})
	_, err := NewHostile(&Instance{
		ObjectID: 1,
		Template: &Template{
			ID:       3001,
			Type:     "Folk",
			HPMax:    1000,
			Passives: []skill.Ref{{ID: 99, Level: 1}},
		},
	}, newHostileLive(t), &hostileMove{}, &hostileAttack{}, table)
	if err == nil {
		t.Fatal("NewHostile() error = nil, want rejection of Folk so template passives never attach")
	}
}

func TestNewHostileFailsOnTemplatePassiveBuildError(t *testing.T) {
	table := skill.NewTable([]skill.Definition{{
		ID:    99,
		Level: 1,
		Funcs: []skill.FuncTemplate{{Op: skill.FuncAdd, Stat: "notAStat", Value: 1}},
	}})
	_, err := NewHostile(&Instance{
		ObjectID: 1,
		Template: &Template{
			ID:       9001,
			Type:     "Monster",
			HPMax:    1000,
			Passives: []skill.Ref{{ID: 99, Level: 1}},
		},
		Kind: "Monster",
	}, newHostileLive(t), &hostileMove{}, &hostileAttack{}, table)
	if err == nil {
		t.Fatal("NewHostile() error = nil, want template-passive build error")
	}
}
