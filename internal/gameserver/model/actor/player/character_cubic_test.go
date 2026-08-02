package player

import (
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
)

func TestCharacter_CubicListFull_DefaultCapIsOne(t *testing.T) {
	c := &Character{}
	if c.CubicListFull() {
		t.Fatal("CubicListFull() on an empty list = true, want false")
	}
	if _, added := c.AddOrRefreshCubic(cubic.Storm, false); !added {
		t.Fatal("AddOrRefreshCubic() first add reported added=false")
	}
	// With no Cubic Mastery (skill 143), size(1) > level(0): full.
	if !c.CubicListFull() {
		t.Fatal("CubicListFull() after one cubic with no mastery = false, want true")
	}
}

func TestCharacter_CubicListFull_MasteryRaisesCap(t *testing.T) {
	c := &Character{}
	c.SetSkillLevel(cubicMasterySkillID, 1)

	c.AddOrRefreshCubic(cubic.Storm, false)
	if c.CubicListFull() {
		t.Fatal("CubicListFull() after one cubic at mastery level 1 = true, want false")
	}
	c.AddOrRefreshCubic(cubic.Vampiric, false)
	if !c.CubicListFull() {
		t.Fatal("CubicListFull() after two cubics at mastery level 1 = false, want true")
	}
}

func TestCharacter_AddOrRefreshCubic_RefreshReportsNotAdded(t *testing.T) {
	c := &Character{}
	if _, added := c.AddOrRefreshCubic(cubic.Storm, false); !added {
		t.Fatal("first add reported added=false")
	}
	if touched, added := c.AddOrRefreshCubic(cubic.Storm, false); added || !touched {
		t.Fatal("re-adding the same cubic reported added=true or touched=false, want added=false, touched=true (refresh only)")
	}
}

func TestCharacter_CubicIDs_GrantOrder(t *testing.T) {
	c := &Character{}
	c.SetSkillLevel(cubicMasterySkillID, 5)
	c.AddOrRefreshCubic(cubic.Vampiric, false)
	c.AddOrRefreshCubic(cubic.Storm, false)

	want := []int{int(cubic.Vampiric), int(cubic.Storm)}
	if got := c.CubicIDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("CubicIDs() = %v, want %v", got, want)
	}
}
