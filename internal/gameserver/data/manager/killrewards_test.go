package manager

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

type sequentialIDs struct{ next int32 }

func (s *sequentialIDs) NextID() (int32, error) {
	s.next++
	return s.next, nil
}

type failingIDs struct{}

func (failingIDs) NextID() (int32, error) { return 0, errors.New("id space exhausted") }

type recordingGround struct {
	dropped []task.DropOptions
	items   []*grounditem.Item
}

func (r *recordingGround) Drop(ground *grounditem.Item, opts task.DropOptions) {
	r.items = append(r.items, ground)
	r.dropped = append(r.dropped, opts)
}

type nopKiller struct{ id int32 }

func (n nopKiller) ObjectID() int32 { return n.id }

func TestKillReward_DropsRolledItemsAtLocation(t *testing.T) {
	items := item.NewTable([]*item.Template{{ID: 57, Name: "adena"}})
	ground := &recordingGround{}
	ids := &sequentialIDs{}

	categories := []item.DropCategory{
		{Kind: item.DropCurrency, Chance: 100, Drops: []item.Drop{{ItemID: 57, Min: 10, Max: 10, Chance: 100}}},
	}
	rates := item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}

	r := NewKillReward(categories, nil, 1, false, rates, false, ids, items, ground, 100, 200, 300, 45)
	r.CalculateRewards(nopKiller{id: 1})

	if len(ground.items) != 1 {
		t.Fatalf("dropped %d items, want 1", len(ground.items))
	}
	got := ground.items[0]
	if got.ItemID() != 57 || got.Count() != 10 {
		t.Fatalf("dropped item = (%d, %d), want (57, 10)", got.ItemID(), got.Count())
	}
	opts := ground.dropped[0]
	if opts.X != 100 || opts.Y != 200 || opts.Z != 300 || opts.Heading != 45 {
		t.Fatalf("drop location = %+v, want (100, 200, 300, 45)", opts)
	}
}

func TestKillReward_SkipsSpoilWithoutPool(t *testing.T) {
	items := item.NewTable([]*item.Template{{ID: 6673, Name: "spoil-item"}})
	ground := &recordingGround{}
	ids := &sequentialIDs{}

	categories := []item.DropCategory{
		{Kind: item.DropSpoil, Chance: 100, Drops: []item.Drop{{ItemID: 6673, Min: 1, Max: 1, Chance: 100}}},
	}
	rates := item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}

	r := NewKillReward(categories, nil, 1, false, rates, false, ids, items, ground, 0, 0, 0, 0)
	r.CalculateRewards(nopKiller{id: 1})

	if len(ground.items) != 0 {
		t.Fatalf("dropped %d items with a nil spoil pool, want 0", len(ground.items))
	}
}

func TestKillReward_SkipsAutoLootHerbs(t *testing.T) {
	items := item.NewTable([]*item.Template{{ID: 8600, Name: "herb"}})
	ground := &recordingGround{}
	ids := &sequentialIDs{}

	categories := []item.DropCategory{
		{Kind: item.DropHerb, Chance: 100, Drops: []item.Drop{{ItemID: 8600, Min: 1, Max: 1, Chance: 100}}},
	}
	rates := item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}

	r := NewKillReward(categories, nil, 1, false, rates, true, ids, items, ground, 0, 0, 0, 0)
	r.CalculateRewards(nopKiller{id: 1})

	if len(ground.items) != 0 {
		t.Fatalf("dropped %d auto-loot herbs on the ground, want 0", len(ground.items))
	}
}

func TestKillReward_SkipsItemOnIDExhaustion(t *testing.T) {
	items := item.NewTable([]*item.Template{{ID: 57, Name: "adena"}})
	ground := &recordingGround{}

	categories := []item.DropCategory{
		{Kind: item.DropCurrency, Chance: 100, Drops: []item.Drop{{ItemID: 57, Min: 10, Max: 10, Chance: 100}}},
	}
	rates := item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}

	r := NewKillReward(categories, nil, 1, false, rates, false, failingIDs{}, items, ground, 0, 0, 0, 0)
	r.CalculateRewards(nopKiller{id: 1})

	if len(ground.items) != 0 {
		t.Fatalf("dropped %d items after id allocation failure, want 0", len(ground.items))
	}
}

var _ creature.Rewarder = (*KillReward)(nil)
