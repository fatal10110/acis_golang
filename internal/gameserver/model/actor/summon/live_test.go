package summon

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type liveOwnerStub struct {
	world.Presence
	id    int32
	level int
}

func (o *liveOwnerStub) ObjectID() int32 { return o.id }
func (o *liveOwnerStub) LevelValue() int { return o.level }

func TestSpawnBesideOwnerRegistersLiveSummon(t *testing.T) {
	state := world.New()
	owner := &liveOwnerStub{id: 100, level: 40}
	state.Spawn(owner, 1000, 2000, -50, 32768)
	state.AddPlayer(owner)

	actor := NewServitor(ServitorConfig{ObjectID: 200, Owner: owner, Level: 44})
	SpawnBesideOwner(state, actor, owner, location.Location{X: 75})

	if _, ok := state.Object(actor.ObjectID()); !ok {
		t.Fatalf("world object %d missing after SpawnBesideOwner", actor.ObjectID())
	}
	got, ok := state.Summon(owner.ObjectID())
	if !ok || got != actor {
		t.Fatalf("Summon(%d) = %+v, %v, want actor,true", owner.ObjectID(), got, ok)
	}
	x, y, z := actor.Position()
	if x != 1075 || y != 2000 || z != -50 {
		t.Fatalf("actor position = (%d,%d,%d), want (1075,2000,-50)", x, y, z)
	}
}

func TestApplyCommandTogglesFollowAndUnsummonsServitor(t *testing.T) {
	state := world.New()
	owner := &liveOwnerStub{id: 100, level: 40}
	state.Spawn(owner, 1000, 2000, -50, 32768)

	actor := NewServitor(ServitorConfig{ObjectID: 200, Owner: owner, Level: 44})
	SpawnBesideOwner(state, actor, owner, location.Location{})

	if !actor.FollowActive() {
		t.Fatal("new servitor should start in follow mode")
	}
	result := actor.ApplyCommand(CommandContext{Command: CommandToggleFollow, World: state})
	if result.Outcome != OutcomeApplied {
		t.Fatalf("toggle follow outcome = %v, want OutcomeApplied", result.Outcome)
	}
	if actor.FollowActive() || actor.Intent() != IntentIdle {
		t.Fatalf("after toggle off: FollowActive=%v Intent=%v, want false,IntentIdle", actor.FollowActive(), actor.Intent())
	}

	result = actor.ApplyCommand(CommandContext{Command: CommandUnsummonServitor, World: state})
	if result.Outcome != OutcomeApplied {
		t.Fatalf("unsummon outcome = %v, want OutcomeApplied", result.Outcome)
	}
	if _, ok := state.Object(actor.ObjectID()); ok {
		t.Fatalf("world object %d still present after unsummon", actor.ObjectID())
	}
	if _, ok := state.Summon(owner.ObjectID()); ok {
		t.Fatalf("owner %d still has active summon after unsummon", owner.ObjectID())
	}
}

func TestServitorTickConsumesOwnerUpkeepAndUnsummonsOnMissingItem(t *testing.T) {
	const upkeepID int32 = 57
	templates := item.NewTable([]*item.Template{{ID: upkeepID, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}}})
	inventory := itemcontainer.NewPlayerInventory(100, templates)
	inventory.AddNew(upkeepID, 1, 300)

	state := world.New()
	owner := &liveOwnerStub{id: 100, level: 40}
	state.Spawn(owner, 1000, 2000, -50, 32768)

	actor := NewServitor(ServitorConfig{
		ObjectID:         200,
		Owner:            owner,
		Level:            44,
		OwnerInventory:   inventory,
		ItemConsumeID:    upkeepID,
		ItemConsumeCount: 1,
		TimeLostIdle:     2000,
		Lifetime: LifetimeState{
			TimeRemaining:       10000,
			TotalLifeTime:       10000,
			NextItemConsumeTime: 5000,
			ItemConsumeSteps:    1,
		},
	})
	SpawnBesideOwner(state, actor, owner, location.Location{})

	for i := 0; i < 3; i++ {
		result := actor.TickServitor(state)
		if i < 2 && result.UpkeepDue {
			t.Fatalf("tick %d upkeep due too early", i+1)
		}
		if i == 2 && (!result.UpkeepDue || !result.UpkeepConsumed || result.Unsummoned) {
			t.Fatalf("checkpoint tick = %+v, want upkeep due+consumed without unsummon", result)
		}
	}
	if got := inventory.ItemCount(upkeepID, -1, true); got != 0 {
		t.Fatalf("owner upkeep item count = %d, want 0", got)
	}

	result := actor.TickServitor(state)
	if result.Unsummoned {
		t.Fatalf("tick after paid checkpoint unsummoned early: %+v", result)
	}
	result = actor.TickServitor(state)
	if !result.UpkeepDue || !result.Unsummoned {
		t.Fatalf("second checkpoint without item = %+v, want upkeep due and unsummoned", result)
	}
	if _, ok := state.Summon(owner.ObjectID()); ok {
		t.Fatal("owner still has active summon after unpaid upkeep")
	}
}

