package attack

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// TestControllerHitTimerAbortReachesPlayableAI pins the swing a dead main
// target aborts at its hit: a playable's AI hears of it (AttackAborted),
// an NPC's does not, and the swing never finishes.
func TestControllerHitTimerAbortReachesPlayableAI(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(event.Sink) *Controller
		want  int
	}{
		{"player", func(s event.Sink) *Controller {
			return NewPlayer(&timingPlayer{timingActor: timingActor{attackSpeed: 500}}, s)
		}, 1},
		{"summon", func(s event.Sink) *Controller {
			return NewPlayable(&timingSummon{timingActor: timingActor{attackSpeed: 500}}, s)
		}, 1},
		{"npc", func(s event.Sink) *Controller { return NewAttackable(&timingActor{attackSpeed: 500}, s) }, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := &timingTarget{id: 2}
			clock := newTimingClock()
			rec := &event.Recorder{}
			ctrl := tc.build(rec)
			ctrl.SetQueue(clock.q)

			ctrl.DoAttack(target)
			target.dead = true
			clock.fire(time.Second)

			if got := event.Count[event.AttackAborted](rec); got != tc.want {
				t.Fatalf("AttackAborted = %d, want %d", got, tc.want)
			}
			if event.Count[event.HitDealt](rec) != 0 || event.Count[event.AttackFinished](rec) != 0 {
				t.Fatal("an aborted swing reported its hit or finished")
			}
			if ctrl.AttackingNow() {
				t.Fatal("AttackingNow() after the abort = true, want false")
			}
		})
	}
}

// TestControllerDualSecondHitLandsOnAKilledTarget pins a dual swing whose
// first hit kills: only the first group checks the main target, so the
// second hit still reports, refreshes the stance and reaches the target,
// and the swing finishes at 3*attackTime/4 without an abort.
func TestControllerDualSecondHitLandsOnAKilledTarget(t *testing.T) {
	actor := &timingPlayer{timingActor: timingActor{attackType: item.WeaponDual, attackSpeed: 500}}
	target := &timingTarget{id: 2}
	target.onDamage = func() { target.dead = true }
	clock := newTimingClock()
	rec := &event.Recorder{}
	ctrl := NewPlayer(actor, rec)
	ctrl.SetQueue(clock.q)

	// attackTime 1000ms.
	ctrl.DoAttack(target)
	clock.fire(250 * time.Millisecond)
	if !target.dead {
		t.Fatal("first hit did not kill the target")
	}
	clock.fire(500 * time.Millisecond)
	want := []event.HitDealt{{TargetID: 2, Damage: 1}, {TargetID: 2, Damage: 1}}
	if got := event.Of[event.HitDealt](rec); !slices.Equal(got, want) {
		t.Fatalf("HitDealt = %+v, want %+v", got, want)
	}
	if got := event.Count[event.AttackStanceRequested](rec); got != 2 {
		t.Fatalf("AttackStanceRequested = %d, want 2", got)
	}
	if target.hits != 2 {
		t.Fatalf("hits reaching the target = %d, want 2", target.hits)
	}
	clock.fire(749 * time.Millisecond)
	if event.Count[event.AttackFinished](rec) != 0 {
		t.Fatal("swing finished before 3*attackTime/4")
	}
	clock.fire(750 * time.Millisecond)
	if event.Count[event.AttackFinished](rec) != 1 || event.Count[event.AttackAborted](rec) != 0 {
		t.Fatal("swing did not finish at 3*attackTime/4 without an abort")
	}
}

// TestControllerPoleHitLandsOnADeadSplashTarget pins a pole splash target
// that died while the swing was in flight: its hit still reports and
// reaches it.
func TestControllerPoleHitLandsOnADeadSplashTarget(t *testing.T) {
	primary := &timingTarget{id: 2, x: 40, attackable: true}
	secondary := &timingTarget{id: 3, x: 50, attackable: true}
	actor := &timingPlayer{timingActor: timingActor{
		attackType:  item.WeaponPole,
		attackSpeed: 500,
		poleMax:     2,
		known:       []attackable.Combatant{secondary},
	}}
	clock := newTimingClock()
	rec := &event.Recorder{}
	ctrl := NewPlayer(actor, rec)
	ctrl.SetQueue(clock.q)

	ctrl.DoAttack(primary)
	secondary.dead = true
	clock.fire(500 * time.Millisecond)

	want := []event.HitDealt{{TargetID: 2, Damage: 1}, {TargetID: 3, Damage: 1}}
	if got := event.Of[event.HitDealt](rec); !slices.Equal(got, want) {
		t.Fatalf("pole HitDealt = %+v, want %+v", got, want)
	}
	if secondary.hits != 1 {
		t.Fatalf("hits reaching the dead splash target = %d, want 1", secondary.hits)
	}
}
