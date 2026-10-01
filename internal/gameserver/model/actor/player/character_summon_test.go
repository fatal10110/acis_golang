package player

import (
	"reflect"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// ---- from character_cubic_test.go ----
// The cubic cap follows CubicList.isFull (size() > Cubic Mastery level,
// CubicList.java:110-113): addOrRefreshCubic (CubicList.java:45-59) polls
// and stops the oldest cubic before admitting past the cap.
func TestCharacter_AddOrRefreshCubic_NoMasteryEvictsOldest(t *testing.T) {
	c := &Character{}
	c.AddOrRefreshCubic(cubic.Storm, false)
	// With no Cubic Mastery (skill 143), size(1) > level(0): full.
	if _, added := c.AddOrRefreshCubic(cubic.Life, false); !added {
		t.Fatal("AddOrRefreshCubic(Life) on a full list reported added=false, want the oldest evicted")
	}
	if got := c.CubicIDs(); !slices.Equal(got, []int{int(cubic.Life)}) {
		t.Fatalf("CubicIDs() = %v, want [%d]", got, cubic.Life)
	}
}

func TestCharacter_AddOrRefreshCubic_MasteryRaisesCap(t *testing.T) {
	c := &Character{}
	c.SetSkillLevel(cubicMasterySkillID, 1)

	c.AddOrRefreshCubic(cubic.Storm, false)
	c.AddOrRefreshCubic(cubic.Vampiric, false)
	if got := c.CubicIDs(); !slices.Equal(got, []int{int(cubic.Storm), int(cubic.Vampiric)}) {
		t.Fatalf("CubicIDs() at mastery level 1 = %v, want both cubics", got)
	}
	c.AddOrRefreshCubic(cubic.Life, false)
	if got := c.CubicIDs(); !slices.Equal(got, []int{int(cubic.Vampiric), int(cubic.Life)}) {
		t.Fatalf("CubicIDs() past the mastery-1 cap = %v, want the oldest (Storm) evicted", got)
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

// ---- from character_summon_test.go ----
func TestCharacterSummonCreatureRequestsPetSpawn(t *testing.T) {
	c := &Character{}
	rec := recordEvents(c)
	inst := &item.Instance{ObjectID: 500, TemplateID: 91000}

	c.SummonCreature(modelskill.Definition{ID: 2046, Level: 1}, inst)

	got := event.Of[event.PetSummonRequested](rec)
	if len(got) != 1 {
		t.Fatalf("pet summon requests = %d, want 1", len(got))
	}
	if got[0].ControlItem != inst {
		t.Fatalf("pet summon item = %v, want %v", got[0].ControlItem, inst)
	}
}

func TestCharacterSummonServitorRequestsServitorSpawn(t *testing.T) {
	c := &Character{}
	rec := recordEvents(c)

	c.SummonServitor(modelskill.Definition{NpcID: 14848})

	got := event.Of[event.ServitorSummonRequested](rec)
	if len(got) != 1 || got[0].Skill.NpcID != 14848 {
		t.Fatalf("servitor summon requests = %+v, want one with NpcID 14848", got)
	}
}

func TestCharacterSummonCreatureNoopsWithoutSink(t *testing.T) {
	c := &Character{}
	// No sink attached: must not panic, matching Java's item==nil early
	// return.
	c.SummonCreature(modelskill.Definition{ID: 2046, Level: 1}, &item.Instance{})
}

func TestCharacterSummonCreatureNoopsOnNonItemArg(t *testing.T) {
	c := &Character{}
	rec := recordEvents(c)

	// A cast-interrupted skill can reach the handler with no item
	// (handler/skill/summon.go's own doc comment); SummonCreature must
	// drop that silently, matching Java's checkedItem==nil early return.
	c.SummonCreature(modelskill.Definition{ID: 2046, Level: 1}, nil)

	if got := event.Count[event.PetSummonRequested](rec); got != 0 {
		t.Fatalf("pet summon requests = %d, want 0 for a non-item cast.Item", got)
	}
}