func TestPetTickAutoFeedsFromPetInventory(t *testing.T) {
	const foodID int32 = 2515
	templates := item.NewTable([]*item.Template{{ID: foodID, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}}})
	inventory := itemcontainer.NewPetInventory(200, templates)
	inventory.AddNew(foodID, 1, 300)

	state := world.New()
	owner := &liveOwnerStub{id: 100, level: 40}
	state.Spawn(owner, 1000, 2000, -50, 32768)

	actor := NewPet(PetConfig{
		ObjectID:      200,
		Owner:         owner,
		Level:         44,
		Inventory:     inventory,
		Fed:           60,
		MaxMeal:       100,
		MealInNormal:  10,
		Food1:         foodID,
		FoodRestore:   30,
		AutoFeedLimit: 0.55,
		HungryLimit:   0.50,
		UnsummonLimit: 0.40,
	})
	SpawnBesideOwner(state, actor, owner, location.Location{})

	result := actor.TickPet(state)
	if !result.AutoFed || result.Unsummoned {
		t.Fatalf("TickPet() = %+v, want auto-fed without unsummon", result)
	}
	if got := actor.Fed(); got != 80 {
		t.Fatalf("Fed() = %d, want 80", got)
	}
	if got := inventory.ItemCount(foodID, -1, true); got != 0 {
		t.Fatalf("pet food count = %d, want 0", got)
	}
}

func TestNewPetAppliesConfiguredInventoryLimits(t *testing.T) {
	inventory := itemcontainer.NewPetInventory(200, item.NewTable(nil))
	cfg := petmodel.DefaultConfig()
	cfg.ExpRate = 1.5
	cfg.SinEaterExpRate = 4.0
	cfg.InventorySlots = 7
	cfg.WeightLimitMultiplier = 2.0

	actor := NewPet(PetConfig{
		ObjectID:  200,
		NPCID:     12077,
		CON:       43,
		Inventory: inventory,
		Config:    &cfg,
	})

	if actor.PetInventory().SlotLimit != 7 {
		t.Fatalf("pet inventory SlotLimit = %d, want 7", actor.PetInventory().SlotLimit)
	}
	if want := int(34500 * statbonus.CONBonus[43] * 2.0); actor.PetInventory().WeightLimit != want {
		t.Fatalf("pet inventory WeightLimit = %d, want %d", actor.PetInventory().WeightLimit, want)
	}
	if got := actor.ScaledExpGain(1000); got != 1500 {
		t.Fatalf("ScaledExpGain(ordinary pet) = %d, want 1500", got)
	}

	sinEater := NewPet(PetConfig{ObjectID: 201, NPCID: 12564, Config: &cfg})
	if got := sinEater.ScaledExpGain(1000); got != 4000 {
		t.Fatalf("ScaledExpGain(sin eater) = %d, want 4000", got)
	}
}

func TestPetTickStarvationCanMakePetLeaveOwner(t *testing.T) {
	state := world.New()
	owner := &liveOwnerStub{id: 100, level: 40}
	state.Spawn(owner, 1000, 2000, -50, 32768)

	actor := NewPet(PetConfig{
		ObjectID:      200,
		Owner:         owner,
		Level:         44,
		Fed:           1,
		MaxMeal:       100,
		MealInNormal:  1,
		AutoFeedLimit: 0.55,
		HungryLimit:   0.50,
		UnsummonLimit: 0.40,
		Roll:          func(int) int { return 0 },
	})
	SpawnBesideOwner(state, actor, owner, location.Location{})

	result := actor.TickPet(state)
	if result.Starvation != petmodel.StarvationSevere || !result.LeftOwner || !result.Unsummoned {
		t.Fatalf("TickPet() = %+v, want severe starvation leaving owner", result)
	}
	if _, ok := state.Summon(owner.ObjectID()); ok {
		t.Fatal("owner still has active summon after starving pet left")
	}
}

func TestStartServitorTicksSchedulesLifetimeExpiry(t *testing.T) {
	state := world.New()
	owner := &liveOwnerStub{id: 100, level: 40}
	state.Spawn(owner, 1000, 2000, -50, 32768)
	actor := NewServitor(ServitorConfig{
		ObjectID:     200,
		Owner:        owner,
		Level:        44,
		TimeLostIdle: 10,
		Lifetime: LifetimeState{
			TimeRemaining:       5,
			TotalLifeTime:       5,
			NextItemConsumeTime: -1,
		},
	})
	SpawnBesideOwner(state, actor, owner, location.Location{})

	ticker := actor.StartServitorTicks(5*time.Millisecond, state, zerolog.Nop())
	defer ticker.Stop()
	waitForNoSummon(t, state, owner.ObjectID())
}

func TestStartPetFeedSchedulesStarvation(t *testing.T) {
	state := world.New()
	owner := &liveOwnerStub{id: 100, level: 40}
	state.Spawn(owner, 1000, 2000, -50, 32768)
	actor := NewPet(PetConfig{
		ObjectID:     200,
		Owner:        owner,
		Level:        44,
		Fed:          1,
		MaxMeal:      100,
		MealInNormal: 1,
		Roll:         func(int) int { return 0 },
	})
	SpawnBesideOwner(state, actor, owner, location.Location{})

	ticker := actor.StartPetFeed(5*time.Millisecond, state, zerolog.Nop())
	defer ticker.Stop()
	waitForNoSummon(t, state, owner.ObjectID())
}

func waitForNoSummon(t *testing.T, state *world.State, ownerID int32) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		if _, ok := state.Summon(ownerID); !ok {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("owner %d still has active summon", ownerID)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
