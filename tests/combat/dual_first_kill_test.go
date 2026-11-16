package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestDualSwingWhoseFirstHitKills pins a dual swing whose first hit kills
// its target (CreatureAttack.onHitTimer DUAL branch: only the first doHit
// follows the main-target check, the second is gated by the swing alone):
// the second hit's feedback still reaches the player at attackTime/2, the
// swing finishes at 3*attackTime/4, where the attack intention's think finds
// the target lost and goes idle with ActionFailed, and the emote asked for
// next is broadcast.
func TestDualSwingWhoseFirstHitKills(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		roll  int
		crits int
	}{
		{"critical", 0, 1},
		{"plain", 99, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithItemTemplates(dualSwordTemplates(t)),
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			if !srv.DrivesClock() {
				t.Skip("timing a swing's landings needs the driven clock")
			}
			c, objID := srv.Client, srv.SoleObjectID(t)
			dual := srv.GiveItem(t, objID, dualSwordID, 1)
			startInWorld(t, c)
			c.Send(encodeUseItem(dual, false))
			drainUntilQuiet(t, c)

			hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 30, Y: hostileY, Z: hostileZ})
			drainUntilQuiet(t, c)
			onPlayerQueue(t, srv, objID, func(pc *player.Character) {
				pc.SetRollSource(func(int) int { return tc.roll })
			})
			// Java halves the int attack time twice, each rounding down.
			step := time.Duration(playerAttackMillis(t, srv, objID)/2/2) * time.Millisecond

			targetHostile(t, c, hostile.ObjectID())
			hostile.SetCurrentHP(1)
			drainUntilQuiet(t, c)
			c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
			readUntilOwnAttack(t, c, objID, "the dual swing")
			drainUntilQuiet(t, c)

			for i, want := range []struct {
				at   string
				seen swingTally
			}{
				{"attackTime/4", swingTally{damage: 1, crits: tc.crits}},
				{"attackTime/2", swingTally{damage: 1, crits: tc.crits}},
				{"3*attackTime/4", swingTally{actionFailed: 1}},
			} {
				srv.Advance(t, step)
				if got := tallySwing(t, srv, c, objID); got != want.seen {
					t.Fatalf("step %d, at %s: %+v, want %+v", i+1, want.at, got, want.seen)
				}
				if i == 0 && !hostile.Dead() {
					t.Fatal("the first hit left the monster standing")
				}
			}

			c.Send(encodeRequestSocialAction(2))
			assertEmoteShown(t, c, objID, 2, "emote after the swing")
		})
	}
}
