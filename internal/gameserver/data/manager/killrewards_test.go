package manager

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
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

type lootKiller struct {
	id    int32
	items map[int32]int
	herbs []int32
	// refuseHerbs models a detached character: it still satisfies the
	// receiver contract but has no consumer behind it.
	refuseHerbs bool
}

func (l *lootKiller) ConsumeHerb(itemID int32) bool {
	if l.refuseHerbs {
		return false
	}
	l.herbs = append(l.herbs, itemID)
	return true
}

func (l *lootKiller) ObjectID() int32 { return l.id }

func (l *lootKiller) AddRewardItem(itemID int32, count int, objectID int32) bool {
	if objectID == 0 {
		return false
	}
	if l.items == nil {
		l.items = make(map[int32]int)
	}
	l.items[itemID] += count
	return true
}

func TestKillReward_DropsRolledItemsAtLocation(t *testing.T) {
	items := item.NewTable([]*item.Template{{ID: 57, Name: "adena"}})
	ground := &recordingGround{}
	ids := &sequentialIDs{}

	categories := []item.DropCategory{
		{Kind: item.DropCurrency, Chance: 100, Drops: []item.Drop{{ItemID: 57, Min: 10, Max: 10, Chance: 100}}},
	}
	rates := item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}

	r := NewKillReward(categories, nil, 1, false, rates, false, false, ids, items, ground, 100, 200, 300, 45, 999)
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
	if opts.DropperID != 999 {
		t.Fatalf("DropperID = %d, want 999 (the dying NPC's object id)", opts.DropperID)
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

	r := NewKillReward(categories, nil, 1, false, rates, false, false, ids, items, ground, 0, 0, 0, 0, 0)
	r.CalculateRewards(nopKiller{id: 1})

	if len(ground.items) != 0 {
		t.Fatalf("dropped %d items with a nil spoil pool, want 0", len(ground.items))
	}
}

// herbTable is a drop table holding one real herb template: the etc type is
// what decides herb handling, not the drop category it was rolled from.
func herbTable() *item.Table {
	return item.NewTable([]*item.Template{{
		ID: 8600, Name: "herb", Kind: item.KindEtcItem,
		EtcItem: &item.EtcItemDetail{Type: item.EtcItemHerb, Handler: "ItemSkills"},
	}})
}

func herbCategories() []item.DropCategory {
	return []item.DropCategory{
		{Kind: item.DropHerb, Chance: 100, Drops: []item.Drop{{ItemID: 8600, Min: 1, Max: 1, Chance: 100}}},
	}
}

func TestKillReward_ConsumesAutoLootHerbInsteadOfStoringIt(t *testing.T) {
	ground := &recordingGround{}
	rates := item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}

	killer := &lootKiller{id: 1}
	r := NewKillReward(herbCategories(), nil, 1, false, rates, false, true, &sequentialIDs{}, herbTable(), ground, 0, 0, 0, 0, 0)
	r.CalculateRewards(killer)

	if len(killer.herbs) != 1 || killer.herbs[0] != 8600 {
		t.Fatalf("consumed herbs = %v, want [8600]", killer.herbs)
	}
	if len(killer.items) != 0 {
		t.Fatalf("inventory items = %v, want none: a herb never occupies a slot", killer.items)
	}
	if len(ground.items) != 0 {
		t.Fatalf("dropped %d auto-loot herbs on the ground, want 0", len(ground.items))
	}
}

// TestKillReward_DropsAutoLootHerbWhenKillerCannotConsume covers the killer
// that cannot consume a herb at all — a pet or servitor kill (#1057). The
// herb has to stay obtainable rather than vanish between the roll and the
// delivery.
func TestKillReward_DropsAutoLootHerbWhenKillerCannotConsume(t *testing.T) {
	ground := &recordingGround{}
	rates := item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}

	r := NewKillReward(herbCategories(), nil, 1, false, rates, false, true, &sequentialIDs{}, herbTable(), ground, 0, 0, 0, 0, 0)
	r.CalculateRewards(nopKiller{id: 1})

	if len(ground.items) != 1 || ground.items[0].ItemID() != 8600 {
		t.Fatalf("ground items = %+v, want the unconsumable herb dropped", ground.items)
	}
}

// TestKillReward_DropsAutoLootHerbWhenTheConsumerIsInactive covers a killer
// that satisfies the receiver contract but consumes nothing — a character
// whose herb consumer was unwired on detach, e.g. by logging out between the
// killing blow and reward resolution. The herb goes to the ground: storing it
// would put back the blank inventory square this whole change removes.
func TestKillReward_DropsAutoLootHerbWhenTheConsumerIsInactive(t *testing.T) {
	ground := &recordingGround{}
	rates := item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}

	killer := &lootKiller{id: 1, refuseHerbs: true}
	r := NewKillReward(herbCategories(), nil, 1, false, rates, false, true, &sequentialIDs{}, herbTable(), ground, 0, 0, 0, 0, 0)
	r.CalculateRewards(killer)

	if len(killer.herbs) != 0 {
		t.Fatalf("consumed herbs = %v, want none", killer.herbs)
	}
	if len(killer.items) != 0 {
		t.Fatalf("inventory items = %v, want none: a herb never occupies a slot", killer.items)
	}
	if len(ground.items) != 1 || ground.items[0].ItemID() != 8600 {
		t.Fatalf("ground items = %+v, want the refused herb dropped", ground.items)
	}
}

