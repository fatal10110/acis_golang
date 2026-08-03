package cast

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// abortActor adds the two optional owner surfaces the abort funnel consults
// to the shared cast test actor.
type abortActor struct {
	*testActor
	allDisabled bool
	enableCalls int
	signetExits int
}

func (a *abortActor) AllSkillsDisabled() bool { return a.allDisabled }

func (a *abortActor) EnableAllSkills() {
	a.enableCalls++
	a.allDisabled = false
}

func (a *abortActor) ExitSignetGround() { a.signetExits++ }

func newAbortController() (*Controller, *abortActor, *int) {
	actor := &abortActor{testActor: scalingActor()}
	ctrl := NewController(actor)
	aborts := new(int)
	ctrl.SetOnAbort(func(bool) { *aborts++ })
	return ctrl, actor, aborts
}

func TestAbortObserverFiresOnlyForAnInFlightCast(t *testing.T) {
	now := time.Unix(1000, 0)

	tests := []struct {
		name       string
		end        func(*Controller)
		start      bool
		wantAborts int
	}{
		{
			name:       "stop in flight",
			start:      true,
			end:        func(c *Controller) { c.Stop() },
			wantAborts: 1,
		},
		{
			name:       "stop while idle",
			end:        func(c *Controller) { c.Stop() },
			wantAborts: 0,
		},
		{
			name:       "natural finish",
			start:      true,
			end:        func(c *Controller) { c.Finish() },
			wantAborts: 0,
		},
		{
			name:       "stop after natural finish",
			start:      true,
			end:        func(c *Controller) { c.Finish(); c.Stop() },
			wantAborts: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl, _, aborts := newAbortController()
			if tt.start {
				if _, err := ctrl.Start(now, testTarget{}, scalingDef); err != nil {
					t.Fatalf("Start() error: %v", err)
				}
			}

			tt.end(ctrl)

			if *aborts != tt.wantAborts {
				t.Fatalf("abort observer fired %d times, want %d", *aborts, tt.wantAborts)
			}
			if ctrl.CastingNow() {
				t.Fatal("CastingNow() = true after the cast ended, want cleared")
			}
		})
	}
}

// TestFinishObserverReportsTheCastThatEnded pins that SetOnFinish's def and
// target are the skill that just ended, not the zero value: the network
// layer's PlayableAI.onEvtFinishedCasting (PlayableAI.java:43-63) port
// gates attack resume on that skill's NextActionIsAttack, so a stale or
// zero def would silently disable or wrongly enable the resume for every
// cast.
func TestFinishObserverReportsTheCastThatEnded(t *testing.T) {
	ctrl, _, _ := newAbortController()
	target := testTarget{}
	var gotDef modelskill.Definition
	var gotTarget any
	ctrl.SetOnFinish(func(_ bool, def modelskill.Definition, tgt any) {
		gotDef, gotTarget = def, tgt
	})
	if _, err := ctrl.Start(time.Unix(1000, 0), target, scalingDef); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	ctrl.Finish()

	if gotDef.ID != scalingDef.ID || gotDef.Level != scalingDef.Level {
		t.Fatalf("finish observer def = %+v, want %+v", gotDef, scalingDef)
	}
	if gotTarget != target {
		t.Fatalf("finish observer target = %v, want %v", gotTarget, target)
	}
}

