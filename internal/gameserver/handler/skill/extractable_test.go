package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ---- from extractable_test.go ----
type extractableFakeCaster struct {
	neutralCreature
	world.Presence
	fakeActor
	granted map[int32]int
	slots   map[int32]int // slots an item id needs per unit; 1 when absent
	free    int
	checked int // slots the last capacity check asked for
}

func (*extractableFakeCaster) Kind() actor.Kind { return actor.KindPlayer }

func (c *extractableFakeCaster) ItemSlotsNeeded(itemID int32, count int) int {
	if per, ok := c.slots[itemID]; ok {
		return per * count
	}
	return count
}

func (c *extractableFakeCaster) ItemSlotsFit(slots int) bool {
	c.checked = slots
	return slots <= c.free
}

func (c *extractableFakeCaster) AddCreatedItem(itemID int32, count int, nextID func() (int32, error)) bool {
	if _, err := nextID(); err != nil {
		return false
	}
	if c.granted == nil {
		c.granted = make(map[int32]int)
	}
	c.granted[itemID] += count
	return true
}

func extractableRegistry() *Registry {
	return NewRegistry(extractableHandler{ids: &fakeSignetIDs{}})
}

func TestExtractableGrantsTheOnlyGuaranteedProduct(t *testing.T) {
	caster := &extractableFakeCaster{free: 10}

	result, _ := extractableRegistry().UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "EXTRACTABLE", ExtractableItems: "57,10,100.0"},
		Targets: []Actor{},
	})

	if caster.granted[57] != 10 {
		t.Fatalf("granted = %v, want {57: 10}", caster.granted)
	}
	if len(result.Messages) != 0 {
		t.Fatalf("messages = %v, want none for a granted product", result.Messages)
	}
}

// TestExtractableChecksTheRowsSlotsTogether pins validateCapacityByItemIds:
// the slots every item of the rolled row needs are summed per item and
// quantity into one check.
func TestExtractableChecksTheRowsSlotsTogether(t *testing.T) {
	caster := &extractableFakeCaster{free: 10, slots: map[int32]int{57: 0}}

	extractableRegistry().Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "EXTRACTABLE", ExtractableItems: "57,10,30,3,100.0"},
		Targets: []Actor{},
	})

	if caster.checked != 3 {
		t.Fatalf("capacity check asked for %d slots, want 3 (0 for the held stack, 3 for three units)", caster.checked)
	}
	if caster.granted[57] != 10 || caster.granted[30] != 3 {
		t.Fatalf("granted = %v, want {57: 10, 30: 3}", caster.granted)
	}
}

func TestExtractableFullInventoryGrantsNothing(t *testing.T) {
	caster := &extractableFakeCaster{free: 1}

	result, _ := extractableRegistry().UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "EXTRACTABLE", ExtractableItems: "57,10,30,1,100.0"},
		Targets: []Actor{},
	})

	if len(caster.granted) != 0 {
		t.Fatalf("granted = %v, want none when the row needs more slots than are free", caster.granted)
	}
	if len(result.Messages) != 1 || result.Messages[0] != (SlotsFullMessage{}) {
		t.Fatalf("messages = %v, want one SlotsFullMessage", result.Messages)
	}
}

func TestExtractableRollMissingEveryRowReportsNothingInside(t *testing.T) {
	caster := &extractableFakeCaster{free: 10}

	result, _ := extractableRegistry().UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "EXTRACTABLE", ExtractableItems: "57,10,0"},
		Targets: []Actor{},
	})

	if len(caster.granted) != 0 {
		t.Fatalf("granted = %v, want none", caster.granted)
	}
	if len(result.Messages) != 1 || result.Messages[0] != (NothingInsideMessage{}) {
		t.Fatalf("messages = %v, want one NothingInsideMessage", result.Messages)
	}
}

func TestExtractableNoDataIsNoop(t *testing.T) {
	caster := &extractableFakeCaster{free: 10}

	result, _ := extractableRegistry().UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "EXTRACTABLE_FISH"},
		Targets: []Actor{},
	})
	if len(caster.granted) != 0 || len(result.Messages) != 0 {
		t.Fatalf("granted = %v, messages = %v; want nothing without extractable data", caster.granted, result.Messages)
	}
}
