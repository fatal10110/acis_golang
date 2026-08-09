package summon

import (
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type liveOwnerStub struct {
	world.Presence
	id    int32
	level int
}

func (o *liveOwnerStub) ObjectID() int32 { return o.id }
func (o *liveOwnerStub) LevelValue() int { return o.level }

type liveOwnerCombatant struct {
	liveOwnerStub
}

func (o *liveOwnerCombatant) SiegeGuard() bool { return false }
func (o *liveOwnerCombatant) AlikeDead() bool  { return false }

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

func TestPetTracksItemSkillReuse(t *testing.T) {
	pet := NewPet(PetConfig{ObjectID: 2, NPCID: 12077})
	ref := modelskill.Ref{ID: 2278, Level: 1}
	const key int32 = 2278*256 + 1

	pet.DisableSkill(key, time.Minute)
	pet.AddSkillReuse(ref, key, time.Minute)

	if !pet.SkillDisabled(key) {
		t.Fatal("SkillDisabled() = false after pet item reuse was installed")
	}
	if got := pet.ShortBuffTaskSkillID(); got != 0 {
		t.Fatalf("ShortBuffTaskSkillID() = %d, want 0 for a pet", got)
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

func TestApplyCommandDispatchesValidatedCommandsToAI(t *testing.T) {
	tests := []struct {
		name             string
		setup            func(*Actor)
		ctx              CommandContext
		wantEvents       []string
		wantIntent       Intent
		wantFollowActive bool
	}{
		{
			name: "attackable creature",
			ctx: CommandContext{
				Command:          CommandAttack,
				Target:           &liveCombatant{id: 300},
				TargetIsCreature: true,
				TargetAttackable: true,
			},
			wantEvents:       []string{"attack:300"},
			wantIntent:       IntentAttackTarget,
			wantFollowActive: true,
		},
		{
			name: "non-attackable creature",
			ctx: CommandContext{
				Command:          CommandAttack,
				Target:           &liveCombatant{id: 301},
				TargetIsCreature: true,
			},
			wantEvents:       []string{"follow:301"},
			wantIntent:       IntentFollowTarget,
			wantFollowActive: true,
		},
		{
			name:             "stop",
			ctx:              CommandContext{Command: CommandStop},
			wantEvents:       []string{"idle"},
			wantIntent:       IntentIdle,
			wantFollowActive: true,
		},
		{
			name: "follow owner",
			setup: func(actor *Actor) {
				actor.followActive = false
			},
			ctx:              CommandContext{Command: CommandToggleFollow},
			wantEvents:       []string{"follow:100"},
			wantIntent:       IntentFollowOwner,
			wantFollowActive: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner := &liveOwnerCombatant{liveOwnerStub: liveOwnerStub{id: 100, level: 40}}
			actor := NewServitor(ServitorConfig{ObjectID: 200, Owner: owner, Level: 44})
			if tt.setup != nil {
				tt.setup(actor)
			}
			brain := &recordingSummonAI{}
			actor.SetAI(brain)

			result := actor.ApplyCommand(tt.ctx)
			if result.Outcome != OutcomeApplied {
				t.Fatalf("outcome = %v, want OutcomeApplied", result.Outcome)
			}
			if !reflect.DeepEqual(brain.events, tt.wantEvents) {
				t.Fatalf("AI events = %#v, want %#v", brain.events, tt.wantEvents)
			}
			if actor.Intent() != tt.wantIntent {
				t.Fatalf("Intent() = %v, want %v", actor.Intent(), tt.wantIntent)
			}
			if actor.FollowActive() != tt.wantFollowActive {
				t.Fatalf("FollowActive() = %v, want %v", actor.FollowActive(), tt.wantFollowActive)
			}
		})
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
	if actor.PetInventory().WeightLimit != 109020 {
		t.Fatalf("pet inventory WeightLimit = %d, want %d", actor.PetInventory().WeightLimit, 109020)
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

// TestServitorLifetimeConcurrentTickAndRead overlaps TickServitor's writes
// (via StartServitorTicks) with concurrent Lifetime() reads, the same
// producer/consumer pattern petInfoSnapshot uses from the world-visibility
// goroutine. Run with -race: without statusMu guarding lifetime, this
// detects the race #1282 fixed.
func TestServitorLifetimeConcurrentTickAndRead(t *testing.T) {
	state := world.New()
	owner := &liveOwnerStub{id: 100, level: 40}
	state.Spawn(owner, 1000, 2000, -50, 32768)
	actor := NewServitor(ServitorConfig{
		ObjectID:     200,
		Owner:        owner,
		Level:        44,
		TimeLostIdle: 1,
		Lifetime: LifetimeState{
			TimeRemaining:       100000,
			TotalLifeTime:       100000,
			NextItemConsumeTime: -1,
		},
	})
	SpawnBesideOwner(state, actor, owner, location.Location{})

	ticker := actor.StartServitorTicks(time.Millisecond, state, zerolog.Nop())
	defer ticker.Stop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		deadline := time.Now().Add(20 * time.Millisecond)
		for time.Now().Before(deadline) {
			_ = actor.Lifetime()
		}
	}()
	<-done
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

func TestActorGetSkillCanUseSkillTryUseSkill(t *testing.T) {
	owner := &liveOwnerStub{id: 100, level: 40}
	target := &liveCombatant{id: 300}

	t.Run("unknown skill id returns false", func(t *testing.T) {
		actor := NewPet(PetConfig{ObjectID: 200, Owner: owner, Level: 40, Skills: map[int]int{4139: 8}})
		brain := &recordingSummonAI{}
		actor.SetAI(brain)

		if actor.TryUseSkill(9999, target) {
			t.Fatal("TryUseSkill = true for a skill id absent from the template's skill map")
		}
		if len(brain.events) != 0 {
			t.Fatalf("AI events = %v, want none", brain.events)
		}
	})

	t.Run("pet within level gap dispatches to AI", func(t *testing.T) {
		actor := NewPet(PetConfig{ObjectID: 200, Owner: owner, Level: 60, Skills: map[int]int{4139: 8}})
		brain := &recordingSummonAI{}
		actor.SetAI(brain)

		if !actor.CanUseSkill() {
			t.Fatal("CanUseSkill = false for a 20-level gap, want true (gate is strictly > 20)")
		}
		if !actor.TryUseSkill(4139, target) {
			t.Fatal("TryUseSkill = false, want true")
		}
		want := []string{"cast:300"}
		if !reflect.DeepEqual(brain.events, want) {
			t.Fatalf("AI events = %v, want %v", brain.events, want)
		}
	})

	t.Run("pet beyond level gap is blocked without touching the AI", func(t *testing.T) {
		actor := NewPet(PetConfig{ObjectID: 200, Owner: owner, Level: 61, Skills: map[int]int{4139: 8}})
		brain := &recordingSummonAI{}
		actor.SetAI(brain)

		if actor.CanUseSkill() {
			t.Fatal("CanUseSkill = true for a 21-level gap, want false")
		}
		if actor.TryUseSkill(4139, target) {
			t.Fatal("TryUseSkill = true, want false")
		}
		if len(brain.events) != 0 {
			t.Fatalf("AI events = %v, want none", brain.events)
		}
	})

	t.Run("servitor has no level gate", func(t *testing.T) {
		actor := NewServitor(ServitorConfig{ObjectID: 200, Owner: owner, Level: 90, Skills: map[int]int{4139: 8}})
		brain := &recordingSummonAI{}
		actor.SetAI(brain)

		if !actor.CanUseSkill() {
			t.Fatal("CanUseSkill = false for a servitor, want true regardless of level gap")
		}
		if !actor.TryUseSkill(4139, target) {
			t.Fatal("TryUseSkill = false, want true")
		}
	})

	t.Run("skill ref carries the template-granted level", func(t *testing.T) {
		actor := NewPet(PetConfig{ObjectID: 200, Owner: owner, Level: 40, Skills: map[int]int{4139: 8}})
		ref, ok := actor.GetSkill(4139)
		if !ok || ref.Level != 8 {
			t.Fatalf("GetSkill(4139) = %+v, %v, want level 8, true", ref, ok)
		}
	})

	t.Run("AI rejection of the dispatched cast does not flip the result", func(t *testing.T) {
		// Matches Java's useSkill (RequestActionUse.java:453-472): it
		// unconditionally returns true once the dispatch preconditions pass,
		// because summon.getAI().tryToCast(...) (PlayableAI.java:297) is void
		// and cannot feed a rejection (cooldown, out of control, no MP, ...)
		// back into the return value.
		actor := NewPet(PetConfig{ObjectID: 200, Owner: owner, Level: 40, Skills: map[int]int{4139: 8}})
		brain := &rejectingSummonAI{}
		actor.SetAI(brain)

		if !actor.TryUseSkill(4139, target) {
			t.Fatal("TryUseSkill = false, want true even though the AI rejected the cast")
		}
		want := []string{"cast:300"}
		if !reflect.DeepEqual(brain.events, want) {
			t.Fatalf("AI events = %v, want %v", brain.events, want)
		}
	})
}

type recordingSummonAI struct {
	events []string
}

func (a *recordingSummonAI) TryToAttack(target attackable.Combatant) bool {
	a.events = append(a.events, "attack:"+objectIDString(target))
	return true
}

func (a *recordingSummonAI) TryToFollow(target attackable.Combatant) bool {
	a.events = append(a.events, "follow:"+objectIDString(target))
	return true
}

func (a *recordingSummonAI) TryToIdle() {
	a.events = append(a.events, "idle")
}

func (a *recordingSummonAI) TryToCast(target attackable.Combatant, ref modelskill.Ref) bool {
	a.events = append(a.events, "cast:"+objectIDString(target))
	return true
}

// rejectingSummonAI records the cast dispatch like recordingSummonAI but
// reports it rejected, simulating the AI declining the cast (cooldown,
// out of control, no MP, ...) after the dispatch already happened.
type rejectingSummonAI struct {
	recordingSummonAI
}

func (a *rejectingSummonAI) TryToCast(target attackable.Combatant, ref modelskill.Ref) bool {
	a.events = append(a.events, "cast:"+objectIDString(target))
	return false
}

type liveCombatant struct {
	id   int32
	dead bool
}

func (c *liveCombatant) ObjectID() int32  { return c.id }
func (c *liveCombatant) SiegeGuard() bool { return false }
func (c *liveCombatant) AlikeDead() bool  { return c.dead }

func objectIDString(target attackable.Combatant) string {
	if target == nil {
		return "nil"
	}
	return strconv.FormatInt(int64(target.ObjectID()), 10)
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
