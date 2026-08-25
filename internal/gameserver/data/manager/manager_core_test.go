package manager

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// ---- from killrewards_test.go ----
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

// fakeGeo is a move.Geo test double whose ValidLocation is the only method
// KillReward's scatter path relies on.
type fakeGeo struct {
	validAt func(ox, oy, oz, tx, ty, tz int) location.Location
}

func (f fakeGeo) CanMove(ox, oy, oz, tx, ty, tz int) bool { return true }
func (f fakeGeo) Height(x, y, z int) int16                { return int16(z) }
func (f fakeGeo) FindPath(origin, target location.Location) ([]location.Location, bool) {
	return nil, false
}
func (f fakeGeo) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	if f.validAt != nil {
		return f.validAt(ox, oy, oz, tx, ty, tz)
	}
	return location.Location{X: tx, Y: ty, Z: tz}
}
func (f fakeGeo) Walkable(x, y, z int) bool { return true }

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

	r := NewKillReward(categories, nil, 1, false, rates, false, false, ids, items, ground, nil, 100, 200, 300, 45, 999)
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

func TestKillReward_ScattersDropAroundCorpseThroughGeoValidation(t *testing.T) {
	items := item.NewTable([]*item.Template{{ID: 57, Name: "adena"}})
	ground := &recordingGround{}
	ids := &sequentialIDs{}
	categories := []item.DropCategory{
		{Kind: item.DropCurrency, Chance: 100, Drops: []item.Drop{{ItemID: 57, Min: 1, Max: 1, Chance: 100}}},
	}
	rates := item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}

	var gotOX, gotOY, gotOZ, gotTX, gotTY int
	geo := fakeGeo{validAt: func(ox, oy, oz, tx, ty, tz int) location.Location {
		gotOX, gotOY, gotOZ, gotTX, gotTY = ox, oy, oz, tx, ty
		return location.Location{X: 555, Y: 666, Z: 300}
	}}

	r := NewKillReward(categories, nil, 1, false, rates, false, false, ids, items, ground, geo, 100, 200, 300, 45, 999)
	r.CalculateRewards(nopKiller{id: 1})

	if gotOX != 100 || gotOY != 200 || gotOZ != 300 {
		t.Fatalf("ValidLocation origin = (%d,%d,%d), want corpse position (100,200,300)", gotOX, gotOY, gotOZ)
	}
	if d := gotTX - 100; d < -dropScatterOffset || d > dropScatterOffset {
		t.Fatalf("scattered target X = %d, want within +/-%d of 100", gotTX, dropScatterOffset)
	}
	if d := gotTY - 200; d < -dropScatterOffset || d > dropScatterOffset {
		t.Fatalf("scattered target Y = %d, want within +/-%d of 200", gotTY, dropScatterOffset)
	}
	opts := ground.dropped[0]
	if opts.X != 555 || opts.Y != 666 || opts.Z != 300 {
		t.Fatalf("drop location = %+v, want the geo-validated point (555, 666, 300)", opts)
	}
}

func TestKillReward_ProtectsDropToPlayerKillerForRegularDuration(t *testing.T) {
	items := item.NewTable([]*item.Template{{ID: 57, Name: "adena"}})
	ground := &recordingGround{}
	ids := &sequentialIDs{}
	categories := []item.DropCategory{
		{Kind: item.DropCurrency, Chance: 100, Drops: []item.Drop{{ItemID: 57, Min: 1, Max: 1, Chance: 100}}},
	}
	rates := item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}

	killer := &lootKiller{id: 77}
	r := NewKillReward(categories, nil, 1, false, rates, false, false, ids, items, ground, nil, 0, 0, 0, 0, 0)
	r.CalculateRewards(killer)

	opts := ground.dropped[0]
	if opts.ProtectOwnerID != 77 {
		t.Fatalf("ProtectOwnerID = %d, want 77 (the killing player)", opts.ProtectOwnerID)
	}
	if opts.ProtectFor != regularLootProtection {
		t.Fatalf("ProtectFor = %v, want %v for a regular mob", opts.ProtectFor, regularLootProtection)
	}
}

