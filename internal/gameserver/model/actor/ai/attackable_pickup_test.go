package ai

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func (a *fakeActor) PickUpTarget(itemObjectID int32) (location.Location, bool) {
	at, ok := a.ground[itemObjectID]
	return at, ok
}

func (a *fakeActor) PickUp(itemObjectID int32) bool {
	if _, ok := a.ground[itemObjectID]; !ok {
		return false
	}
	a.picked = append(a.picked, itemObjectID)
	if a.onPickUp != nil {
		a.onPickUp()
	}
	delete(a.ground, itemObjectID)
	return true
}

// walkingMove is a recordingMove that records each straight walk to a
// point, the pick-up's walk with path-finding off; a routed walk is not
// recorded.
type walkingMove struct {
	recordingMove
	walks []location.Location
}

func (m *walkingMove) MoveStraightToLocation(at location.Location) (bool, error) {
	m.walks = append(m.walks, at)
	return true, nil
}

// pickUpWorld is an actor at the origin with one ground item, 9001, 100 away
// on the x axis.
func pickUpWorld() (*fakeActor, *walkingMove, *Attackable) {
	owner := actor(1)
	owner.ground = map[int32]location.Location{9001: {X: 100}}
	move := &walkingMove{}
	return owner, move, NewAttackable(owner, move, &recordingAttack{})
}

// TestAttackablePickUpWalksThenPicks pins the pick-up step: out of range the
// actor walks to the item; strictly within 36 it picks the item up, with the
// AI loop unlocked, and the desire leaves the queue while the intention
// stays current.
func TestAttackablePickUpWalksThenPicks(t *testing.T) {
	owner, move, brain := pickUpWorld()
	brain.AddPickUpDesire(9001, 1_000_000)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionPickUp {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionPickUp)
	}
	if want := []location.Location{{X: 100}}; !slices.Equal(move.walks, want) {
		t.Fatalf("walks = %v, want %v", move.walks, want)
	}
	if len(owner.picked) != 0 {
		t.Fatalf("picked %v from 100 away", owner.picked)
	}

	// 36 away is not yet within reach: the step walks again.
	owner.x = 64
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if len(move.walks) != 2 || len(owner.picked) != 0 {
		t.Fatalf("at 36 away: walks %v, picked %v; want a second walk and no pickup", move.walks, owner.picked)
	}

	owner.x = 65
	var during Intention
	owner.onPickUp = func() { during = brain.CurrentIntention() }
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if !slices.Equal(owner.picked, []int32{9001}) {
		t.Fatalf("picked = %v, want [9001]", owner.picked)
	}
	if during != IntentionPickUp {
		t.Fatalf("intention seen during the pickup = %v, want %v", during, IntentionPickUp)
	}
	if brain.Desires().Len() != 0 {
		t.Fatalf("desires after the pickup = %d, want none", brain.Desires().Len())
	}
	if got := brain.CurrentIntention(); got != IntentionPickUp {
		t.Fatalf("CurrentIntention() after the pickup = %v, want %v kept", got, IntentionPickUp)
	}
	if len(move.walks) != 2 {
		t.Fatalf("walks = %v, want no walk in reach", move.walks)
	}
}

// TestAttackablePickUpOfGoneItemStaysQueued: a desire whose item the actor
// can no longer pick up does nothing and stays queued, outweighing lighter
// desires.
func TestAttackablePickUpOfGoneItemStaysQueued(t *testing.T) {
	owner, move, brain := pickUpWorld()
	delete(owner.ground, 9001)
	brain.AddPickUpDesire(9001, 1_000_000)
	brain.AddWanderDesire(5, 5)
	for range 3 {
		if err := brain.RunAI(); err != nil {
			t.Fatalf("RunAI() error: %v", err)
		}
	}
	if got := brain.CurrentIntention(); got != IntentionPickUp {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionPickUp)
	}
	if len(move.walks) != 0 || len(owner.picked) != 0 {
		t.Fatalf("walks %v, picked %v; want neither", move.walks, owner.picked)
	}
	if !brain.Desires().Has(&Desire{Kind: IntentionPickUp, ItemObjectID: 9001}) {
		t.Fatal("the pick-up desire left the queue")
	}
}

// TestAttackablePickUpGates: object id 0 queues nothing, an actor that
// cannot act takes no step, and one that cannot move does not walk.
func TestAttackablePickUpGates(t *testing.T) {
	_, _, brain := pickUpWorld()
	brain.AddPickUpDesire(0, 1_000_000)
	if brain.Desires().Len() != 0 {
		t.Fatal("a pick-up of object 0 was queued")
	}

	owner, move, brain := pickUpWorld()
	owner.denyAction = true
	brain.AddPickUpDesire(9001, 1)
	if err := brain.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	owner.denyAction = false
	owner.immobile = true
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if len(move.walks) != 0 || len(owner.picked) != 0 {
		t.Fatalf("walks %v, picked %v; want neither", move.walks, owner.picked)
	}
}

// TestAttackablePickUpRanksByWeight: a pick-up desire is taken up against
// the other kinds by weight, and an equal one adds its weight.
func TestAttackablePickUpRanksByWeight(t *testing.T) {
	_, move, brain := pickUpWorld()
	brain.AddMoveToDesire(location.Location{X: -500}, 100)
	brain.AddPickUpDesire(9001, 60)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionMoveTo {
		t.Fatalf("CurrentIntention() = %v, want the heavier %v", got, IntentionMoveTo)
	}
	brain.AddPickUpDesire(9001, 60)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionPickUp {
		t.Fatalf("CurrentIntention() = %v, want %v at weight 120", got, IntentionPickUp)
	}
	if want := []location.Location{{X: 100}}; !slices.Equal(move.walks, want) {
		t.Fatalf("walks = %v, want %v", move.walks, want)
	}
}
