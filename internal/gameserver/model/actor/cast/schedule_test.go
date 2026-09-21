package cast

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/rs/zerolog"
)

// scalingActor and scalingDef are the exact fixture TestStartScalesTimingAndInstallsReuse
// already verifies against the oracle formula: HitTime 1500ms scales to
// 525ms, InterruptAfter to 325ms, LaunchDelay 125ms, HitDelay 400ms,
// FinalDelay 210ms — comfortably past every threshold the scheduling tests
// below exercise (410ms gauge, interrupt window).
func scalingActor() *testActor {
	return &testActor{mp: 100, hp: 1000, mAtkSpd: 666, pAtkSpd: 333, magicReuseRate: 1.25, initialCost: 7, spiritshot: true}
}

var scalingDef = modelskill.Definition{ID: 10, Level: 2, Magic: true, HitTime: 1500, CoolTime: 600, ReuseDelay: 12000}

func TestScheduleRunsLaunchHitAndFinishInOrder(t *testing.T) {
	clock := &fakeCastClock{}
	actor := scalingActor()
	ctrl := NewController(actor, nil)
	ctrl.afterFunc = clock.AfterFunc
	now := time.Unix(1000, 0)

	def := scalingDef
	plan, err := ctrl.Start(now, testTarget{}, def)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	var order []string
	ctrl.Schedule(plan, Hooks{
		Launch: func() bool { order = append(order, "launch"); return true },
		Hit:    func() { order = append(order, "hit") },
		Finish: func() { order = append(order, "finish") },
	})

	if len(order) != 0 {
		t.Fatalf("hooks fired before any timer, order = %v", order)
	}

	clock.fire(plan.LaunchDelay)
	if got := []string{"launch"}; !equalStrings(order, got) {
		t.Fatalf("order after launch delay = %v, want %v", order, got)
	}

	clock.fire(plan.HitDelay)
	if got := []string{"launch", "hit"}; !equalStrings(order, got) {
		t.Fatalf("order after hit delay = %v, want %v", order, got)
	}
	if !ctrl.CastingNow() {
		t.Fatal("CastingNow() = false between Hit and Finish, want still casting")
	}

	clock.fire(plan.FinalDelay)
	if got := []string{"launch", "hit", "finish"}; !equalStrings(order, got) {
		t.Fatalf("order after final delay = %v, want %v", order, got)
	}
	if ctrl.CastingNow() {
		t.Fatal("CastingNow() = true after Finish, want cleared")
	}
}

func TestScheduleStopsWhenLaunchRejectsTheCast(t *testing.T) {
	clock := &fakeCastClock{}
	actor := &testActor{mp: 100, hp: 100}
	ctrl := NewController(actor, nil)
	ctrl.afterFunc = clock.AfterFunc
	now := time.Unix(1000, 0)

	def := modelskill.Definition{ID: 1, Level: 1, StaticHitTime: true, HitTime: 1000, StaticReuse: true}
	plan, err := ctrl.Start(now, testTarget{}, def)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	hitCalled := false
	ctrl.Schedule(plan, Hooks{
		Launch: func() bool { return false },
		Hit:    func() { hitCalled = true },
	})

	clock.fire(plan.LaunchDelay)

	if hitCalled {
		t.Fatal("Hit hook ran after Launch rejected the cast")
	}
	if ctrl.CastingNow() {
		t.Fatal("CastingNow() = true after a rejected Launch, want stopped")
	}
}

func TestScheduleFailedHitStopsBeforeFinish(t *testing.T) {
	clock := &fakeCastClock{}
	actor := &testActor{mp: 100, hp: 100, hitCost: 50}
	ctrl := NewController(actor, nil)
	ctrl.afterFunc = clock.AfterFunc
	now := time.Unix(1000, 0)

	def := modelskill.Definition{ID: 1, Level: 1, StaticHitTime: true, HitTime: 1000, StaticReuse: true, MPConsume: 50}
	plan, err := ctrl.Start(now, testTarget{}, def)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	actor.mp = 10 // not enough for the 50 MP final cost.

	var failed error
	hitCalled, finishCalled := false, false
	ctrl.Schedule(plan, Hooks{
		Hit:    func() { hitCalled = true },
		Finish: func() { finishCalled = true },
		Failed: func(err error) { failed = err },
	})

	clock.fire(plan.LaunchDelay)
	clock.fire(plan.HitDelay)

	if !errors.Is(failed, ErrNotEnoughMP) {
		t.Fatalf("Failed hook error = %v, want ErrNotEnoughMP", failed)
	}
	if hitCalled {
		t.Fatal("Hit hook ran despite Controller.Hit failing")
	}
	if finishCalled {
		t.Fatal("Finish hook ran after a failed Hit")
	}
	if ctrl.CastingNow() {
		t.Fatal("CastingNow() = true after a failed Hit, want stopped")
	}
}