func TestFinishObserverReportsEveryInFlightCastOnce(t *testing.T) {
	now := time.Unix(1000, 0)

	tests := []struct {
		name  string
		end   func(*Controller)
		start bool
		want  []bool
	}{
		{name: "abort", start: true, end: func(c *Controller) { c.Stop() }, want: []bool{true}},
		{name: "natural finish", start: true, end: func(c *Controller) { c.Finish() }, want: []bool{false}},
		{name: "idle stop", end: func(c *Controller) { c.Stop() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl, _, _ := newAbortController()
			var got []bool
			ctrl.SetOnFinish(func(interrupted bool, _ modelskill.Definition, _ any) { got = append(got, interrupted) })
			if tt.start {
				if _, err := ctrl.Start(now, testTarget{}, scalingDef); err != nil {
					t.Fatalf("Start() error: %v", err)
				}
			}

			tt.end(ctrl)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("finish observer = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStopCancelsPendingPhaseTimers(t *testing.T) {
	clock := &fakeCastClock{}
	ctrl, _, aborts := newAbortController()
	ctrl.afterFunc = clock.AfterFunc

	plan, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, scalingDef)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	var fired []string
	ctrl.Schedule(plan, Hooks{
		Launch: func() bool { fired = append(fired, "launch"); return true },
		Hit:    func() { fired = append(fired, "hit") },
		Finish: func() { fired = append(fired, "finish") },
	})

	ctrl.Stop()
	clock.fire(plan.LaunchDelay)
	clock.fire(plan.HitDelay)
	clock.fire(plan.FinalDelay)

	if len(fired) != 0 {
		t.Fatalf("phase hooks ran after Stop: %v", fired)
	}
	if *aborts != 1 {
		t.Fatalf("abort observer fired %d times, want 1", *aborts)
	}
}

func TestUnaffordableHitReportsBeforeTheAbortFunnel(t *testing.T) {
	clock := &fakeCastClock{}
	ctrl, actor, aborts := newAbortController()
	ctrl.afterFunc = clock.AfterFunc

	actor.hitCost = 20

	plan, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, scalingDef)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	// Drained mid-cast, exactly how a real caster loses the MP it had when
	// the cast started.
	actor.mp = 10

	var order []string
	ctrl.Schedule(plan, Hooks{
		Launch: func() bool { return true },
		Hit:    func() { order = append(order, "hit") },
		Failed: func(err error) {
			if !errors.Is(err, ErrNotEnoughMP) {
				t.Fatalf("Failed hook error = %v, want ErrNotEnoughMP", err)
			}
			if *aborts != 0 {
				t.Fatal("abort observer fired before the failure was reported")
			}
			order = append(order, "failed")
		},
	})

	clock.fire(plan.LaunchDelay)
	clock.fire(plan.HitDelay)

	if len(order) != 1 || order[0] != "failed" {
		t.Fatalf("hook order = %v, want only the failure hook", order)
	}
	if *aborts != 1 {
		t.Fatalf("abort observer fired %d times, want 1", *aborts)
	}
	if ctrl.CastingNow() {
		t.Fatal("CastingNow() = true after an unaffordable hit, want cleared")
	}
	if actor.mp != 10 {
		t.Fatalf("MP = %d, want 10; the unaffordable hit must charge nothing", actor.mp)
	}
}

func TestPlayerActorExitSignetGroundDropsOnlyTheSignetEffect(t *testing.T) {
	ch := &player.Character{ID: 1}
	live, err := creature.NewLive(location.Location{}, 100, permissiveGeo{}, ch)
	if err != nil {
		t.Fatal(err)
	}
	ch.Live = live

	lasting := modelskill.EffectTemplate{Time: 60}
	signet := &effect.Effect{Skill: effect.Skill{ID: 7}, Template: lasting, Type: effect.TypeSignetGround}
	ch.EffectList().Add(&effect.Effect{Skill: effect.Skill{ID: 8}, Template: lasting, Type: effect.TypeBuff})
	ch.EffectList().Add(signet)

	PlayerActor{Character: ch}.ExitSignetGround()

	remaining := ch.EffectList().All()
	if len(remaining) != 1 || remaining[0].Type != effect.TypeBuff {
		t.Fatalf("effects after ExitSignetGround = %+v, want only the buff left", remaining)
	}
}

func TestStopRunsOwnerCleanupEvenWhenIdle(t *testing.T) {
	tests := []struct {
		name        string
		allDisabled bool
		wantEnables int
	}{
		{name: "skills already usable", wantEnables: 0},
		{name: "all skills disabled", allDisabled: true, wantEnables: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl, actor, _ := newAbortController()
			actor.allDisabled = tt.allDisabled

			ctrl.Stop()

			if actor.signetExits != 1 {
				t.Fatalf("ExitSignetGround called %d times, want 1", actor.signetExits)
			}
			if actor.enableCalls != tt.wantEnables {
				t.Fatalf("EnableAllSkills called %d times, want %d", actor.enableCalls, tt.wantEnables)
			}
		})
	}
}
