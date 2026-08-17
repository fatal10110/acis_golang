package npc

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

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