func TestScheduleCancelsPendingTimersOnStop(t *testing.T) {
	clock := &fakeCastClock{}
	actor := scalingActor()
	ctrl := NewController(actor, nil)
	ctrl.afterFunc = clock.AfterFunc
	now := time.Unix(1000, 0)

	def := scalingDef
	plan, err := ctrl.Start(now, testTarget{}, def)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	hitCalled := false
	ctrl.Schedule(plan, Hooks{Hit: func() { hitCalled = true }})

	clock.fire(plan.LaunchDelay)
	ctrl.Stop()
	clock.fire(plan.HitDelay)

	if hitCalled {
		t.Fatal("Hit hook ran on a timer belonging to a stopped cast")
	}
}

func TestScheduleCancelsPendingTimersOnInterruptOnDamage(t *testing.T) {
	clock := &fakeCastClock{}
	actor := scalingActor()
	ctrl := NewController(actor, nil)
	ctrl.afterFunc = clock.AfterFunc
	now := time.Unix(1000, 0)

	def := scalingDef
	plan, err := ctrl.Start(now, testTarget{}, def)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	hitCalled := false
	ctrl.Schedule(plan, Hooks{Hit: func() { hitCalled = true }})

	if !ctrl.InterruptOnDamage(now.Add(50*time.Millisecond), DamageInterrupt{Damage: 1e9, MEN: 30, Roll: 0}) {
		t.Fatal("InterruptOnDamage() = false inside the interrupt window with a guaranteed break")
	}

	clock.fire(plan.LaunchDelay)
	clock.fire(plan.HitDelay)

	if hitCalled {
		t.Fatal("Hit hook ran after InterruptOnDamage aborted the cast")
	}
}

