package skill

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ---- from spoil_test.go ----
type spoilFakeTarget struct {
	world.Presence
	neutralNPC
	fakeActor
	dead  bool
	level int
	pool  *item.SpoilPool
}

func (*spoilFakeTarget) Kind() actor.Kind             { return actor.KindNPC }
func (s *spoilFakeTarget) Dead() bool                 { return s.dead }
func (s *spoilFakeTarget) Level() int                 { return s.level }
func (s *spoilFakeTarget) SpoilPool() *item.SpoilPool { return s.pool }

type spoilFakeCaster struct {
	neutralPlayer
	world.Presence
	fakeActor
	id             int32
	level          int
	items          map[int32]int
	earned         []int32
	alreadyNotices int
	notices        []string
	resisted       []Resisted
}

func (c *spoilFakeCaster) ObjectID() int32 { return c.id }
func (*spoilFakeCaster) Kind() actor.Kind  { return actor.KindPlayer }
func (c *spoilFakeCaster) Level() int      { return c.level }
func (c *spoilFakeCaster) AddEarnedItem(itemID int32, count int, nextID func() (int32, error)) bool {
	if _, err := nextID(); err != nil {
		return false
	}
	if c.items == nil {
		c.items = make(map[int32]int)
	}
	c.items[itemID] += count
	c.earned = append(c.earned, itemID)
	return true
}

func sweepRegistry() *Registry {
	return NewRegistry(sweepHandler{ids: &fakeSignetIDs{}})
}

func (c *spoilFakeCaster) NotifySpoilAlready() {
	c.alreadyNotices++
	c.notices = append(c.notices, "already")
}
func (c *spoilFakeCaster) NotifySpoilSuccess() { c.notices = append(c.notices, "success") }
func (c *spoilFakeCaster) NotifyResistedSkill(name string, id modelskill.ID, level int) {
	c.notices = append(c.notices, "resisted")
	c.resisted = append(c.resisted, Resisted{TargetName: name, SkillID: id, SkillLevel: level})
}

func TestSpoilPreservesPerTargetNoticeOrder(t *testing.T) {
	rolls := []int{0, 9999} // failed roll, then successful roll
	registry := NewRegistry(spoilHandler{roll: func(int) int {
		roll := rolls[0]
		rolls = rolls[1:]
		return roll
	}})
	caster := &spoilFakeCaster{id: 42, level: 1}
	failed := &spoilFakeTarget{level: 100, pool: &item.SpoilPool{}}
	succeeded := &spoilFakeTarget{level: 1, pool: &item.SpoilPool{}}
	result, ok := registry.UseResult(Cast{
		Caster: caster, Skill: modelskill.Definition{ID: 302, Level: 7, SkillType: "SPOIL", MagicLevel: 1},
		Targets: []Actor{failed, succeeded},
	})
	if !ok || failed.pool.IsSpoiled() || !succeeded.pool.IsSpoiled() {
		t.Fatalf("UseResult() handled = %t, spoiled = %t/%t; want false/true", ok, failed.pool.IsSpoiled(), succeeded.pool.IsSpoiled())
	}
	// The network delivers Result messages only after the handler returns.
	for _, message := range result.Messages {
		if _, ok := message.(Resisted); !ok {
			t.Fatalf("unexpected deferred message %T", message)
		}
		caster.notices = append(caster.notices, "resisted")
	}
	if !slices.Equal(caster.notices, []string{"resisted", "success"}) {
		t.Fatalf("caster notices = %v, want resisted then success", caster.notices)
	}
}

