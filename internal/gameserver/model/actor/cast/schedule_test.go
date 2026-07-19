package cast

import (
	"errors"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
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
	ctrl := NewController(actor)
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
	ctrl := NewController(actor)
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
	ctrl := NewController(actor)
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
	ctrl := NewController(actor)
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
	ctrl := NewController(actor)
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
	ctrl := NewController(actor)
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

func (t *fakeCastTimer) Stop() bool {
	if t.stopped {
		return false
	}
	t.stopped = true
	return true
}
