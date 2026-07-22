package task

import (
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// fakePlayerClockActor is the minimum worldobject.Object + PlayerClockActor
// needed to drive PlayerClock tests: identity, a known-skills map mirrored
// by SetSkillLevel, and an online flag.
type fakePlayerClockActor struct {
	id    int32
	mu    sync.Mutex
	known map[int]int
}

func newFakeActor(id int32, skills ...int) *fakePlayerClockActor {
	known := make(map[int]int, len(skills))
	for _, s := range skills {
		known[s] = 1
	}
	return &fakePlayerClockActor{id: id, known: known}
}

func (a *fakePlayerClockActor) ObjectID() int32 { return a.id }
func (a *fakePlayerClockActor) HasSkill(skillID int) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.known[skillID]
	return ok
}
func (a *fakePlayerClockActor) SetSkillLevel(skillID, level int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if level <= 0 {
		delete(a.known, skillID)
		return
	}
	a.known[skillID] = level
}

// fakePlayerClockEffects records the order of player-visible notifications
// PlayerClock produces, so tests assert exactly what reached the player.
type fakePlayerClockEffects struct {
	mu      sync.Mutex
	tooLong []int32
	skilled []skillTransition
}

type skillTransition struct {
	actorID int32
	night   bool
	skillID int32
	level   int32
}

func (e *fakePlayerClockEffects) NotifyPlayingTooLong(actorID int32) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tooLong = append(e.tooLong, actorID)
}

func (e *fakePlayerClockEffects) NotifyDayNightSkillTransition(actorID int32, night bool, skillID, level int32) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.skilled = append(e.skilled, skillTransition{actorID: actorID, night: night, skillID: skillID, level: level})
}

func (e *fakePlayerClockEffects) snapshot() ([]int32, []skillTransition) {
	e.mu.Lock()
	defer e.mu.Unlock()
	tooLong := append([]int32(nil), e.tooLong...)
	skilled := append([]skillTransition(nil), e.skilled...)
	return tooLong, skilled
}

// newTestGameClockFixedAt returns a GameClock whose absolute minutes
// counter is preset to minutes; its `night` flag is set to match, mimicking
// NewGameClock's initialization. Tests can mutate `.minutes` across ticks
// (GameClock.Minutes reads under its RLock) or call Tick to drive boundary
// crossings through the clock's listener dispatch.
func newTestGameClockFixedAt(minutes int) *GameClock {
	night := minutes%minutesPerDay < nightEndMinute
	return &GameClock{now: time.Now, minutes: minutes, night: night}
}

func TestPlayerClockNewRejectsNilDependencies(t *testing.T) {
	if _, err := NewPlayerClock(nil, world.New(), &fakePlayerClockEffects{}); err == nil {
		t.Fatal("nil clock should be rejected")
	}
	clock := &GameClock{}
	if _, err := NewPlayerClock(clock, world.New(), nil); err == nil {
		t.Fatal("nil effects should be rejected")
	}
}

func TestPlayerClockAddSchedulesFirstReminder(t *testing.T) {
	clock := newTestGameClockFixedAt(0)
	effects := &fakePlayerClockEffects{}
	pc, err := NewPlayerClock(clock, world.New(), effects)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pc.Add(newFakeActor(42)) // registered at minute 0; first due at 0 + 720 = 720.

	clock.minutes = 719
	pc.Tick()
	if tooLong, _ := effects.snapshot(); len(tooLong) != 0 {
		t.Fatalf("reminder fired at minute 719: %v", tooLong)
	}

	clock.minutes = 720
	pc.Tick()
	if tooLong, _ := effects.snapshot(); len(tooLong) != 1 || tooLong[0] != 42 {
		t.Fatalf("Tick at minute 720 want [42], got %v", tooLong)
	}
}

func TestPlayerClockActivityReminderRunsOnGameClockTick(t *testing.T) {
	clock := newTestGameClockFixedAt(0)
	effects := &fakePlayerClockEffects{}
	pc, err := NewPlayerClock(clock, world.New(), effects)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pc.Add(newFakeActor(21))
	clock.minutes = activityReminderMinutes - 1
	clock.Tick()

	if tooLong, _ := effects.snapshot(); len(tooLong) != 1 || tooLong[0] != 21 {
		t.Fatalf("GameClock tick to reminder minute want [21], got %v", tooLong)
	}
}

func TestPlayerClockTickReschedulesAndReFires(t *testing.T) {
	clock := newTestGameClockFixedAt(0)
	effects := &fakePlayerClockEffects{}
	pc, err := NewPlayerClock(clock, world.New(), effects)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Player joins at minute 0; first reminder due at 0 + 720 = 720.
	pc.Add(newFakeActor(7))

	clock.minutes = 720
	pc.Tick()
	if tooLong, _ := effects.snapshot(); len(tooLong) != 1 || tooLong[0] != 7 {
		t.Fatalf("first fire: want [7], got %v", tooLong)
	}

	clock.minutes += activityReminderMinutes - 1
	pc.Tick()
	if tooLong, _ := effects.snapshot(); len(tooLong) != 1 {
		t.Fatalf("intermediate tick want no extra fire, got %v", tooLong)
	}

	clock.minutes += 1
	pc.Tick()
	if tooLong, _ := effects.snapshot(); len(tooLong) != 2 {
		t.Fatalf("second fire: want [7 7], got %v", tooLong)
	}
}

func TestPlayerClockRemoveStopsReminders(t *testing.T) {
	clock := newTestGameClockFixedAt(0)
	effects := &fakePlayerClockEffects{}
	pc, err := NewPlayerClock(clock, world.New(), effects)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pc.Add(newFakeActor(11))
	pc.Remove(11)

	clock.minutes = activityReminderMinutes
	pc.Tick()
	if tooLong, _ := effects.snapshot(); len(tooLong) != 0 {
		t.Fatalf("removed actor still fired: %v", tooLong)
	}
}