// TestKillReward_StoresNonHerbTemplateFromHerbCategory pins the discriminator:
// a category tagged HERB holding an ordinary item still delivers that item the
// ordinary way instead of being consumed and discarded.
func TestKillReward_StoresNonHerbTemplateFromHerbCategory(t *testing.T) {
	items := item.NewTable([]*item.Template{{
		ID: 8600, Name: "not-a-herb", Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{},
	}})
	ground := &recordingGround{}
	rates := item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}

	killer := &lootKiller{id: 1}
	r := NewKillReward(herbCategories(), nil, 1, false, rates, false, true, &sequentialIDs{}, items, ground, 0, 0, 0, 0, 0)
	r.CalculateRewards(killer)

	if len(killer.herbs) != 0 {
		t.Fatalf("consumed herbs = %v, want none for a non-herb template", killer.herbs)
	}
	if got := killer.items[8600]; got != 1 {
		t.Fatalf("inventory count = %d, want 1", got)
	}
	if len(ground.items) != 0 {
		t.Fatalf("dropped %d items on the ground, want 0", len(ground.items))
	}
}

func TestKillReward_AutoLootsRolledItemsIntoKillerInventory(t *testing.T) {
	items := item.NewTable([]*item.Template{{ID: 57, Name: "adena", Stackable: true}})
	ground := &recordingGround{}
	ids := &sequentialIDs{}

	categories := []item.DropCategory{
		{Kind: item.DropCurrency, Chance: 100, Drops: []item.Drop{{ItemID: 57, Min: 10, Max: 10, Chance: 100}}},
	}
	rates := item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}

	killer := &lootKiller{id: 1}
	r := NewKillReward(categories, nil, 1, false, rates, true, false, ids, items, ground, 0, 0, 0, 0, 0)
	r.CalculateRewards(killer)

	if len(ground.items) != 0 {
		t.Fatalf("dropped %d auto-looted items on the ground, want 0", len(ground.items))
	}
	if got := killer.items[57]; got != 10 {
		t.Fatalf("auto-looted item 57 = %d, want 10", got)
	}
}

func TestKillReward_SkipsItemOnIDExhaustion(t *testing.T) {
	items := item.NewTable([]*item.Template{{ID: 57, Name: "adena"}})
	ground := &recordingGround{}

	categories := []item.DropCategory{
		{Kind: item.DropCurrency, Chance: 100, Drops: []item.Drop{{ItemID: 57, Min: 10, Max: 10, Chance: 100}}},
	}
	rates := item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}

	r := NewKillReward(categories, nil, 1, false, rates, false, false, failingIDs{}, items, ground, 0, 0, 0, 0, 0)
	r.CalculateRewards(nopKiller{id: 1})

	if len(ground.items) != 0 {
		t.Fatalf("dropped %d items after id allocation failure, want 0", len(ground.items))
	}
}

func TestPetRewardSharesAndRoundsBeforeRateScaling(t *testing.T) {
	tests := []struct {
		name                   string
		expType                int
		petDamage, totalDamage float64
		exp                    int64
		sp                     int
		wantExp, wantSP        int
	}{
		{"damage share", -1, 1, 3, 100, 10, 33, 3},
		{"configured share", 25, 0, 1, 100, 10, 75, 8},
		{"ratio capped", 150, 0, 1, 100, 10, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp, sp := petReward(tt.expType, tt.petDamage, tt.totalDamage, tt.exp, tt.sp)
			if exp != int64(tt.wantExp) || sp != tt.wantSP {
				t.Fatalf("petReward() = (%d, %d), want (%d, %d)", exp, sp, tt.wantExp, tt.wantSP)
			}
		})
	}
}

func TestRewardLeaderUsesCombinedOwnerAndPetDamage(t *testing.T) {
	a := &player.Character{CharLevel: 10}
	b := &player.Character{CharLevel: 20}
	entries := []playerRewardEntry{
		{actor: a, damage: 100},
		{actor: b, damage: 120},
	}

	dealer, level := rewardLeader(entries)
	if dealer != b {
		t.Fatalf("max dealer = %p, want player with combined 120 damage", dealer)
	}
	if level != b.CharLevel {
		t.Fatalf("highest level = %d, want %d", level, b.CharLevel)
	}
}

var _ creature.Rewarder = (*KillReward)(nil)
