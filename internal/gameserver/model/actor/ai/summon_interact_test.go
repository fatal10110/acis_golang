package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// TestSummonAITryToInteractQueuesWhileSwinging pins PlayableAI.tryToInteract's
// queueing: a summon mid-swing takes the interact as its next intention and
// keeps its attack; the swing ending makes it current.
func TestSummonAITryToInteractQueuesWhileSwinging(t *testing.T) {
	owner := actor(100)
	monster := actor(200)
	object := actor(300)
	strike := &recordingAttack{canAttack: true}
	brain := NewSummon(owner, &summonMove{}, strike)
	brain.TryToAttack(monster, false)
	strike.attackingNow = true

	if !brain.TryToInteract(object, false) {
		t.Fatal("TryToInteract() mid-swing = false, want queued")
	}
	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() mid-swing = %v, want attack", got)
	}
	if kind, _, ok := brain.NextIntention(); !ok || kind != IntentionInteract {
		t.Fatalf("NextIntention() = (%v, %v), want interact", kind, ok)
	}

	strike.attackingNow = false
	brain.FinishedAttack(nil)
	if got := brain.CurrentIntention(); got != IntentionInteract {
		t.Fatalf("CurrentIntention() after the swing = %v, want interact", got)
	}
}

// TestSummonAIThinkInteract pins SummonAI.thinkInteract: a lost object turns
// following the owner on and sends the summon idle; with SHIFT held, an
// object beyond the physical attack range (3D) sends it idle, one within it
// keeps the intention; unshifted, the summon keeps the intention wherever the
// object is. It never walks.
func TestSummonAIThinkInteract(t *testing.T) {
	for _, tc := range []struct {
		name      string
		shift     bool
		dx        int
		lost      bool
		wantIdle  bool
		wantOwner bool
	}{
		{name: "lost", lost: true, wantIdle: true, wantOwner: true},
		{name: "unshifted out of reach", dx: 300},
		{name: "shift out of reach", shift: true, dx: 300, wantIdle: true},
		{name: "shift just out of reach", shift: true, dx: 40, wantIdle: true},
		{name: "shift in reach", shift: true, dx: 39},
	} {
		t.Run(tc.name, func(t *testing.T) {
			summon := actor(100)
			summon.followOff = true
			object := actor(300)
			object.x = tc.dx
			if tc.lost {
				summon.known[object.ObjectID()] = false
			}
			move := &summonMove{}
			brain := NewSummon(summon, move, &recordingAttack{})

			if !brain.TryToInteract(object, tc.shift) {
				t.Fatal("TryToInteract() = false, want accepted")
			}
			want := IntentionInteract
			if tc.wantIdle {
				want = IntentionIdle
			}
			if got := brain.CurrentIntention(); got != want {
				t.Fatalf("CurrentIntention() = %v, want %v", got, want)
			}
			if got := summon.idles > 0; got != tc.wantIdle {
				t.Fatalf("summon went idle = %v, want %v", got, tc.wantIdle)
			}
			if got := !summon.followOff; got != tc.wantOwner {
				t.Fatalf("following the owner = %v, want %v", got, tc.wantOwner)
			}
			if move.followStarted || move.moveTo != (location.Location{}) {
				t.Fatal("summon walked toward an object that is no creature")
			}
		})
	}
}

// TestSummonAITryToInteractRefusedWhileDenied pins the denyAiAction gate.
func TestSummonAITryToInteractRefusedWhileDenied(t *testing.T) {
	summon := actor(100)
	summon.denyAction = true
	brain := NewSummon(summon, &summonMove{}, &recordingAttack{})
	if brain.TryToInteract(actor(300), false) {
		t.Fatal("TryToInteract() while denied = true, want refused")
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle", got)
	}
}