// TestScheduleStartedAfterInterruptDoesNotFireStaleTimer covers the seq
// guard directly: a timer captured for one cast must not act on a later,
// unrelated cast that reused the same Controller.
func TestScheduleStartedAfterInterruptDoesNotFireStaleTimer(t *testing.T) {
	clock := &fakeCastClock{}
	actor := &testActor{mp: 100, hp: 100, mAtkSpd: 333, pAtkSpd: 333, magicReuseRate: 1, physicalReuseRate: 1}
	ctrl := NewController(actor, nil)
	ctrl.afterFunc = clock.AfterFunc
	now := time.Unix(1000, 0)

	def := modelskill.Definition{ID: 1, Level: 1, Magic: true, StaticHitTime: true, HitTime: 1000, StaticReuse: true}
	plan, err := ctrl.Start(now, testTarget{}, def)
	if err != nil {
		t.Fatalf("first Start() error: %v", err)
	}

	firstHit := false
	ctrl.Schedule(plan, Hooks{Hit: func() { firstHit = true }})
	ctrl.Stop()

	secondPlan, err := ctrl.Start(now, testTarget{}, def)
	if err != nil {
		t.Fatalf("second Start() error: %v", err)
	}
	secondHit := false
	ctrl.Schedule(secondPlan, Hooks{Hit: func() { secondHit = true }})

	// Fire every timer queued so far, including the stale first-cast Launch
	// timer that Stop should have cancelled.
	for _, d := range []time.Duration{plan.LaunchDelay, secondPlan.LaunchDelay, secondPlan.HitDelay} {
		clock.fire(d)
	}

	if firstHit {
		t.Fatal("the superseded cast's Hit hook ran")
	}
	if !secondHit {
		t.Fatal("the current cast's Hit hook did not run")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type fakeCastClock struct {
	timers []*fakeCastTimer
}

func (c *fakeCastClock) AfterFunc(delay time.Duration, f func()) scheduledTimer {
	timer := &fakeCastTimer{delay: delay, f: f}
	c.timers = append(c.timers, timer)
	return timer
}

// fire runs every not-yet-stopped timer registered with the given delay, in
// registration order.
func (c *fakeCastClock) fire(delay time.Duration) {
	for _, timer := range c.timers {
		if timer.delay == delay && !timer.stopped {
			timer.stopped = true
			timer.f()
		}
	}
}

type fakeCastTimer struct {
	delay   time.Duration
	f       func()
	stopped bool
}

func TestScheduleFusionEndsOnAbortOrChannelCompletion(t *testing.T) {
	now := time.Unix(1000, 0)
	ctrl, _, rec := newAbortController()
	plan, err := ctrl.Start(now, testTarget{}, modelskill.Definition{ID: 426, Level: 1, Magic: true, SkillType: "FUSION", HitTime: 15000})
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeCastClock{}
	ctrl.afterFunc = clock.AfterFunc
	ended := 0
	abortsBeforeEnd := -1
	ctrl.ScheduleFusion(plan, time.Second, func() bool { return true }, func() {
		ended++
		abortsBeforeEnd = event.Count[event.CastAborted](rec)
	})
	ctrl.Stop()
	if ended != 1 {
		t.Fatalf("fusion end calls after abort = %d, want 1", ended)
	}
	if got := event.Count[event.CastAborted](rec); abortsBeforeEnd != 0 || got != 1 {
		t.Fatalf("fusion abort order: aborts before end = %d, after stop = %d; want end before abort (0, 1)", abortsBeforeEnd, got)
	}

	plan, err = ctrl.Start(now, testTarget{}, modelskill.Definition{ID: 426, Level: 1, Magic: true, SkillType: "FUSION", HitTime: 15000})
	if err != nil {
		t.Fatal(err)
	}
	ctrl.ScheduleFusion(plan, time.Second, func() bool { return true }, func() { ended++ })
	clock.fire(plan.LaunchDelay)
	if ended != 2 {
		t.Fatalf("fusion end calls after natural completion = %d, want 2", ended)
	}
}

func TestScheduleFusionStopsWhenRecurringCheckFails(t *testing.T) {
	now := time.Unix(1000, 0)
	ctrl, _, _ := newAbortController()
	plan, err := ctrl.Start(now, testTarget{}, modelskill.Definition{ID: 426, Level: 1, Magic: true, SkillType: "FUSION", HitTime: 15000})
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeCastClock{}
	ctrl.afterFunc = clock.AfterFunc
	ended := 0
	ctrl.ScheduleFusion(plan, time.Second, func() bool { return false }, func() { ended++ })
	clock.fire(time.Second)
	if ended != 1 || ctrl.CastingNow() {
		t.Fatalf("failed fusion check = end calls %d, casting %v; want 1 and false", ended, ctrl.CastingNow())
	}
}

func TestScheduleFusionReportsWhenCastAlreadyStopped(t *testing.T) {
	now := time.Unix(1000, 0)
	ctrl, _, _ := newAbortController()
	plan, err := ctrl.Start(now, testTarget{}, modelskill.Definition{ID: 426, Level: 1, Magic: true, SkillType: "FUSION", HitTime: 15000})
	if err != nil {
		t.Fatal(err)
	}
	ctrl.Stop()
	if ctrl.ScheduleFusion(plan, time.Second, func() bool { return true }, nil) {
		t.Fatal("ScheduleFusion() = true after Stop, want false")
	}
}

func (t *fakeCastTimer) Stop() bool {
	if t.stopped {
		return false
	}
	t.stopped = true
	return true
}

// TestScheduleRecoversPanickingHook is the regression test for the panic
// class fixed in dispatch.go's scheduleAfter (network/dispatch_test.go's
// TestScheduleAfterRecoversPanickingCallback): a scheduled cast callback
// (Launch/Hit/Finish) runs on its own goroutine via the real time.AfterFunc
// default branch, outside any per-connection recover, so an unrecovered
// panic there would kill the whole process instead of just this cast.
func TestScheduleRecoversPanickingHook(t *testing.T) {
	buf := &syncCastBuffer{}
	actor := scalingActor()
	ctrl := NewController(actor, nil)
	ctrl.SetLogger(zerolog.New(buf))
	now := time.Unix(1000, 0)

	def := scalingDef
	plan, err := ctrl.Start(now, testTarget{}, def)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	ctrl.Schedule(plan, Hooks{Launch: func() bool { panic("boom") }})

	deadline := time.Now().Add(time.Second)
	for !strings.Contains(buf.String(), "boom") {
		if time.Now().After(deadline) {
			t.Fatalf("panic was not recovered and logged, got: %s", buf.String())
		}
		time.Sleep(time.Millisecond)
	}
}

// syncCastBuffer is a mutex-guarded bytes.Buffer safe for a test's polling
// goroutine to read while a scheduled callback's goroutine writes to it.
type syncCastBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncCastBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncCastBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// TestHoldFinishDefersFinishUntilReleased covers the grant a Hit effect
// takes when its own work has to leave the actor's queue and come back: Hit
// still runs on time, Finish waits for the release, and the release arms it
// even though the final delay already elapsed while the hold stood.
func TestHoldFinishDefersFinishUntilReleased(t *testing.T) {
	clock := &fakeCastClock{}
	ctrl := NewController(scalingActor(), nil)
	ctrl.afterFunc = clock.AfterFunc

	plan, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, scalingDef)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	var order []string
	var release func()
	ctrl.Schedule(plan, Hooks{
		Hit:    func() { order = append(order, "hit"); release = ctrl.HoldFinish() },
		Finish: func() { order = append(order, "finish") },
	})

	clock.fire(plan.LaunchDelay)
	clock.fire(plan.HitDelay)
	if got := []string{"hit"}; !equalStrings(order, got) {
		t.Fatalf("order after hit delay = %v, want %v", order, got)
	}

	clock.fire(plan.FinalDelay)
	if got := []string{"hit"}; !equalStrings(order, got) {
		t.Fatalf("order after final delay while held = %v, want Finish still pending (%v)", order, got)
	}
	if !ctrl.CastingNow() {
		t.Fatal("CastingNow() = false while a Finish hold stands, want still casting")
	}

	release()
	clock.fire(plan.FinalDelay)
	if got := []string{"hit", "finish"}; !equalStrings(order, got) {
		t.Fatalf("order after release = %v, want %v", order, got)
	}
	if ctrl.CastingNow() {
		t.Fatal("CastingNow() = true after the released Finish ran, want cleared")
	}

	// Releasing again must not arm a second Finish for a cast that ended.
	release()
	clock.fire(plan.FinalDelay)
	if got := []string{"hit", "finish"}; !equalStrings(order, got) {
		t.Fatalf("order after a repeated release = %v, want %v", order, got)
	}
}

// TestHoldFinishDroppedWhenCastStops covers the abandoned grant: a cast
// stopped while a hold stands clears it with the rest of its state, and the
// late release neither revives that cast's Finish nor leaks into the next
// cast on the same controller.
func TestHoldFinishDroppedWhenCastStops(t *testing.T) {
	clock := &fakeCastClock{}
	ctrl := NewController(scalingActor(), nil)
	ctrl.afterFunc = clock.AfterFunc

	plan, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, scalingDef)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	var finishes int
	var release func()
	ctrl.Schedule(plan, Hooks{
		Hit:    func() { release = ctrl.HoldFinish() },
		Finish: func() { finishes++ },
	})
	clock.fire(plan.LaunchDelay)
	clock.fire(plan.HitDelay)

	ctrl.Stop()
	if ctrl.CastingNow() {
		t.Fatal("CastingNow() = true after Stop, want cleared")
	}

	release()
	clock.fire(plan.FinalDelay)
	if finishes != 0 {
		t.Fatalf("Finish ran %d times for a stopped cast, want 0", finishes)
	}

	// A fresh cast on the same controller must not inherit the stale hold.
	var order []string
	plan2, err := ctrl.Start(time.Unix(2000, 0), testTarget{}, scalingDef)
	if err != nil {
		t.Fatalf("second Start() error: %v", err)
	}
	ctrl.Schedule(plan2, Hooks{Finish: func() { order = append(order, "finish") }})
	clock.fire(plan2.LaunchDelay)
	clock.fire(plan2.HitDelay)
	clock.fire(plan2.FinalDelay)
	if got := []string{"finish"}; !equalStrings(order, got) {
		t.Fatalf("second cast order = %v, want %v", order, got)
	}
}
