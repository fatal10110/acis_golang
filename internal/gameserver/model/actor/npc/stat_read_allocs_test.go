package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// A stat read on an NPC no effect touched allocates nothing: the folk's and
// the hostile's stat views pass as an interface without a boxed copy, and
// the builtin-only chain runs without a calculator of its own.
func TestNoCalculatorStatReadAllocatesNothing(t *testing.T) {
	inst, err := NewInstance(1, &Template{ID: 30001, TemplateID: 30001, Type: "Merchant", Level: 70, HPMax: 2444, PAtk: 100})
	if err != nil {
		t.Fatal(err)
	}
	folk, err := NewFolk(inst)
	if err != nil {
		t.Fatal(err)
	}
	hostile, err := NewHostile(&Instance{
		ObjectID: 101,
		Template: &Template{ID: 9001, Type: "Monster", Level: 20, PAtk: 100},
		Kind:     "Monster",
	}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		calc func(stat.Stat, float64) float64
	}{
		{"folk", folk.CalcStat},
		{"hostile", hostile.CalcStat},
	} {
		want := tc.calc(stat.PowerAttack, 100)
		if want == 100 {
			t.Fatalf("%s P.Atk = %v, want the builtin to change the base 100", tc.name, want)
		}
		var got float64
		allocs := testing.AllocsPerRun(100, func() { got = tc.calc(stat.PowerAttack, 100) })
		if allocs != 0 {
			t.Errorf("%s no-calculator stat read allocates %v times, want 0", tc.name, allocs)
		}
		if got != want {
			t.Errorf("%s P.Atk = %v on repeat, want %v", tc.name, got, want)
		}
	}
}
