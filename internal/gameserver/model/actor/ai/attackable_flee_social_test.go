package ai

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// fleeMove records the walks MoveToLocation is asked for.
type fleeMove struct {
	recordingMove
	walks []location.Location
}

func (m *fleeMove) MoveToLocation(dest location.Location) (bool, error) {
	m.walks = append(m.walks, dest)
	return true, nil
}

// fleeSocialAI is an AI past its first periodic cycle on a clock the test
// moves by hand.
func fleeSocialAI(t *testing.T) (*Attackable, *fakeActor, *fleeMove, *recordingAttack, *time.Time) {
	t.Helper()
	owner := actor(1)
	move := &fleeMove{}
	strike := &recordingAttack{canAttack: true}
	brain := NewAttackable(owner, move, strike)
	now := time.Unix(1_000, 0)
	brain.now = func() time.Time { return now }
	brain.lifeTime.Store(1)
	return brain, owner, move, strike, &now
}

func runAI(t *testing.T, brain *Attackable) {
	t.Helper()
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
}

func tickThink(t *testing.T, brain *Attackable) {
	t.Helper()
	if err := brain.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
}

// TestSocialDesirePlaysAndHoldsSelection pins a promoted social desire: its
// desire leaves the queue, the actor stops and plays the animation once, and
// no desire is taken up until the timer has run out, the last millisecond
// included.
func TestSocialDesirePlaysAndHoldsSelection(t *testing.T) {
	brain, owner, move, strike, now := fleeSocialAI(t)

	brain.AddSocialDesire(3, 2000, 50)
	runAI(t, brain)
	if !slices.Equal(owner.socials, []int{3}) {
		t.Fatalf("social animations = %v, want [3]", owner.socials)
	}
	if move.stopCount != 1 {
		t.Fatalf("movement stops = %d, want 1", move.stopCount)
	}
	if got := brain.Desires().Len(); got != 0 {
		t.Fatalf("queued desires after the social = %d, want 0", got)
	}
	if got := brain.CurrentIntention(); got != IntentionSocial {
		t.Fatalf("CurrentIntention() = %v, want social", got)
	}

	target := actor(2)
	addAttackHate(brain, target, 0, 1000)
	*now = now.Add(1999 * time.Millisecond)
	runAI(t, brain)
	tickThink(t, brain)
	if got := brain.CurrentIntention(); got != IntentionSocial {
		t.Fatalf("CurrentIntention() inside the social timer = %v, want social", got)
	}
	if strike.doAttackCalls != 0 || move.followCalls != 0 {
		t.Fatalf("attack steps inside the social timer: swings %d, chases %d", strike.doAttackCalls, move.followCalls)
	}
	if len(owner.socials) != 1 {
		t.Fatalf("social animations = %v, want the one only", owner.socials)
	}

	*now = now.Add(time.Millisecond)
	runAI(t, brain)
	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() once the social timer ran out = %v, want attack", got)
	}
	if strike.doAttackCalls != 1 {
		t.Fatalf("swings once the social timer ran out = %d, want 1", strike.doAttackCalls)
	}
}

// TestSocialHoldKeepsTheIdles pins the idles under a social timer: the event
// idle waits, and the periodic cycle's idle aborts and opens the no-desire
// point but leaves the social current. Once the timer runs out the event
// idle runs.
func TestSocialHoldKeepsTheIdles(t *testing.T) {
	brain, owner, move, _, now := fleeSocialAI(t)

	brain.AddSocialDesire(1, 1000, 50)
	runAI(t, brain)
	stops := move.stopCount

	runAI(t, brain)
	if move.stopCount != stops || len(owner.hooks.points) != 0 {
		t.Fatalf("event pass under the social timer: stops %d -> %d, hook points %v; want no idle", stops, move.stopCount, owner.hooks.points)
	}

	tickThink(t, brain)
	if move.stopCount != stops+1 || !slices.Equal(owner.hooks.points, []HookPoint{HookSeeCreature, HookNoDesire}) {
		t.Fatalf("periodic pass under the social timer: stops %d -> %d, hook points %v; want one idle", stops, move.stopCount, owner.hooks.points)
	}
	if got := brain.CurrentIntention(); got != IntentionSocial {
		t.Fatalf("CurrentIntention() after the periodic idle = %v, want social", got)
	}

	*now = now.Add(time.Second)
	runAI(t, brain)
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() once the social timer ran out = %v, want idle", got)
	}
}

