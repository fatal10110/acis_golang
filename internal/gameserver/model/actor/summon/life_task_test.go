package summon

import (
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// spawnOnClock spawns a with rec attached, its work on a queue of the inline
// clock it returns.
func spawnOnClock(t *testing.T, a *Actor) (*sim.Inline, *event.Recorder) {
	t.Helper()
	clock := sim.NewInline(time.Unix(0, 0))
	a.SetQueue(clock.NewQueue("summon"))
	rec := &event.Recorder{}
	a.Attach(Runtime{Sink: rec})
	SpawnBesideOwner(world.New(), a, &fakeSummonOwner{id: 1}, location.Location{})
	return clock, rec
}

// eventTypes renders the recorded events of the types tracked here as
// names, in emit order.
func eventTypes(rec *event.Recorder) []string {
	var got []string
	for _, e := range rec.Events() {
		switch e.(type) {
		case event.LifetimeShown, event.EffectIconsChanged, event.UpkeepPaid, event.UpkeepShort,
			event.PetFoodDue, event.PetHungry, event.PetLeavingHungry, event.PetDeserted,
			event.MoveTypeChanged, event.Unsummoning, event.Despawned:
			got = append(got, fmt.Sprintf("%T", e))
		}
	}
	return got
}

// A servitor's lifetime task ticks once a second on its queue: each tick
// shows the remaining time, then refreshes the effect icons. At its upkeep
// checkpoint an owner with no inventory to pay from loses it.
func TestServitorLifetimeTaskTicksOnItsQueue(t *testing.T) {
	a := mustServitor(t, ServitorConfig{
		ObjectID: 7,
		Lifetime: LifetimeState{
			TimeRemaining: 3520, TotalLifeTime: 3520,
			NextItemConsumeTime: InitialNextConsumeTime(3520, 1, 1458), ItemConsumeSteps: 1,
		},
		TimeLostIdle: 500, TimeLostActive: 1000, ItemConsumeID: 1458, ItemConsumeCount: 1,
	})
	clock, rec := spawnOnClock(t, a)

	clock.AdvanceBefore(time.Second)
	if got := a.Lifetime().TimeRemaining; got != 3520 {
		t.Fatalf("lifetime before the first tick = %d, want 3520", got)
	}
	clock.Advance(time.Millisecond)
	if got := a.Lifetime().TimeRemaining; got != 3020 {
		t.Fatalf("lifetime after one tick = %d, want 3020", got)
	}
	shown := []string{"event.LifetimeShown", "event.EffectIconsChanged"}
	if got := eventTypes(rec); fmt.Sprint(got) != fmt.Sprint(shown) {
		t.Fatalf("one tick emitted %v, want %v", got, shown)
	}

	// The checkpoint at 1760 falls on the fourth tick (1520).
	clock.Advance(3 * time.Second)
	want := append(append(append([]string{}, shown...), shown...), shown...)
	want = append(want, "event.UpkeepShort", "event.Unsummoning", "event.Despawned")
	if got := eventTypes(rec); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("ticks to the checkpoint emitted %v, want %v", got, want)
	}
	clock.Advance(5 * time.Second)
	if got := a.Lifetime().TimeRemaining; got != 1520 {
		t.Fatalf("lifetime after the servitor left = %d, want 1520", got)
	}
}

// A pet's feeding ticks every ten seconds on its queue. Under its hungry
// limit with no food it is very hungry and, on a roll that comes up, leaves
// for good; one that stays walks.
func TestPetFeedTaskTicksOnItsQueue(t *testing.T) {
	for _, tc := range []struct {
		name string
		roll int
		want []string
	}{
		{"stays", 99, []string{"event.PetHungry", "event.MoveTypeChanged"}},
		{"leaves", 0, []string{"event.PetHungry", "event.PetLeavingHungry", "event.Unsummoning", "event.Despawned", "event.PetDeserted"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := mustPet(t, PetConfig{
				ObjectID: 7, Fed: 5, MaxMeal: 3000, MealInNormal: 5, MealInBattle: 10,
				AutoFeedLimit: 0.55, HungryLimit: 0.3, UnsummonLimit: 0.1,
				RunSpeed: 120, WalkSpeed: 60,
				Roll: func(int) int { return tc.roll },
			})
			clock, rec := spawnOnClock(t, a)

			clock.AdvanceBefore(10 * time.Second)
			if got := a.Fed(); got != 5 {
				t.Fatalf("meal gauge before the first meal = %d, want 5", got)
			}
			clock.Advance(time.Millisecond)
			if got := a.Fed(); got != 0 {
				t.Fatalf("meal gauge after the meal = %d, want 0", got)
			}
			if got := eventTypes(rec); fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("meal emitted %v, want %v", got, tc.want)
			}
			for _, e := range rec.Events() {
				if hungry, ok := e.(event.PetHungry); ok && !hungry.Starving {
					t.Fatal("PetHungry at an empty gauge is not starving")
				}
			}
			if tc.roll == 99 && a.Running() {
				t.Fatal("starving pet still runs")
			}
		})
	}
}

// A dead summon's lifetime task stops; a revived pet's starts again, its
// first meal a full period after the revive, and a revived servitor's stays
// stopped.
func TestRevivedSummonLifetimeTask(t *testing.T) {
	pet := mustPet(t, PetConfig{
		ObjectID: 7, Fed: 3000, MaxMeal: 3000, MealInNormal: 5, MealInBattle: 10,
		AutoFeedLimit: 0.55, HungryLimit: 0.3, RespawnRestoreHP: 0.7,
		Stats: CombatStats{MaxHP: 100, MaxMP: 10},
	})
	servitor := mustServitor(t, ServitorConfig{
		ObjectID: 8, Lifetime: LifetimeState{TimeRemaining: 35200, TotalLifeTime: 35200, NextItemConsumeTime: -1},
		TimeLostIdle: 500, RespawnRestoreHP: 0.7, Stats: CombatStats{MaxHP: 100, MaxMP: 10},
	})
	petClock, _ := spawnOnClock(t, pet)
	servitorClock, _ := spawnOnClock(t, servitor)

	pet.Kill(nil)
	servitor.Kill(nil)
	petClock.Advance(20 * time.Second)
	servitorClock.Advance(3 * time.Second)
	if pet.Fed() != 3000 || servitor.Lifetime().TimeRemaining != 35200 {
		t.Fatalf("dead pet fed %d, dead servitor lifetime %d, want 3000 and 35200 kept", pet.Fed(), servitor.Lifetime().TimeRemaining)
	}

	if !pet.Revive() || !servitor.Revive() {
		t.Fatal("revive refused")
	}
	petClock.AdvanceBefore(10 * time.Second)
	if got := pet.Fed(); got != 3000 {
		t.Fatalf("revived pet ate before a feed period passed: %d", got)
	}
	petClock.Advance(time.Millisecond)
	if got := pet.Fed(); got != 2995 {
		t.Fatalf("revived pet gauge after its meal = %d, want 2995", got)
	}
	servitorClock.Advance(3 * time.Second)
	if got := servitor.Lifetime().TimeRemaining; got != 35200 {
		t.Fatalf("revived servitor lifetime = %d, want 35200 kept", got)
	}
}