func TestKillReward_ProtectsRaidDropForLongerDuration(t *testing.T) {
	items := item.NewTable([]*item.Template{{ID: 57, Name: "adena"}})
	ground := &recordingGround{}
	ids := &sequentialIDs{}
	categories := []item.DropCategory{
		{Kind: item.DropCurrency, Chance: 100, Drops: []item.Drop{{ItemID: 57, Min: 1, Max: 1, Chance: 100}}},
	}
	rates := item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}

	killer := &lootKiller{id: 77}
	r := NewKillReward(categories, nil, 1, true, rates, false, false, ids, items, ground, nil, 0, 0, 0, 0, 0)
	r.CalculateRewards(killer)

	opts := ground.dropped[0]
	if opts.ProtectFor != raidLootProtection {
		t.Fatalf("ProtectFor = %v, want %v for a raid boss", opts.ProtectFor, raidLootProtection)
	}
}

func TestKillReward_NonPlayerKillerGetsNoDropProtection(t *testing.T) {
	items := item.NewTable([]*item.Template{{ID: 57, Name: "adena"}})
	ground := &recordingGround{}
	ids := &sequentialIDs{}
	categories := []item.DropCategory{
		{Kind: item.DropCurrency, Chance: 100, Drops: []item.Drop{{ItemID: 57, Min: 1, Max: 1, Chance: 100}}},
	}
	rates := item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}

	r := NewKillReward(categories, nil, 1, false, rates, false, false, ids, items, ground, nil, 0, 0, 0, 0, 0)
	r.CalculateRewards(nopKiller{id: 1})

	opts := ground.dropped[0]
	if opts.ProtectOwnerID != 0 {
		t.Fatalf("ProtectOwnerID = %d, want 0 (no getActingPlayer() to protect)", opts.ProtectOwnerID)
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

	r := NewKillReward(categories, nil, 1, false, rates, false, false, ids, items, ground, nil, 0, 0, 0, 0, 0)
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
	r := NewKillReward(herbCategories(), nil, 1, false, rates, false, true, &sequentialIDs{}, herbTable(), ground, nil, 0, 0, 0, 0, 0)
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

	r := NewKillReward(herbCategories(), nil, 1, false, rates, false, true, &sequentialIDs{}, herbTable(), ground, nil, 0, 0, 0, 0, 0)
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
	r := NewKillReward(herbCategories(), nil, 1, false, rates, false, true, &sequentialIDs{}, herbTable(), ground, nil, 0, 0, 0, 0, 0)
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
	r := NewKillReward(herbCategories(), nil, 1, false, rates, false, true, &sequentialIDs{}, items, ground, nil, 0, 0, 0, 0, 0)
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
	r := NewKillReward(categories, nil, 1, false, rates, true, false, ids, items, ground, nil, 0, 0, 0, 0, 0)
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

	r := NewKillReward(categories, nil, 1, false, rates, false, false, failingIDs{}, items, ground, nil, 0, 0, 0, 0, 0)
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

// ---- from npcs_hostile_test.go ----
func TestCreatureActorRefSatisfiesTargetCreature(t *testing.T) {
	var _ skilltarget.Creature = (*creatureActorRef)(nil)
}

// ---- from npcs_territory_test.go ----
// constGeoZ.Height always returns the same z regardless of x, y, or the
// caller's average-z seed, so tests can isolate the Z-range-check logic
// from the geodata query itself.
type constGeoZ struct{ z int16 }

func (g constGeoZ) CanMove(int, int, int, int, int, int) bool { return true }
func (g constGeoZ) Height(int, int, int) int16                { return g.z }
func (constGeoZ) FindPath(_, _ location.Location) ([]location.Location, bool) {
	return nil, false
}
func (constGeoZ) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}
func (constGeoZ) Walkable(int, int, int) bool { return true }

func TestMergedZRangeIsMinOfMinsMaxOfMaxes(t *testing.T) {
	territories := []*spawn.Territory{
		{Name: "a", MinZ: -100, MaxZ: 50, Nodes: []spawn.Node{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 0, Y: 10}}},
		{Name: "b", MinZ: 20, MaxZ: 300, Nodes: []spawn.Node{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 0, Y: 10}}},
	}

	minZ, maxZ := mergedZRange(territories)
	if minZ != -100 || maxZ != 300 {
		t.Fatalf("mergedZRange() = (%d,%d), want (-100,300)", minZ, maxZ)
	}
}