func TestSpoilEventuallyMarksTarget(t *testing.T) {
	// Level-equal caster/target still carries a real magic-resist chance
	// (never exactly 100%), so retry instead of asserting a single roll.
	registry := NewDefaultRegistry()
	caster := &spoilFakeCaster{id: 42, level: 40}

	for i := 0; i < 300; i++ {
		target := &spoilFakeTarget{level: 40, pool: &item.SpoilPool{}}
		registry.Use(Cast{
			Caster:  caster,
			Skill:   modelskill.Definition{SkillType: "SPOIL", MagicLevel: 40},
			Targets: []Actor{target},
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

func TestSpoilReportsFailedMagicRollAtLevelOne(t *testing.T) {
	registry := NewRegistry(spoilHandler{roll: func(int) int { return 0 }})
	caster := &spoilFakeCaster{id: 42, level: 1}
	def := modelskill.Definition{ID: 254, Level: 7, SkillType: "SPOIL", MagicLevel: 1}
	target := &spoilFakeTarget{level: 100, pool: &item.SpoilPool{}}
	result, ok := registry.UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
	if !ok || target.pool.IsSpoiled() {
		t.Fatalf("UseResult() handled = %t, spoiled = %t; want failed SPOIL", ok, target.pool.IsSpoiled())
	}
	if len(caster.resisted) != 1 || caster.resisted[0] != (Resisted{SkillID: 254, SkillLevel: 1}) || len(result.Resisted) != 0 {
		t.Fatalf("direct resists = %+v, deferred resists = %+v; want one direct level-1 report", caster.resisted, result.Resisted)
	}
}

func TestSpoilAlreadySpoiledIsSkipped(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &spoilFakeCaster{id: 42, level: 40}
	target := &spoilFakeTarget{level: 40, pool: &item.SpoilPool{}}
	target.pool.Mark(99)

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "SPOIL", MagicLevel: 40}, Targets: []Actor{target}})
	if !target.pool.IsSpoiler(99) {
		t.Fatal("an already-spoiled pool should keep its original spoiler")
	}
	if caster.alreadyNotices != 1 {
		t.Fatalf("already-spoiled notices = %d, want 1", caster.alreadyNotices)
	}
}

func TestSweepEarnsPooledItemsAndClearsPool(t *testing.T) {
	caster := &spoilFakeCaster{id: 1}
	target := &spoilFakeTarget{pool: &item.SpoilPool{}}
	target.pool.Mark(1)
	target.pool.Add(57, 10)

	sweepRegistry().Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "SWEEP"}, Targets: []Actor{target}})

	if caster.items[57] != 10 {
		t.Fatalf("caster earned items = %v, want {57: 10}", caster.items)
	}
	if target.pool.IsSpoiled() || target.pool.Sweepable() {
		t.Fatal("sweeping should fully clear the pool, spoiler marker included")
	}
}

// TestSweepEarnsInPoolOrder: the swept items are earned in the order the
// pool hands them over.
func TestSweepEarnsInPoolOrder(t *testing.T) {
	caster := &spoilFakeCaster{id: 1}
	target := &spoilFakeTarget{pool: &item.SpoilPool{}}
	target.pool.Mark(1)
	for _, id := range []int32{1869, 1876, 1864, 116, 1872} {
		target.pool.Add(id, 1)
	}

	sweepRegistry().Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "SWEEP"}, Targets: []Actor{target}})

	if want := []int32{1872, 1876, 116, 1864, 1869}; !slices.Equal(caster.earned, want) {
		t.Fatalf("earned order = %v, want %v", caster.earned, want)
	}
}

func TestSweepEmptyPoolIsNoop(t *testing.T) {
	caster := &spoilFakeCaster{id: 1}
	target := &spoilFakeTarget{pool: &item.SpoilPool{}}

	sweepRegistry().Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "SWEEP"}, Targets: []Actor{target}})
	if len(caster.items) != 0 {
		t.Fatalf("nothing to sweep should reward nothing, got %v", caster.items)
	}
}

// TestSweepByNonPlayerLeavesPool: only a player sweeps; any other caster
// leaves the pool, spoiler marker included, for the spoiler.
func TestSweepByNonPlayerLeavesPool(t *testing.T) {
	caster := &manorFakeCaster{id: 1}
	target := &spoilFakeTarget{pool: &item.SpoilPool{}}
	target.pool.Mark(1)
	target.pool.Add(57, 10)

	sweepRegistry().Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "SWEEP"}, Targets: []Actor{target}})
	if !target.pool.IsSpoiler(1) || !target.pool.Sweepable() {
		t.Fatal("a non-player sweep drained the pool")
	}
	if len(caster.items) != 0 {
		t.Fatalf("non-player caster earned %v, want nothing", caster.items)
	}
}
