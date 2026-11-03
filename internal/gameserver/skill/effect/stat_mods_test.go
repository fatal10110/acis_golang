package effect

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// groupedMods mixes every op order over two stats, with same-order mods
// whose float sums depend on their sequence.
func groupedMods() []Mod {
	a := ModOwnerSkill(modelskill.Ref{ID: 1, Level: 1})
	b := ModOwnerSkill(modelskill.Ref{ID: 2, Level: 1})
	return []Mod{
		{Stat: stat.MaxHP, Op: OpAdd, Value: 0.1, Owner: a},
		{Stat: stat.PowerDefence, Op: OpMul, Value: 1.3, Owner: a},
		{Stat: stat.MaxHP, Op: OpMul, Value: 1.1, Owner: a},
		{Stat: stat.MaxHP, Op: OpSet, Value: 777.7, Owner: b},
		{Stat: stat.MaxHP, Op: OpAdd, Value: 0.2, Owner: b},
		{Stat: stat.MaxHP, Op: OpBaseMul, Value: 0.3, Owner: b},
		{Stat: stat.PowerDefence, Op: OpAdd, Value: 1e16, Owner: b},
		{Stat: stat.MaxHP, Op: OpAddMul, Value: 0.05, Owner: a},
		{Stat: stat.PowerDefence, Op: OpAdd, Value: 1, Owner: a},
		{Stat: stat.MaxHP, Op: OpAdd, Value: 0.3, Owner: a},
	}
}

func addedOneByOne(fns []Mod, s stat.Stat) *Calculator {
	calc := NewCalculator(nil)
	for _, m := range fns {
		if m.Stat == s {
			calc.AddMod(m)
		}
	}
	return &calc
}

// TestGroupStatModsKeepsAddModOrder: each group holds its stat's mods in
// the exact sequence adding them one at a time leaves in a Calculator.
func TestGroupStatModsKeepsAddModOrder(t *testing.T) {
	t.Parallel()
	fns := groupedMods()
	g := GroupStatMods(fns)
	for _, s := range []stat.Stat{stat.MaxHP, stat.PowerDefence, stat.MagicDefence} {
		want := addedOneByOne(fns, s).mods
		got := g.mods(s)
		if len(got) != len(want) {
			t.Fatalf("%v: %d mods, want %d", s, len(got), len(want))
		}
		for i := range want {
			if got[i].Op != want[i].Op || got[i].Value != want[i].Value || got[i].Owner != want[i].Owner {
				t.Errorf("%v[%d] = %+v, want %+v", s, i, got[i], want[i])
			}
		}
		if got, want := g.Calc(s, nil, nil, 1000, nil, nil), addedOneByOne(fns, s).Calc(nil, 1000, nil, nil); got != want {
			t.Errorf("%v: Calc = %v, want %v", s, got, want)
		}
	}
	if GroupStatMods(nil) != nil {
		t.Error("GroupStatMods(nil) != nil")
	}
	var none *StatMods
	if got := none.Calc(stat.MaxHP, nil, nil, 5, nil, nil); got != 5 {
		t.Errorf("nil StatMods Calc = %v, want the base", got)
	}
}

// TestStatModsCalculatorChangesLeaveGroupIntact: a Calculator started from
// a group adds and drops mods on its own chain only.
func TestStatModsCalculatorChangesLeaveGroupIntact(t *testing.T) {
	t.Parallel()
	g := GroupStatMods(groupedMods())
	want := g.Calc(stat.MaxHP, nil, nil, 1000, nil, nil)
	calc := g.NewCalculator(stat.MaxHP, nil)
	if calc.Calc(nil, 1000, nil, nil) != want {
		t.Fatal("a Calculator started from the group computes something else")
	}

	calc.AddMod(Mod{Stat: stat.MaxHP, Op: OpAdd, Value: 50})
	if removed := calc.RemoveOwner(ModOwnerSkill(modelskill.Ref{ID: 2, Level: 1})); removed != 3 {
		t.Fatalf("RemoveOwner removed %d mods, want 3", removed)
	}
	if got := g.Calc(stat.MaxHP, nil, nil, 1000, nil, nil); got != want {
		t.Errorf("group Calc = %v after the Calculator changed, want %v", got, want)
	}
	if got := len(g.mods(stat.MaxHP)); got != 7 {
		t.Errorf("group holds %d MaxHP mods, want 7", got)
	}
	var stats []stat.Stat
	for s := range g.StatsOwnedBy(ModOwnerSkill(modelskill.Ref{ID: 2, Level: 1})) {
		stats = append(stats, s)
	}
	if len(stats) != 2 || stats[0] != stat.MaxHP || stats[1] != stat.PowerDefence {
		t.Errorf("StatsOwnedBy = %v, want [MaxHP PowerDefence]", stats)
	}
}
