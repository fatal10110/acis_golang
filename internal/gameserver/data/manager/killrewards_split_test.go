package manager

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Item ids of the split fixtures: scroll is non-stackable like Scroll:
// Enchant Armor (Grade D), thread stackable like Thread.
const (
	splitScrollID int32 = 956
	splitThreadID int32 = 1868
)

func splitItems() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: splitScrollID, Name: "Scroll: Enchant Armor (Grade D)"},
		{ID: splitThreadID, Name: "Thread", Stackable: true},
	})
}

func guaranteed(kind item.DropKind, itemID, count int32) item.DropCategory {
	return item.DropCategory{Kind: kind, Chance: 100, Drops: []item.Drop{{ItemID: itemID, Min: count, Max: count, Chance: 100}}}
}

var splitRates = item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}

// groundDrop is one placed ground item: object id, item and count.
type groundDrop struct {
	objectID, itemID int32
	count            int
}

func placed(g *recordingGround) []groundDrop {
	out := make([]groundDrop, len(g.items))
	for i, it := range g.items {
		out[i] = groundDrop{it.Instance.ObjectID, it.Instance.TemplateID, it.Instance.Count}
	}
	return out
}

// TestKillRewardSplitsNonStackableUnderMultipleItemDrop pins Npc.dropItem:
// a non-stackable rolled N times falls as N count-1 ground items with their
// own object ids and their own scatter around the corpse; with
// MultipleItemDrop off it falls as one count-1 item.
func TestKillRewardSplitsNonStackableUnderMultipleItemDrop(t *testing.T) {
	categories := []item.DropCategory{guaranteed(item.DropNormal, splitScrollID, 3)}

	ground := &recordingGround{}
	var targets []location.Location
	geo := fakeGeo{validAt: func(_, _, _, tx, ty, tz int) location.Location {
		targets = append(targets, location.Location{X: tx, Y: ty, Z: tz})
		return location.Location{X: tx, Y: ty, Z: tz}
	}}
	NewKillReward(categories, nil, 1, true, splitRates, false, false, &sequentialIDs{}, splitItems(), ground, geo, 1000, 2000, 300, 0, 9).
		MultipleItemDrop(true).CalculateRewards(nopKiller{id: 1})

	want := []groundDrop{{1, splitScrollID, 1}, {2, splitScrollID, 1}, {3, splitScrollID, 1}}
	if got := placed(ground); !slices.Equal(got, want) {
		t.Fatalf("ground items = %v, want %v", got, want)
	}
	if len(targets) != 3 {
		t.Fatalf("scatter points = %d, want one per ground item", len(targets))
	}
	for i, opts := range ground.dropped {
		if opts.X != targets[i].X || opts.Y != targets[i].Y {
			t.Fatalf("item %d placed at (%d,%d), want its own scatter point %v", i, opts.X, opts.Y, targets[i])
		}
		if dx, dy := opts.X-1000, opts.Y-2000; dx < -dropScatterOffset || dx > dropScatterOffset || dy < -dropScatterOffset || dy > dropScatterOffset {
			t.Fatalf("item %d at (%d,%d), want within +/-%d of the corpse", i, opts.X, opts.Y, dropScatterOffset)
		}
		if opts.ProtectOwnerID != 0 || opts.ProtectFor != raidLootProtection {
			t.Fatalf("item %d options = %+v, want the raid protection window", i, opts)
		}
	}

	single := &recordingGround{}
	NewKillReward(categories, nil, 1, false, splitRates, false, false, &sequentialIDs{}, splitItems(), single, nil, 0, 0, 0, 0, 9).
		MultipleItemDrop(false).CalculateRewards(nopKiller{id: 1})
	if got := placed(single); !slices.Equal(got, []groundDrop{{1, splitScrollID, 1}}) {
		t.Fatalf("MultipleItemDrop off: ground items = %v, want one count-1 scroll", got)
	}
}

// TestKillRewardKeepsStackablesWhole: a stackable roll stays one stack of
// its count whatever MultipleItemDrop says.
func TestKillRewardKeepsStackablesWhole(t *testing.T) {
	ground := &recordingGround{}
	NewKillReward([]item.DropCategory{guaranteed(item.DropNormal, splitThreadID, 7)}, nil, 1, false, splitRates, false, false, &sequentialIDs{}, splitItems(), ground, nil, 0, 0, 0, 0, 9).
		MultipleItemDrop(true).CalculateRewards(nopKiller{id: 1})
	if got := placed(ground); !slices.Equal(got, []groundDrop{{1, splitThreadID, 7}}) {
		t.Fatalf("ground items = %v, want one stack of 7 thread", got)
	}
}