func TestSocialDesireEqualityAndSleep(t *testing.T) {
	brain, owner, _, _, _ := fleeSocialAI(t)

	brain.AddSocialDesire(2, 1000, 10)
	brain.AddSocialDesire(2, 5000, 5)
	brain.AddSocialDesire(3, 1000, 1)
	got := brain.Desires().Snapshot()
	if len(got) != 2 || got[0].ItemObjectID != 2 || got[0].Weight != 15 || got[0].Timer != 1000 || got[1].ItemObjectID != 3 {
		t.Fatalf("queued socials = %+v, want id 2 (weight 15, timer 1000) then id 3", got)
	}

	brain.Desires().Clear()
	owner.sleeping = true
	brain.AddSocialDesire(2, 1000, 10)
	if n := brain.Desires().Len(); n != 0 {
		t.Fatalf("queued desires while the AI sleeps = %d, want 0", n)
	}
}

// TestFleeDesireRunsAndHoldsSelection pins a promoted flee: the actor runs
// distance straight away from its target, and while the flee is current
// and still queued no heavier desire replaces it and no step repeats. The
// arrival drops the flee, and the next pass takes the heavier desire.
func TestFleeDesireRunsAndHoldsSelection(t *testing.T) {
	brain, owner, move, strike, _ := fleeSocialAI(t)
	target := actor(2)
	target.x, target.y = 100, 50

	brain.AddFleeDesire(target, 300, 100)
	runAI(t, brain)
	if want := (location.Location{}).FleeFrom(100, 50, 300); !slices.Equal(move.walks, []location.Location{want}) {
		t.Fatalf("flee walks = %v, want [%v]", move.walks, want)
	}
	if owner.runStanceCalls != 1 {
		t.Fatalf("run stance switches = %d, want 1", owner.runStanceCalls)
	}

	addAttackHate(brain, target, 0, 1_000_000)
	runAI(t, brain)
	tickThink(t, brain)
	if got := brain.CurrentIntention(); got != IntentionFlee {
		t.Fatalf("CurrentIntention() with a heavier attack queued = %v, want flee", got)
	}
	if len(move.walks) != 1 || strike.doAttackCalls != 0 || move.followCalls != 0 {
		t.Fatalf("steps while fleeing: walks %d, swings %d, chases %d; want the one walk", len(move.walks), strike.doAttackCalls, move.followCalls)
	}

	brain.Arrived()
	if !slices.Contains(owner.hooks.points, HookMoveFinished) {
		t.Fatalf("hook points on the flee arrival = %v, want move finished", owner.hooks.points)
	}
	runAI(t, brain)
	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() after the flee arrived = %v, want attack", got)
	}
}

// TestFleeDesireFoldsOnItsTarget pins the flee's identity: a second flee from
// the same target adds its weight and keeps the first one's start and
// distance.
func TestFleeDesireFoldsOnItsTarget(t *testing.T) {
	brain, owner, _, _, _ := fleeSocialAI(t)
	target := actor(2)

	brain.AddFleeDesire(target, 300, 10)
	owner.x = 50
	brain.AddFleeDesire(target, 500, 5)
	got := brain.Desires().Snapshot()
	if len(got) != 1 || got[0].Weight != 15 || got[0].Distance != 300 || got[0].Location != (location.Location{}) {
		t.Fatalf("queued flees = %+v, want one of weight 15, distance 300, from the origin", got)
	}
}

func TestFleeDesireRefusals(t *testing.T) {
	brain, owner, _, _, _ := fleeSocialAI(t)

	brain.AddFleeDesire(nil, 300, 10)
	owner.immobile = true
	brain.AddFleeDesire(actor(2), 300, 10)
	if n := brain.Desires().Len(); n != 0 {
		t.Fatalf("queued desires = %d, want a nil target and an immobile actor refused", n)
	}
}

