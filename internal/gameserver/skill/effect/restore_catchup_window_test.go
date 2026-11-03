package effect

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// windowList is catchUpList with its clock returned, so a test can move
// time while a Restore is applying (a long replay, a GC pause, a descheduled
// worker on the wall clock).
func windowList(now time.Time, events *[]string) (*List, *sim.Inline) {
	in := sim.NewInline(now)
	l := NewList(iconEventOwner{eventOwner{events: events}})
	l.SetQueue(in.NewQueue("test"))
	return l, in
}

func otherBuff() []modelskill.EffectTemplate {
	return []modelskill.EffectTemplate{{Name: "Buff", Count: 1, Time: 1200, StackType: "other_buff", StackOrder: 1, Icon: true}}
}

// A selection-anchored poison whose tick comes due while the replay is
// applying, after it began, is left to the effects sweep as well: the
// catch-up stops at the replay's start instant.
func TestRestoreLeavesATickDueAfterTheReplayBeganToTheSweep(t *testing.T) {
	now := time.Unix(5000, 0)
	var events []string
	list, in := windowList(now, &events)
	target := &liveEffectTarget{hp: 100, list: list}

	list.Restore(func() {
		// Restored 995ms before the replay with a 1s period: due 5ms into it.
		ApplyRestored(list, target, target, Skill{ID: 84, Level: 1}, restoredPoison(10, 1), 10, 0, now.Add(-995*time.Millisecond))
		in.AdvanceBefore(10 * time.Millisecond)
		ApplyRestored(list, target, target, Skill{ID: 1040, Level: 1}, otherBuff(), 1, 0, now.Add(-995*time.Millisecond))
	})

	if damageTicks(target.events) != 0 {
		t.Fatalf("damage ticks run by the replay = %d (%v), want 0: the tick came due after the replay began", damageTicks(target.events), target.events)
	}
	list.Tick()
	if damageTicks(target.events) != 1 {
		t.Fatalf("damage ticks after the next sweep = %d (%v), want 1", damageTicks(target.events), target.events)
	}
}

// An in-world replay (a duel's end, a subclass change) stages its effects
// with no restore instant, so each runs from the replay itself: an overdue
// tick is not run inside the replay however long it takes, and the quiet
// catch-up flag is never raised for a character other casters can reach.
func TestInWorldRestoreRunsNoTickEvenWhenTheReplayIsSlow(t *testing.T) {
	now := time.Unix(5000, 0)
	var events []string
	list, in := windowList(now, &events)
	target := &liveEffectTarget{hp: 100, list: list}

	list.Restore(func() {
		ApplyRestored(list, target, target, Skill{ID: 84, Level: 1}, restoredPoison(10, 3), 10, 3, time.Time{})
		in.AdvanceBefore(10 * time.Millisecond)
		ApplyRestored(list, target, target, Skill{ID: 1040, Level: 1}, otherBuff(), 1, 0, time.Time{})
	})

	// No tick ran, so no catch-up tick raised the quiet flag (catchUpTick
	// raises it only around a due tick, whose action is this damage).
	if target.hp != 100 || damageTicks(target.events) != 0 {
		t.Fatalf("in-world replay: HP %g, events %v; want no tick inside the replay", target.hp, target.events)
	}
	for _, e := range list.All() {
		if e.Skill.ID == 84 && !e.dueAt().Equal(now.Add(restoreMinDelay)) {
			t.Fatalf("poison first tick = %v, want %v: it runs from its add in the replay", e.dueAt(), now.Add(restoreMinDelay))
		}
	}
}
