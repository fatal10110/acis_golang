package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type spoilFakeTarget struct {
	dead  bool
	level int
	pool  *item.SpoilPool
}

func (s *spoilFakeTarget) Dead() bool                 { return s.dead }
func (s *spoilFakeTarget) Level() int                 { return s.level }
func (s *spoilFakeTarget) SpoilPool() *item.SpoilPool { return s.pool }

type spoilFakeCaster struct {
	id       int32
	level    int
	inParty  bool
	items    map[int32]int
	distItem int32
	distCnt  int32
}

func (c spoilFakeCaster) ObjectID() int32 { return c.id }
func (c spoilFakeCaster) Level() int      { return c.level }
func (c *spoilFakeCaster) AddEarnedItem(itemID int32, count int) {
	if c.items == nil {
		c.items = make(map[int32]int)
	}
	c.items[itemID] += count
}
func (c *spoilFakeCaster) InParty() bool { return c.inParty }
func (c *spoilFakeCaster) DistributeItem(itemID, count int32) {
	c.distItem, c.distCnt = itemID, count
}

func TestSpoilEventuallyMarksTarget(t *testing.T) {
	// Level-equal caster/target still carries a real magic-resist chance
	// (never exactly 100%), so retry instead of asserting a single roll.
	registry := NewDefaultRegistry()
	caster := spoilFakeCaster{id: 42, level: 40}

	for i := 0; i < 300; i++ {
		target := &spoilFakeTarget{level: 40, pool: &item.SpoilPool{}}
		registry.Use(Cast{
			Caster:  caster,
			Skill:   modelskill.Definition{SkillType: "SPOIL", MagicLevel: 40},
			Targets: []any{target},
		})
		if target.pool.IsSpoiled() {
			if !target.pool.IsSpoiler(42) {
				t.Fatal("spoiled pool should be marked by the caster")
			}
			return
		}
	}
	t.Fatal("SPOIL never succeeded in 300 attempts")
}

func TestSpoilAlreadySpoiledIsSkipped(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := spoilFakeCaster{id: 42, level: 40}
	target := &spoilFakeTarget{level: 40, pool: &item.SpoilPool{}}
	target.pool.Mark(99)

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "SPOIL", MagicLevel: 40}, Targets: []any{target}})
	if !target.pool.IsSpoiler(99) {
		t.Fatal("an already-spoiled pool should keep its original spoiler")
	}
}

func TestSweepDistributesPooledItemsAndClearsPool(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &spoilFakeCaster{id: 1}
	target := &spoilFakeTarget{pool: &item.SpoilPool{}}
	target.pool.Mark(1)
	target.pool.Add(57, 10)

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "SWEEP"}, Targets: []any{target}})

	if caster.items[57] != 10 {
		t.Fatalf("caster earned items = %v, want {57: 10}", caster.items)
	}
	if target.pool.IsSpoiled() || target.pool.Sweepable() {
		t.Fatal("sweeping should fully clear the pool, spoiler marker included")
	}
}

func TestSweepDistributesThroughParty(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &spoilFakeCaster{id: 1, inParty: true}
	target := &spoilFakeTarget{pool: &item.SpoilPool{}}
	target.pool.Mark(1)
	target.pool.Add(57, 10)

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "SWEEP"}, Targets: []any{target}})

	if caster.distItem != 57 || caster.distCnt != 10 {
		t.Fatalf("party distribution = (%d, %d), want (57, 10)", caster.distItem, caster.distCnt)
	}
	if len(caster.items) != 0 {
		t.Fatalf("caster should not also receive a direct reward: %v", caster.items)
	}
}

func TestSweepEmptyPoolIsNoop(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &spoilFakeCaster{id: 1}
	target := &spoilFakeTarget{pool: &item.SpoilPool{}}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "SWEEP"}, Targets: []any{target}})
	if len(caster.items) != 0 {
		t.Fatalf("nothing to sweep should reward nothing, got %v", caster.items)
	}
}
