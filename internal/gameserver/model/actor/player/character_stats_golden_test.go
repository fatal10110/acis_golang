package player

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// goldenPlayerScenarios computes the stat pipeline parity oracle for
// player.Character: actor x stat x active-modifier-set, including several
// same-order funcs attached/detached in different sequences (float addition
// is not associative, and AddFunc's insertion order is load-bearing — see
// issue #1527), a Set rebase, and an item-driven Enchant. Every case is
// deterministic given fresh actors, so re-running this against a rewritten
// stat pipeline must reproduce bit-identical float64 values.
func goldenPlayerScenarios(t testing.TB) map[string]float64 {
	t.Helper()
	out := make(map[string]float64)

	// Same-order (30: Add/Sub) funcs attached in two different sequences,
	// using precision-sensitive values so the stable-insertion-order
	// contract is actually observable in the result, not just documented.
	{
		tmpl := combatTemplate()
		c := liveCharacter(1, tmpl, combatItems())
		c.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 1e16}})
		c.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpSub, Value: 1e16}})
		c.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 1}})
		out["order30_forward"] = c.PAtk()

		tmpl2 := combatTemplate()
		c2 := liveCharacter(2, tmpl2, combatItems())
		c2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 1}})
		c2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpSub, Value: 1e16}})
		c2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 1e16}})
		out["order30_reverse"] = c2.PAtk()
	}

	// Set rebasing: a *Set at order 0 must replace `base` for every func
	// that runs after it, including the builtin at order 10.
	{
		tmpl := combatTemplate()
		c := liveCharacter(3, tmpl, combatItems())
		c.AddStatFuncs([]effect.Mod{
			{Stat: stat.MagicDefence, Op: effect.OpSet, Value: 500},
			{Stat: stat.MagicDefence, Op: effect.OpBaseMul, Value: 0.5},
		})
		out["set_rebase_mdef"] = c.MDef()
	}

	// Attach then detach: value must return exactly to the pre-attach
	// value, including through the builtin finalize step.
	{
		tmpl := combatTemplate()
		c := liveCharacter(4, tmpl, combatItems())
		base := c.PAtk()
		owner := effect.ModOwnerEffect(&effect.Effect{})
		c.AddStatFuncs([]effect.Mod{
			{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 7, Owner: owner},
			{Stat: stat.PowerAttack, Op: effect.OpMul, Value: 1.25, Owner: owner},
		})
		out["attach_detach_before"] = base
		out["attach_detach_during"] = c.PAtk()
		c.RemoveStatsByOwner(owner)
		out["attach_detach_after"] = c.PAtk()
	}

	// Mixed orders across several stats at once (BaseAdd, Mul, Add, AddMul,
	// SubDiv), each stat also carrying its own builtin at order 10.
	{
		tmpl := combatTemplate()
		tmpl.MAtk = 20
		tmpl.RunSpeed = 100
		c := liveCharacter(5, tmpl, combatItems())
		c.AddStatFuncs([]effect.Mod{
			{Stat: stat.PowerDefence, Op: effect.OpBaseAdd, Value: 4},
			{Stat: stat.PowerDefence, Op: effect.OpMul, Value: 1.1},
			{Stat: stat.MagicAttack, Op: effect.OpAdd, Value: 6},
			{Stat: stat.MagicAttack, Op: effect.OpAddMul, Value: 10}, // -10%
			{Stat: stat.RunSpeed, Op: effect.OpSubDiv, Value: 20},    // /(1-0.2)
		})
		out["mixed_pdef"] = c.PDef()
		out["mixed_matk"] = c.MAtk()
		out["mixed_runspeed"] = c.RunSpeed()
	}

	// Enchant: item-driven, no configured Value, math keyed off the
	// item's live EnchantLevel/Weapon/Crystal.
	{
		tmpl := combatTemplate()
		items := item.NewTable([]*item.Template{
			{ID: 50, Kind: item.KindWeapon, Slot: item.SlotRHand, Crystal: item.CrystalS,
				Weapon: &item.WeaponDetail{Type: item.WeaponSword}},
		})
		inst := &item.Instance{ObjectID: 900, TemplateID: 50, Location: item.LocationPaperdoll, LocationData: 0, EnchantLevel: 7}
		c := liveCharacter(6, tmpl, items, inst)
		tmplRef, _ := items.Get(50)
		owner := effect.ModOwnerItem(effect.ItemOwner{Inst: inst, Tmpl: tmplRef})
		c.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpEnchant, Owner: owner}})
		out["enchant_patk_s_over3"] = c.PAtk()

		inst2 := &item.Instance{ObjectID: 901, TemplateID: 50, Location: item.LocationPaperdoll, LocationData: 0, EnchantLevel: 2}
		c2 := liveCharacter(7, tmpl, items, inst2)
		tmplRef2, _ := items.Get(50)
		owner2 := effect.ModOwnerItem(effect.ItemOwner{Inst: inst2, Tmpl: tmplRef2})
		c2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpEnchant, Owner: owner2}})
		out["enchant_patk_s_under3"] = c2.PAtk()
	}

	return out
}

func TestGoldenPlayerStatPipelineCapture(t *testing.T) {
	if os.Getenv("ACIS_CAPTURE_GOLDEN") == "" {
		t.Skip("set ACIS_CAPTURE_GOLDEN=1 to (re)capture the golden fixture from the current implementation")
	}
	got := goldenPlayerScenarios(t)
	writeGolden(t, "testdata/golden_stats.json", got)
}

func TestGoldenPlayerStatPipelineParity(t *testing.T) {
	want := readGolden(t, "testdata/golden_stats.json")
	got := goldenPlayerScenarios(t)
	compareGolden(t, want, got)
}

func writeGolden(t testing.TB, path string, values map[string]float64) {
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

func readGolden(t testing.TB, path string) map[string]float64 {
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

func compareGolden(t testing.TB, want, got map[string]float64) {
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
