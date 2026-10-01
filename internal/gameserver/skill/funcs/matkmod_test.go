package funcs

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// TestMAtkModGroupsTheSquaredFactors pins FuncMAtkMod.calc's association,
// value * ((lvlMod * lvlMod) * (intMod * intMod)): at INT 52, level 31 and
// base 75 it finalizes just under 243, which the (int) M.Atk getter reads as
// 242; multiplying left to right lands on 243 exactly.
func TestMAtkModGroupsTheSquaredFactors(t *testing.T) {
	a := fakeActor{intv: 52, level: 31, levelMod: (100.0 - 11 + 31) / 100.0}
	got := MAtkMod(a, 0, 75)

	lm, im := a.levelMod, statbonus.INTBonus[52]
	if want := 75 * ((lm * lm) * (im * im)); got != want {
		t.Fatalf("MAtkMod() = %v, want %v", got, want)
	}
	if math.Trunc(got) != 242 {
		t.Fatalf("MAtkMod() = %v truncates to %v, want 242", got, math.Trunc(got))
	}
}