func TestWeightedTerritoryPickFavorsLargerArea(t *testing.T) {
	big := &spawn.Territory{Name: "big", Nodes: []spawn.Node{{X: 0, Y: 0}, {X: 1000, Y: 0}, {X: 0, Y: 1000}}} // area 500000
	small := &spawn.Territory{Name: "small", Nodes: []spawn.Node{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 0, Y: 10}}} // area 50
	territories := []*spawn.Territory{big, small}

	const trials = 5000
	bigCount := 0
	for i := 0; i < trials; i++ {
		if weightedTerritoryPick(territories) == big {
			bigCount++
		}
	}

	// big is ~10000x small's area, so it should dominate selection; a loose
	// 90% floor distinguishes this from the old uniform 50/50 pick without
	// making the test flaky.
	if frac := float64(bigCount) / trials; frac < 0.90 {
		t.Fatalf("weightedTerritoryPick picked big %.3f of the time, want >0.90 (old uniform pick would give ~0.5)", frac)
	}
}

// TestRandomTerritoryPositionUsesMergedZRangeNotSubTerritoryOwnRange
// reproduces PR 552 review finding 1: a multi-territory maker's spawn-time
// Z check must validate against the merged min-of-mins/max-of-maxes Z range
// (SpawnManager.findTerritory, Territory.java's merged _minZ/_maxZ), not
// each sub-territory's own MinZ/MaxZ. Territory B's own range excludes the
// z the geo mock always returns, but the merged range (driven by A) includes
// it, so a fixed spawn-time Z check must still accept points landing in B.
func TestRandomTerritoryPositionUsesMergedZRangeNotSubTerritoryOwnRange(t *testing.T) {
	territoryA := &spawn.Territory{
		Name: "a", MinZ: 500, MaxZ: 500,
		Nodes: []spawn.Node{{X: 0, Y: 0}, {X: 100, Y: 0}, {X: 0, Y: 100}},
	}
	territoryB := &spawn.Territory{
		Name: "b", MinZ: -500, MaxZ: -500,
		Nodes: []spawn.Node{{X: 1000, Y: 0}, {X: 1100, Y: 0}, {X: 1000, Y: 100}},
	}
	maker := &spawn.Maker{Territories: []*spawn.Territory{territoryA, territoryB}}
	geo := constGeoZ{500}

	const trials = 2000
	inB := 0
	for i := 0; i < trials; i++ {
		pos, ok := randomTerritoryPosition(maker, geo)
		if !ok {
			t.Fatalf("randomTerritoryPosition() ok = false, want true")
		}
		if pos.Location.Z != 500 {
			t.Fatalf("randomTerritoryPosition() z = %d, want 500", pos.Location.Z)
		}
		if pos.Location.X >= 1000 {
			inB++
		}
	}

	// A and B have equal area, so an unbiased pick lands in B ~50% of the
	// time. Under the old per-territory Z check, B's own [-500,-500] never
	// accepts z=500, so B only survives via full-budget exhaustion
	// (probability ~0.5^10 ≈ 0.1%). The merged range accepts B immediately,
	// so a fixed implementation should land in B close to 50% of the time.
	if frac := float64(inB) / trials; frac < 0.20 {
		t.Fatalf("randomTerritoryPosition landed in territory B %.3f of the time, want >0.20 (old per-territory Z check would give ~0.001)", frac)
	}
}

// halfWalkableGeo models a territory whose geodata straddles walkable and
// unwalkable ground: every point with X < unwalkableMaxX is rejected, same
// as Territory.getRandomLocation's canMoveAround retry in the Java
// reference (aCis_gameserver Territory.java:138/193).
type halfWalkableGeo struct {
	unwalkableMaxX int
	walkableCalls  int
}

