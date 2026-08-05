package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type spySummonSpawner struct {
	calls []spySpawnCall
	ok    bool
}

type spySpawnCall struct {
	owner *Character
	item  *item.Instance
}

func (s *spySummonSpawner) SpawnPet(owner *Character, controlItem *item.Instance) bool {
	s.calls = append(s.calls, spySpawnCall{owner: owner, item: controlItem})
	return s.ok
}

func TestCharacterSummonCreatureDelegatesToSpawner(t *testing.T) {
	c := &Character{}
	spy := &spySummonSpawner{ok: true}
	c.SetSummonSpawner(spy)
	inst := &item.Instance{ObjectID: 500, TemplateID: 91000}

	c.SummonCreature(modelskill.Definition{ID: 2046, Level: 1}, inst)

	if len(spy.calls) != 1 {
		t.Fatalf("SpawnPet calls = %d, want 1", len(spy.calls))
	}
	if spy.calls[0].owner != c {
		t.Fatalf("SpawnPet owner = %v, want %v", spy.calls[0].owner, c)
	}
	if spy.calls[0].item != inst {
		t.Fatalf("SpawnPet item = %v, want %v", spy.calls[0].item, inst)
	}
}

func TestCharacterSummonCreatureNoopsWithoutSpawner(t *testing.T) {
	c := &Character{}
	// No SetSummonSpawner call: must not panic, matching Java's item==nil
	// early return.
	c.SummonCreature(modelskill.Definition{ID: 2046, Level: 1}, &item.Instance{})
}

func TestCharacterSummonCreatureNoopsOnNonItemArg(t *testing.T) {
	c := &Character{}
	spy := &spySummonSpawner{ok: true}
	c.SetSummonSpawner(spy)

	// A cast-interrupted skill can reach the handler with no item
	// (handler/skill/summon.go's own doc comment); SummonCreature must
	// drop that silently, matching Java's checkedItem==nil early return.
	c.SummonCreature(modelskill.Definition{ID: 2046, Level: 1}, nil)

	if len(spy.calls) != 0 {
		t.Fatalf("SpawnPet calls = %d, want 0 for a non-item cast.Item", len(spy.calls))
	}
}
