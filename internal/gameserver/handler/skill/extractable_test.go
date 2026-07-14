package skill

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type extractableFakeCaster struct {
	granted  map[int32]int
	capacity bool
}

func (c *extractableFakeCaster) AddItem(itemID int32, count int) {
	if c.granted == nil {
		c.granted = make(map[int32]int)
	}
	c.granted[itemID] += count
}

func (c *extractableFakeCaster) HasCapacityFor(itemIDs []int32) bool { return c.capacity }

func TestExtractableGrantsTheOnlyGuaranteedProduct(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &extractableFakeCaster{capacity: true}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "EXTRACTABLE", ExtractableItems: "57,10,100.0"},
		Targets: []any{},
	})

	if caster.granted[57] != 10 {
		t.Fatalf("granted = %v, want {57: 10}", caster.granted)
	}
}

func TestExtractableFullInventoryGrantsNothing(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &extractableFakeCaster{capacity: false}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "EXTRACTABLE", ExtractableItems: "57,10,100.0"},
		Targets: []any{},
	})

	if len(caster.granted) != 0 {
		t.Fatalf("granted = %v, want none when inventory is full", caster.granted)
	}
}

func TestExtractableNoDataIsNoop(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &extractableFakeCaster{capacity: true}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "EXTRACTABLE_FISH"},
		Targets: []any{},
	})
	if len(caster.granted) != 0 {
		t.Fatalf("granted = %v, want none without extractable data", caster.granted)
	}
}