// TestFleeStepRunsOnlyWithinItsDistance pins the flee step's own gates: it
// does not walk once the actor stands its distance or more from where the
// flee was queued, for a distance under 10, or for an actor that cannot move
// by the time the flee is taken up.
func TestFleeStepRunsOnlyWithinItsDistance(t *testing.T) {
	tests := []struct {
		name     string
		distance int
		movedTo  int
		immobile bool
		walks    int
	}{
		{name: "short of the distance", distance: 300, movedTo: 299, walks: 1},
		{name: "at the distance", distance: 300, movedTo: 300},
		{name: "distance under 10", distance: 9},
		{name: "cannot move", distance: 300, immobile: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			brain, owner, move, _, _ := fleeSocialAI(t)
			target := actor(2)
			target.x, target.y = -100, -50

			brain.AddFleeDesire(target, tc.distance, 10)
			owner.x = tc.movedTo
			owner.immobile = tc.immobile
			runAI(t, brain)
			if got := brain.CurrentIntention(); got != IntentionFlee {
				t.Fatalf("CurrentIntention() = %v, want flee", got)
			}
			if len(move.walks) != tc.walks {
				t.Fatalf("flee walks = %v, want %d", move.walks, tc.walks)
			}
		})
	}
}

// TestFleeDesireDroppedForLostTarget pins the flee's prune: a flee from a
// creature the actor no longer knows, or one that is dead, leaves the queue
// before a desire is chosen, and with it the hold.
func TestFleeDesireDroppedForLostTarget(t *testing.T) {
	for _, dead := range []bool{false, true} {
		brain, owner, _, _, _ := fleeSocialAI(t)
		target := actor(2)
		target.x, target.y = 100, 50
		brain.AddFleeDesire(target, 300, 10)
		runAI(t, brain)

		if dead {
			target.alikeDead = true
		} else {
			owner.known[target.ObjectID()] = false
		}
		runAI(t, brain)
		if n := brain.Desires().Len(); n != 0 {
			t.Fatalf("dead=%v: queued desires = %d, want the flee dropped", dead, n)
		}
		if got := brain.CurrentIntention(); got != IntentionIdle {
			t.Fatalf("dead=%v: CurrentIntention() = %v, want idle", dead, got)
		}
	}
}

// TestFleeAfterArrivalIsTakenUp pins the hold's identity: once a flee has
// arrived, a new flee from the same target, queued from where the actor now
// stands, is not mistaken for the arrived one, so it is taken up and runs.
func TestFleeAfterArrivalIsTakenUp(t *testing.T) {
	brain, owner, move, _, _ := fleeSocialAI(t)
	target := actor(2)
	target.x, target.y = 100, 50

	brain.AddFleeDesire(target, 300, 10)
	runAI(t, brain)
	owner.x, owner.y = -200, -100
	brain.Arrived()

	brain.AddFleeDesire(target, 300, 10)
	runAI(t, brain)
	if len(move.walks) != 2 {
		t.Fatalf("flee walks = %v, want the second flee to run too", move.walks)
	}
	if want := (location.Location{X: -200, Y: -100}).FleeFrom(100, 50, 300); move.walks[1] != want {
		t.Fatalf("second flee walk = %v, want %v", move.walks[1], want)
	}
}

// TestFleeSocialDesiresQueueConcurrently queues flees and socials from many
// goroutines while desire selection runs. Whichever is taken up first holds
// the selection, so at most one social plays and at most one flee runs.
func TestFleeSocialDesiresQueueConcurrently(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	target.x, target.y = 100, 50
	move := &fleeMove{}
	brain := NewAttackable(owner, move, &recordingAttack{})

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			brain.AddFleeDesire(target, 300, 1)
			brain.AddSocialDesire(1, 60_000, 1)
			_ = brain.RunAI()
		})
	}
	wg.Wait()

	brain.mu.Lock()
	defer brain.mu.Unlock()
	if len(owner.socials) > 1 || len(move.walks) > 1 || len(owner.socials)+len(move.walks) == 0 {
		t.Fatalf("socials %v, flee walks %v: want exactly one of them taken up, once", owner.socials, move.walks)
	}
}
