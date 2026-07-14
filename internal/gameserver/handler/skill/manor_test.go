package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type manorFakeSeedState struct {
	seeded, harvested bool
	allowed           bool
	sownBy            int32
	sownSeed          manor.Seed
	cropID            int32
	cropCount         int
}

func (s *manorFakeSeedState) Seeded() bool                         { return s.seeded }
func (s *manorFakeSeedState) Harvested() bool                      { return s.harvested }
func (s *manorFakeSeedState) MarkHarvested()                       { s.harvested = true }
func (s *manorFakeSeedState) AllowedToHarvest(playerID int32) bool { return s.allowed }
func (s *manorFakeSeedState) HarvestedCrop() (int32, int)          { return s.cropID, s.cropCount }
func (s *manorFakeSeedState) Sow(sowerID int32, seed manor.Seed) {
	s.seeded = true
	s.sownBy = sowerID
	s.sownSeed = seed
}

type manorFakeTarget struct {
	dead  bool
	level int
	state *manorFakeSeedState
}

func (m *manorFakeTarget) Dead() bool           { return m.dead }
func (m *manorFakeTarget) Level() int           { return m.level }
func (m *manorFakeTarget) SeedState() seedState { return m.state }

type manorFakeItem struct {
	seed manor.Seed
	ok   bool
}

func (i manorFakeItem) Seed() (manor.Seed, bool) { return i.seed, i.ok }

type manorFakeCaster struct {
	id    int32
	level int
	items map[int32]int
}

func (c manorFakeCaster) ObjectID() int32 { return c.id }
func (c manorFakeCaster) Level() int      { return c.level }
func (c *manorFakeCaster) AddEarnedItem(itemID int32, count int) {
	if c.items == nil {
		c.items = make(map[int32]int)
	}
	c.items[itemID] += count
}

func TestSowEventuallySucceedsAndMarksSeeded(t *testing.T) {
	// Seed/target/player levels all equal give a 90% sow success rate — not
	// a certainty, so the roll can't be forced deterministically. Retrying
	// drives the false-negative chance for this assertion to effectively
	// zero (0.1^300) without depending on a specific random outcome.
	registry := NewDefaultRegistry()
	caster := manorFakeCaster{id: 7, level: 40}
	item := manorFakeItem{seed: manor.Seed{Level: 40, Alternative: false}, ok: true}

	for i := 0; i < 300; i++ {
		target := &manorFakeTarget{level: 40, state: &manorFakeSeedState{}}
		if !registry.Use(Cast{
			Caster:  caster,
			Item:    item,
			Skill:   modelskill.Definition{SkillType: "SOW"},
			Targets: []any{target},
		}) {
			t.Fatal("Use() returned false for SOW")
		}
		if target.state.seeded {
			if target.state.sownBy != 7 {
				t.Fatalf("sown by = %d, want 7", target.state.sownBy)
			}
			return
		}
	}
	t.Fatal("SOW never succeeded in 300 attempts at a 90% success rate")
}

func TestSowAlreadySeededIsNoop(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := manorFakeCaster{id: 7, level: 40}
	target := &manorFakeTarget{level: 40, state: &manorFakeSeedState{seeded: true, sownBy: 3}}
	item := manorFakeItem{seed: manor.Seed{Level: 40}, ok: true}

	registry.Use(Cast{Caster: caster, Item: item, Skill: modelskill.Definition{SkillType: "SOW"}, Targets: []any{target}})
	if target.state.sownBy != 3 {
		t.Fatalf("already-seeded target should be untouched, sownBy = %d", target.state.sownBy)
	}
}

func TestHarvestRewardsAllowedHarvester(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &manorFakeCaster{id: 7, level: 40}
	target := &manorFakeTarget{level: 40, state: &manorFakeSeedState{seeded: true, allowed: true, cropID: 5001, cropCount: 12}}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "HARVEST"}, Targets: []any{target}})

	if !target.state.harvested {
		t.Error("target should be marked harvested")
	}
	if caster.items[5001] != 12 {
		t.Fatalf("caster earned items = %v, want {5001: 12}", caster.items)
	}
}

func TestHarvestDisallowedHarvesterGetsNothing(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &manorFakeCaster{id: 7, level: 40}
	target := &manorFakeTarget{level: 40, state: &manorFakeSeedState{seeded: true, allowed: false, cropID: 5001, cropCount: 12}}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "HARVEST"}, Targets: []any{target}})

	if target.state.harvested {
		t.Error("a disallowed harvester should not mark the target harvested")
	}
	if len(caster.items) != 0 {
		t.Fatalf("caster earned items = %v, want none", caster.items)
	}
}

func TestHarvestAlreadyHarvestedIsNoop(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &manorFakeCaster{id: 7, level: 40}
	target := &manorFakeTarget{level: 40, state: &manorFakeSeedState{seeded: true, harvested: true, allowed: true, cropID: 5001, cropCount: 12}}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "HARVEST"}, Targets: []any{target}})
	if len(caster.items) != 0 {
		t.Fatalf("caster earned items = %v, want none", caster.items)
	}
}