func TestPlayerClockDayNightReappliesShadowSenseOnlyForHolders(t *testing.T) {
	clock := newTestGameClockFixedAt(359)
	state := world.New()
	holder := newFakeActor(1, shadowSenseSkillID)
	nonHolder := newFakeActor(2)
	state.AddPlayer(holder)
	state.AddPlayer(nonHolder)

	effects := &fakePlayerClockEffects{}
	if _, err := NewPlayerClock(clock, state, effects); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	clock.Tick() // 359 → 360, night ⇒ day cross fires OnDayNight listeners.

	tooLong, skilled := effects.snapshot()
	if len(tooLong) != 0 || len(skilled) != 1 {
		t.Fatalf("want 1 skill transition, got tooLong=%v skilled=%v", tooLong, skilled)
	}
	got := skilled[0]
	if got.actorID != 1 || got.night || got.skillID != shadowSenseSkillID || got.level != shadowSenseLevel {
		t.Fatalf("day transition wrong: %+v", got)
	}
	if holder.HasSkill(shadowSenseSkillID) == false {
		t.Fatalf("holder should still know shadow sense after reapply")
	}
	if nonHolder.HasSkill(shadowSenseSkillID) {
		t.Fatalf("non-holder should not have gained shadow sense")
	}
}

func TestPlayerClockDayNightNightBoundarySendsNightMessage(t *testing.T) {
	// Start at the last day-minute (1439); Tick moves to minute 1440 →
	// TimeOfDay 0, which is night, so night ⇒ night fires with night=true.
	clock := newTestGameClockFixedAt(1439)
	state := world.New()
	holder := newFakeActor(7, shadowSenseSkillID)
	state.AddPlayer(holder)

	effects := &fakePlayerClockEffects{}
	if _, err := NewPlayerClock(clock, state, effects); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	clock.Tick()

	_, skilled := effects.snapshot()
	if len(skilled) != 1 || !skilled[0].night || skilled[0].actorID != 7 {
		t.Fatalf("night transition want actor 7 night=true, got %v", skilled)
	}
}

func TestPlayerClockDayNightNoBoundaryDoesNothing(t *testing.T) {
	clock := newTestGameClockFixedAt(1000) // well inside the day
	state := world.New()
	state.AddPlayer(newFakeActor(3, shadowSenseSkillID))

	effects := &fakePlayerClockEffects{}
	if _, err := NewPlayerClock(clock, state, effects); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	clock.Tick()

	if tooLong, skilled := effects.snapshot(); len(tooLong) != 0 || len(skilled) != 0 {
		t.Fatalf("non-boundary tick want empty, got tooLong=%v skilled=%v", tooLong, skilled)
	}
}

func TestPlayerClockDayNightReproduceShadowSkillLevel(t *testing.T) {
	// A holder whose known Shadow Sense level was lifted (unlikely in-game,
	// but the reference re-fetches level 1 unconditionally) is reset to 1
	// after the boundary crossing, mirroring the re-fetch.
	clock := newTestGameClockFixedAt(359)
	state := world.New()
	holder := &fakePlayerClockActor{id: 5, known: map[int]int{shadowSenseSkillID: 9}}
	state.AddPlayer(holder)

	effects := &fakePlayerClockEffects{}
	if _, err := NewPlayerClock(clock, state, effects); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	clock.Tick()

	if level := holder.known[shadowSenseSkillID]; level != shadowSenseLevel {
		t.Fatalf("reapplied level = %d, want %d", level, shadowSenseLevel)
	}
}

// TestPlayerClockConcurrentAddRemoveAndTick exercises the activity map
// under concurrent Add/Remove/Tick to keep -race satisfied.
func TestPlayerClockConcurrentAddRemoveAndTick(t *testing.T) {
	clock := newTestGameClockFixedAt(0)
	effects := &fakePlayerClockEffects{}
	pc, err := NewPlayerClock(clock, world.New(), effects)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			pc.Add(newFakeActor(int32(i)))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			pc.Remove(int32(i))
			pc.Tick()
		}
	}()
	wg.Wait()
}

// TestPlayerClockTickAfterBoundaryTickIsSeparate exercises that a Tick on
// the activity counter (one per GameMinute) does not perturb the day/night
// reapply (driven by GameClock.OnDayNight), even when both fire close
// together in real time.
func TestPlayerClockTickAfterBoundaryTickIsSeparate(t *testing.T) {
	clock := newTestGameClockFixedAt(359)
	state := world.New()
	state.AddPlayer(newFakeActor(13, shadowSenseSkillID))

	// Deliberately schedule the holder's first reminder exactly at the
	// boundary minute so both the day/night reapply and the activity
	// reminder would fire on the same game-minute tick.
	effects := &fakePlayerClockEffects{}
	pc, err := NewPlayerClock(clock, state, effects)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Override the post-Add schedule to dueAt exactly 360.
	pc.mu.Lock()
	pc.activity[13] = 360
	pc.mu.Unlock()

	clock.Tick() // moves clock to 360, dispatching boundary and minute listeners

	tooLong, skilled := effects.snapshot()
	if len(skilled) != 1 || skilled[0].actorID != 13 || skilled[0].night {
		t.Fatalf("boundary reapply want single day transition for 13, got %v", skilled)
	}
	if len(tooLong) != 1 || tooLong[0] != 13 {
		t.Fatalf("activity reminder want [13], got %v", tooLong)
	}
}