func (halfWalkableGeo) CanMove(int, int, int, int, int, int) bool { return true }
func (halfWalkableGeo) Height(_, _, z int) int16                  { return int16(z) }
func (halfWalkableGeo) FindPath(_, _ location.Location) ([]location.Location, bool) {
	return nil, false
}
func (halfWalkableGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

func (g *halfWalkableGeo) Walkable(x, _, _ int) bool {
	g.walkableCalls++
	return x >= g.unwalkableMaxX
}

func squareTerritory(minX, minY, maxX, maxY int) *spawn.Territory {
	return &spawn.Territory{
		Name: "half-walkable",
		MinZ: -100,
		MaxZ: 100,
		Nodes: []spawn.Node{
			{X: minX, Y: minY},
			{X: maxX, Y: minY},
			{X: maxX, Y: maxY},
			{X: minX, Y: maxY},
		},
	}
}

// TestRandomTerritoryPosition_RetriesUnwalkablePoints regression-tests #1716:
// a candidate point that fails the walkability check must be retried within
// the existing territorySpawnAttempts budget rather than accepted outright.
// The unwalkable band covers only a fifth of the territory's width, so the
// 10-attempt budget finds a walkable point with overwhelming probability;
// this asserts every returned placement over many runs lands on the
// walkable side.
func TestRandomTerritoryPosition_RetriesUnwalkablePoints(t *testing.T) {
	territory := squareTerritory(0, 0, 100, 100)
	maker := &spawn.Maker{Territories: []*spawn.Territory{territory}}
	geo := &halfWalkableGeo{unwalkableMaxX: 20}

	for i := 0; i < 300; i++ {
		pos, ok := randomTerritoryPosition(maker, geo)
		if !ok {
			t.Fatalf("run %d: randomTerritoryPosition returned ok=false", i)
		}
		if pos.Location.X < 20 {
			t.Fatalf("run %d: placed NPC at unwalkable X=%d, want >= 20", i, pos.Location.X)
		}
	}

	if geo.walkableCalls == 0 {
		t.Fatal("Walkable was never called; randomTerritoryPosition is not checking geodata")
	}
}

// TestRandomTerritoryPosition_FallsBackWhenNeverWalkable matches
// Territory.getRandomLocation's exhaustion behavior: when every candidate
// fails the walkability check, the last rolled position is still returned
// rather than failing placement outright.
func TestRandomTerritoryPosition_FallsBackWhenNeverWalkable(t *testing.T) {
	territory := squareTerritory(0, 0, 100, 100)
	maker := &spawn.Maker{Territories: []*spawn.Territory{territory}}
	geo := &halfWalkableGeo{unwalkableMaxX: 1000} // rejects every point

	pos, ok := randomTerritoryPosition(maker, geo)
	if !ok {
		t.Fatal("randomTerritoryPosition returned ok=false, want fallback position")
	}
	if pos.Location.X < 0 || pos.Location.X > 100 {
		t.Fatalf("fallback position X=%d outside territory bounds", pos.Location.X)
	}
	if geo.walkableCalls != territorySpawnAttempts {
		t.Fatalf("walkableCalls = %d, want %d (one per attempt, all exhausted)", geo.walkableCalls, territorySpawnAttempts)
	}
}

// ---- from spawns_test.go ----
func TestNewSpawnsCreatesMissingStateRows(t *testing.T) {
	dir := t.TempDir()
	writeSpawnFixture(t, filepath.Join(dir, "20_20.xml"), `
<list>
	<territory name="field" minZ="-10" maxZ="10">
		<node x="0" y="0"/>
		<node x="100" y="0"/>
		<node x="100" y="100"/>
		<node x="0" y="100"/>
	</territory>
	<npcmaker name="maker" territory="field" maximumNpcs="2">
		<npc id="1" total="1" dbName="existing"/>
		<npc id="2" total="1" dbName="missing"/>
		<npc id="3" total="1"/>
	</npcmaker>
</list>`)

	table, err := xml.LoadSpawnlist(dir)
	if err != nil {
		t.Fatalf("LoadSpawnlist() unexpected error: %v", err)
	}

	existing := &spawn.State{Name: "existing", Status: spawn.StatusAlive, CurrentHP: 10}
	spawns := NewSpawns(table, map[string]*spawn.State{"existing": existing})

	if got, ok := spawns.State("existing"); !ok || got != existing {
		t.Fatalf("State(existing) = %p, %v; want original row", got, ok)
	}
	missing, ok := spawns.State("missing")
	if !ok {
		t.Fatal("State(missing) = missing")
	}
	if missing.Status != spawn.StatusUninitialized {
		t.Fatalf("missing status = %d, want %d", missing.Status, spawn.StatusUninitialized)
	}
	if got, ok := spawns.State(""); ok || got != nil {
		t.Fatalf("State(empty) = %p, %v; want nil, false", got, ok)
	}
	if got, want := spawns.StateCount(), 2; got != want {
		t.Fatalf("StateCount() = %d, want %d", got, want)
	}
}

func writeSpawnFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(`<?xml version="1.0" encoding="utf-8"?>`+body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
