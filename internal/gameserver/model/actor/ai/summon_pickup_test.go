package ai

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// pickupMove is a summon move controller recording straight walks.
type pickupMove struct {
	summonMove
	straight []location.Location
}

func (m *pickupMove) MoveStraightToLocation(at location.Location) (bool, error) {
	m.straight = append(m.straight, at)
	return true, nil
}

// recordingPickUp places ground items and records what the AI asks of them.
type recordingPickUp struct {
	items map[int32]location.Location
	idles int
	taken []int32
}

func (p *recordingPickUp) PickUpTarget(id int32) (location.Location, bool) {
	at, ok := p.items[id]
	return at, ok
}
func (p *recordingPickUp) Idle()           { p.idles++ }
func (p *recordingPickUp) PickUp(id int32) { p.taken = append(p.taken, id) }

func TestSummonPickUpWalksToTheItemThenTakesIt(t *testing.T) {
	pet := actor(100)
	move := &pickupMove{}
	item := location.Location{X: 100}
	pickup := &recordingPickUp{items: map[int32]location.Location{7: item}}
	brain := NewSummon(pet, move, &recordingAttack{})
	brain.SetPickUp(pickup)

	if !brain.TryToPickUp(7) {
		t.Fatal("TryToPickUp() = false, want accepted")
	}
	if !slices.Equal(move.straight, []location.Location{item}) || len(pickup.taken) != 0 {
		t.Fatalf("out of reach: walks %v, taken %v; want one straight walk to %v and nothing taken", move.straight, pickup.taken, item)
	}
	if got := brain.CurrentIntention(); got != IntentionPickUp {
		t.Fatalf("CurrentIntention() while walking = %v, want %v", got, IntentionPickUp)
	}

	pet.x = 100 - PickUpRange + 1
	brain.Think()
	if !slices.Equal(pickup.taken, []int32{7}) || pickup.idles != 1 {
		t.Fatalf("within reach: taken %v, idles %d; want item 7 taken after one idle", pickup.taken, pickup.idles)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after the pickup = %v, want idle", got)
	}
}

func TestSummonPickUpWaitsOutTheSwing(t *testing.T) {
	attack := &recordingAttack{attackingNow: true}
	pickup := &recordingPickUp{items: map[int32]location.Location{7: {}}}
	brain := NewSummon(actor(100), &pickupMove{}, attack)
	brain.SetPickUp(pickup)

	if !brain.TryToPickUp(7) {
		t.Fatal("TryToPickUp() mid-swing = false, want queued")
	}
	if kind, _, ok := brain.NextIntention(); !ok || kind != IntentionPickUp || len(pickup.taken) != 0 {
		t.Fatalf("mid-swing: next = %v (%v), taken %v; want the pickup queued", kind, ok, pickup.taken)
	}

	attack.attackingNow = false
	brain.FinishedAttack(nil)
	if !slices.Equal(pickup.taken, []int32{7}) {
		t.Fatalf("after the swing: taken %v, want item 7", pickup.taken)
	}
}

func TestSummonPickUpOfAGoneItemGoesIdle(t *testing.T) {
	move := &pickupMove{}
	pickup := &recordingPickUp{}
	brain := NewSummon(actor(100), move, &recordingAttack{})
	brain.SetPickUp(pickup)

	brain.TryToPickUp(7)
	if len(move.straight) != 0 || len(pickup.taken) != 0 || pickup.idles != 1 {
		t.Fatalf("gone item: walks %v, taken %v, idles %d; want only one idle", move.straight, pickup.taken, pickup.idles)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle", got)
	}
}

func TestSummonPickUpWithoutWiringIsRefused(t *testing.T) {
	brain := NewSummon(actor(100), &pickupMove{}, &recordingAttack{})
	if brain.TryToPickUp(7) {
		t.Fatal("TryToPickUp() with no pick-up wiring = true, want refused")
	}
}