// TestKillRewardDropsEachCategoryApart pins Monster.doItemDrop's loop over
// categories: the same item rolled by two categories falls as two ground
// items, in category order.
func TestKillRewardDropsEachCategoryApart(t *testing.T) {
	ground := &recordingGround{}
	categories := []item.DropCategory{
		guaranteed(item.DropNormal, splitThreadID, 2),
		guaranteed(item.DropNormal, splitScrollID, 1),
		guaranteed(item.DropNormal, splitThreadID, 5),
	}
	NewKillReward(categories, nil, 1, false, splitRates, false, false, &sequentialIDs{}, splitItems(), ground, nil, 0, 0, 0, 0, 9).
		MultipleItemDrop(true).CalculateRewards(nopKiller{id: 1})
	want := []groundDrop{{1, splitThreadID, 2}, {2, splitScrollID, 1}, {3, splitThreadID, 5}}
	if got := placed(ground); !slices.Equal(got, want) {
		t.Fatalf("ground items = %v, want %v", got, want)
	}
}

// recordingAnnouncer records each raid drop announcement, and the number
// of ground items placed when it came.
type recordingAnnouncer struct {
	ground *recordingGround
	calls  [][3]int
}

func (a *recordingAnnouncer) AnnounceRaidDrop(itemID int32, count int) {
	a.calls = append(a.calls, [3]int{int(itemID), count, len(a.ground.items)})
}

// TestKillRewardAnnouncesRaidDrops pins the end of
// Monster.dropOrAutoLootItem: a raid kill names every dropped or
// auto-looted item, once per category roll with its whole count, after the
// item is placed or looted; herbs and non-raid kills are never named.
func TestKillRewardAnnouncesRaidDrops(t *testing.T) {
	categories := []item.DropCategory{
		guaranteed(item.DropNormal, splitScrollID, 3),
		guaranteed(item.DropHerb, 8600, 1),
		guaranteed(item.DropNormal, splitThreadID, 4),
	}
	items := item.NewTable([]*item.Template{
		{ID: splitScrollID, Name: "scroll"},
		{ID: splitThreadID, Name: "thread", Stackable: true},
		{ID: 8600, Name: "herb", Stackable: true},
	})

	t.Run("ground", func(t *testing.T) {
		ground := &recordingGround{}
		a := &recordingAnnouncer{ground: ground}
		NewKillReward(categories, nil, 1, true, splitRates, false, false, &sequentialIDs{}, items, ground, nil, 0, 0, 0, 0, 9).
			MultipleItemDrop(true).AnnounceTo(a).CalculateRewards(nopKiller{id: 1})
		// The herb between them falls unnamed.
		want := [][3]int{{int(splitScrollID), 3, 3}, {int(splitThreadID), 4, 5}}
		if !slices.Equal(a.calls, want) {
			t.Fatalf("announcements (item, count, placed so far) = %v, want %v", a.calls, want)
		}
	})

	t.Run("auto-loot", func(t *testing.T) {
		ground := &recordingGround{}
		a := &recordingAnnouncer{ground: ground}
		killer := &lootKiller{id: 1}
		NewKillReward(categories, nil, 1, true, splitRates, true, false, &sequentialIDs{}, items, ground, nil, 0, 0, 0, 0, 9).
			MultipleItemDrop(true).AnnounceTo(a).CalculateRewards(killer)
		want := [][3]int{{int(splitScrollID), 3, 0}, {int(splitThreadID), 4, 1}}
		if !slices.Equal(a.calls, want) {
			t.Fatalf("announcements = %v, want %v", a.calls, want)
		}
		if killer.items[splitScrollID] != 3 || killer.items[splitThreadID] != 4 {
			t.Fatalf("looted %v, want both rolls", killer.items)
		}
	})

	t.Run("non-raid", func(t *testing.T) {
		ground := &recordingGround{}
		a := &recordingAnnouncer{ground: ground}
		NewKillReward(categories, nil, 1, false, splitRates, false, false, &sequentialIDs{}, items, ground, nil, 0, 0, 0, 0, 9).
			AnnounceTo(a).CalculateRewards(nopKiller{id: 1})
		if len(a.calls) != 0 {
			t.Fatalf("non-raid kill announced %v", a.calls)
		}
	})
}

// TestRewarderTreatsGrandBossAsRaid pins Monster.isRaidBoss for GrandBoss:
// its kill takes every raid branch of the drop path.
func TestRewarderTreatsGrandBossAsRaid(t *testing.T) {
	for _, tc := range []struct {
		kind string
		raid bool
	}{{"GrandBoss", true}, {"RaidBoss", true}, {"Monster", false}} {
		tmpl := &npc.Template{ID: 29001, TemplateID: 29001, Type: tc.kind, Level: 40, HPMax: 1000, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20}
		inst, err := npc.NewInstance(100, tmpl)
		if err != nil {
			t.Fatal(err)
		}
		hostile := &npc.Hostile{Instance: inst}
		rewards, _ := NewHostileRewarder(hostile, tmpl, nil, KillRewardConfig{}, nil, nil, nil)
		if got := rewards.(*deathRewards).raid; got != tc.raid {
			t.Errorf("%s: raid = %v, want %v", tc.kind, got, tc.raid)
		}
	}
}
