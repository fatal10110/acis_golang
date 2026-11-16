package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestSummonAIThinkIdleExitsFollowOwner pins the idle exits of the attack and
// cast thinks: a summon that can no longer act on its intention goes idle,
// which for a summon following its owner means following the owner again,
// and for one with follow mode off means stopping where it is. Reverting any
// exit to a plain idle fails the follows-owner cases.
func TestSummonAIThinkIdleExitsFollowOwner(t *testing.T) {
	ref := skill.Ref{ID: 4139, Level: 8}
	exits := []struct {
		name string
		// start makes the attack or cast current; exit then breaks it.
		start func(t *testing.T, brain *Summon)
		exit  func(self *fakeActor, brain *Summon, cast *recordingCast)
	}{
		{
			name: "attack think denied",
			start: func(t *testing.T, brain *Summon) {
				if !brain.TryToAttack(actor(200), false) {
					t.Fatal("TryToAttack() = false, want accepted attack")
				}
			},
			exit: func(self *fakeActor, _ *Summon, _ *recordingCast) { self.denyAction = true },
		},
		{
			name: "cast think denied",
			start: func(t *testing.T, brain *Summon) {
				if !brain.TryToCast(actor(200), ref, false, false) {
					t.Fatal("TryToCast() = false, want accepted cast")
				}
			},
			exit: func(self *fakeActor, _ *Summon, _ *recordingCast) { self.denyAction = true },
		},
		{
			name: "cast disabled",
			start: func(t *testing.T, brain *Summon) {
				if !brain.TryToCast(actor(200), ref, false, false) {
					t.Fatal("TryToCast() = false, want accepted cast")
				}
			},
			exit: func(_ *fakeActor, _ *Summon, cast *recordingCast) { cast.disabled = true },
		},
		{
			name: "no cast controller",
			start: func(t *testing.T, brain *Summon) {
				if !brain.TryToCast(actor(200), ref, false, false) {
					t.Fatal("TryToCast() = false, want accepted cast")
				}
			},
			exit: func(_ *fakeActor, brain *Summon, _ *recordingCast) { brain.SetCastController(nil) },
		},
	}

	for _, tc := range exits {
		for _, followOff := range []bool{false, true} {
			name := tc.name + "/follows owner"
			if followOff {
				name = tc.name + "/follow off"
			}
			t.Run(name, func(t *testing.T) {
				self := actor(100)
				self.owner = actor(1)
				self.followOff = followOff
				move := &summonMove{recordingMove: recordingMove{followStarted: true}}
				cast := &recordingCast{canAttempt: true, canCast: true, castRange: 400}
				brain := NewSummon(self, move, &recordingAttack{canAttack: true})
				brain.SetCastController(cast)

				tc.start(t, brain)
				if got := brain.CurrentIntention(); got != IntentionAttack && got != IntentionCast {
					t.Fatalf("CurrentIntention() = %v before the exit, want attack or cast", got)
				}
				tc.exit(self, brain, cast)
				idles, stops := self.idles, move.stopCount
				brain.Think()

				if self.idles != idles+1 {
					t.Fatalf("GoneIdle calls = %d, want %d", self.idles, idles+1)
				}
				if followOff {
					if got := brain.CurrentIntention(); got != IntentionIdle {
						t.Fatalf("CurrentIntention() = %v, want idle", got)
					}
					if move.stopCount != stops+1 {
						t.Fatalf("Stop calls = %d, want %d", move.stopCount, stops+1)
					}
					if move.friendlyTarget != nil {
						t.Fatalf("friendly follow target = %v, want none", move.friendlyTarget)
					}
					return
				}
				if got := brain.CurrentIntention(); got != IntentionFollow {
					t.Fatalf("CurrentIntention() = %v, want follow", got)
				}
				if self.denyAction {
					// A denied follow think starts no walk; the follow
					// intention on the owner waits for the summon to act.
					if move.friendlyTarget != nil {
						t.Fatalf("friendly follow target = %v while denied, want none", move.friendlyTarget)
					}
					self.denyAction = false
					brain.Think()
				}
				if move.friendlyTarget != self.owner {
					t.Fatalf("friendly follow target = %v, want the owner", move.friendlyTarget)
				}
			})
		}
	}
}
