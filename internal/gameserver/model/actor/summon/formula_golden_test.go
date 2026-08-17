package summon

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// goldenSummonScenarios is summon.Actor's half of the stat pipeline parity
// oracle described in issue #1527: see the player and npc packages' golden
// tests for the same shape of coverage (same-order attach-sequence
// sensitivity, Set rebasing, attach/detach round-tripping).
func goldenSummonScenarios(t testing.TB) map[string]float64 {
	t.Helper()
	out := make(map[string]float64)

	stats := CombatStats{
		STR: 40, CON: 21, DEX: 30, INT: 20, WIT: 43, MEN: 20,
		PAtk: 100, PDef: 50, MAtk: 64, MDef: 40,
		MaxHP: 500, MaxMP: 200, BaseRandomDamage: 5,
	}

	{
		a1 := NewServitor(ServitorConfig{ObjectID: 1, Level: 44, Stats: stats, Roll: zeroSummonRoll})
		a1.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 1e16}})
		a1.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpSub, Value: 1e16}})
		a1.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 1}})
		out["order30_forward"] = a1.PDef()

		a2 := NewServitor(ServitorConfig{ObjectID: 2, Level: 44, Stats: stats, Roll: zeroSummonRoll})
		a2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 1}})
		a2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpSub, Value: 1e16}})
		a2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 1e16}})
		out["order30_reverse"] = a2.PDef()
	}

	{
		a := NewPet(PetConfig{ObjectID: 3, Level: 44, Stats: stats, Roll: zeroSummonRoll})
		a.AddStatFuncs([]effect.Mod{
			{Stat: stat.MagicDefence, Op: effect.OpSet, Value: 500},
			{Stat: stat.MagicDefence, Op: effect.OpBaseMul, Value: 0.5},
		})
		out["set_rebase_mdef"] = a.MDef()
	}

	{
		a := NewPet(PetConfig{ObjectID: 4, Level: 44, Stats: stats, Roll: zeroSummonRoll})
		base := a.PAtk()
		owner := effect.ModOwnerEffect(&effect.Effect{})
		a.AddStatFuncs([]effect.Mod{
			{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 7, Owner: owner},
			{Stat: stat.PowerAttack, Op: effect.OpMul, Value: 1.25, Owner: owner},
		})
		out["attach_detach_before"] = base
		out["attach_detach_during"] = a.PAtk()
		a.RemoveStatsByOwner(owner)
		out["attach_detach_after"] = a.PAtk()
	}

	return out
}

func TestGoldenSummonStatPipelineCapture(t *testing.T) {
	if os.Getenv("ACIS_CAPTURE_GOLDEN") == "" {
		t.Skip("set ACIS_CAPTURE_GOLDEN=1 to (re)capture the golden fixture from the current implementation")
	}
	got := goldenSummonScenarios(t)
	writeSummonGolden(t, "testdata/golden_stats.json", got)
}

func TestGoldenSummonStatPipelineParity(t *testing.T) {
	want := readSummonGolden(t, "testdata/golden_stats.json")
	got := goldenSummonScenarios(t)
	compareSummonGolden(t, want, got)
}

func writeSummonGolden(t testing.TB, path string, values map[string]float64) {
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

func readSummonGolden(t testing.TB, path string) map[string]float64 {
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

func compareSummonGolden(t testing.TB, want, got map[string]float64) {
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
