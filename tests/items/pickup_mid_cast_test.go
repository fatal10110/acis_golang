package items

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestMidCastPickupRunsAtCastEnd pins PlayableAI.tryToPickUp
// (PlayableAI.java:411-428) for a cast in flight: the click is answered
// ActionFailed only and kept as the next intention, which
// PlayableAI.onEvtFinishedCasting (PlayableAI.java:43-63) runs once the cast
// ends. The item is collected after the cast's MagicSkillLaunched, walked to
// first when out of reach.
func TestMidCastPickupRunsAtCastEnd(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		back int
	}{
		{name: "in reach", back: 0},
		{name: "walked to", back: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithSkills(masteryItemSkills(t)),
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1))
			if !srv.DrivesClock() {
				t.Skip("holding a cast open needs the driven clock")
			}
			c, objID := srv.Client, srv.SoleObjectID(t)
			potion := srv.GiveItem(t, objID, 1060, 5)
			startInWorld(t, c)
			srv.SeedGroundItem(t, objID, item.AdenaID, 40, spawnX+tc.back, spawnY, spawnZ)
			groundID := soleGroundObjectID(t, srv)
			drainUntilQuiet(t, c)

			c.Send(encodeUseItem(potion, false))
			for frame := c.Read(); frame[0] != serverpackets.OpcodeMagicSkillUse; frame = c.Read() {
			}
			c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
			// The cast's own start frames may still be ahead of the answer.
			for frame := c.Read(); frame[0] != serverpackets.OpcodeActionFailed; frame = c.Read() {
				if frame[0] == serverpackets.OpcodeGetItem || frame[0] == serverpackets.OpcodeMagicSkillLaunched {
					t.Fatalf("frame %#x before the mid-cast pickup was answered", frame[0])
				}
			}
			if !srv.PlayerCastingNow(t, objID) {
				t.Fatal("the potion cast ended before the queued pickup was checked")
			}

			launched, walked := false, false
			for i := 0; ; i++ {
				frame := c.ReadWithTimeout(3 * time.Second)
				if frame == nil || i == 100 {
					t.Fatal("the pickup queued behind the cast never ran")
				}
				switch frame[0] {
				case serverpackets.OpcodeMagicSkillLaunched:
					launched = true
				case serverpackets.OpcodeMoveToLocation:
					if !launched {
						t.Fatal("the queued pickup walked before the cast launched")
					}
					walked = true
				}
				if frame[0] == serverpackets.OpcodeGetItem {
					break
				}
			}
			if !launched {
				t.Fatal("the item was collected before the cast launched")
			}
			if walked != (tc.back > 0) {
				t.Fatalf("walked to the item = %v, want %v", walked, tc.back > 0)
			}
			if got := srv.GroundItems.Snapshots(nil); len(got) != 0 {
				t.Fatalf("ground items after the queued pickup = %d, want 0", len(got))
			}
		})
	}
}
