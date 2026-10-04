package character

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// msgLeftCombatZone is SystemMessageId.LEFT_COMBAT_ZONE.
const msgLeftCombatZone = 284

// TestWalkOffBattlefieldFlagsBeforeCompass pins the order of a walk off a
// battlefield under siege: the battlefield's exit says the player left the
// combat zone and flags it (UserInfo), and only then does the zone
// revalidation send the new compass code. The walk revalidates on the
// player's own queue, so a flag deferred to that queue would reach the
// client after the compass. Onto peace ground the compass path flags
// nothing of its own, so the battlefield exit is the only flag.
func TestWalkOffBattlefieldFlagsBeforeCompass(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		peace   bool
		compass byte
	}{
		{name: "onto peace ground", peace: true, compass: 0x0c},
		{name: "onto general ground", compass: 0x0f},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			battlefield, err := zone.NewCuboid(-100, 100, -100, 100, -10_000, 10_000)
			if err != nil {
				t.Fatal(err)
			}
			zones := zone.NewIndex()
			siege, err := zone.NewSiege(1, battlefield, commons.NewStatSet())
			if err != nil {
				t.Fatal(err)
			}
			siege.SetActive(true)
			zones.Add(siege)
			if tc.peace {
				town, err := zone.NewCuboid(100, 500, -100, 100, -10_000, 10_000)
				if err != nil {
					t.Fatal(err)
				}
				zones.Add(zone.NewPeace(2, town))
			}
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 1, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithZones(zones),
			)
			c := srv.Client
			c.Send(encodeRequestGameStart(0))
			c.Read()
			c.Read()
			c.Send(encodeEnterWorld())
			assertCompassCodes(t, readEnterWorldBurst(t, c), 0x0b)
			drainQuiet(t, c)
			objID := srv.SoleObjectID(t)
			obj, ok := srv.State.Player(objID)
			if !ok {
				t.Fatal("player missing from world state")
			}
			character, ok := network.OnlineCharacter(obj)
			if !ok {
				t.Fatal("player has no live character")
			}
			if character.PvPFlagState() != 0 {
				t.Fatal("player flagged before leaving the battlefield")
			}

			spawn := location.Location{X: 10, Y: 20, Z: 30}
			target := location.Location{X: 300, Y: 20, Z: 30}
			c.Send(encodeMoveBackwardToLocation(target, spawn, 1))
			if frame := c.Read(); frame[0] != serverpackets.OpcodeMoveToLocation {
				t.Fatalf("walk opcode = %#x, want MoveToLocation", frame[0])
			}
			waitForWorldPosition(t, srv, objID, target)
			frames := readUntilQuiet(c)
			assertCompassCodes(t, frames, tc.compass)
			left, userInfo, compass := -1, -1, -1
			for i, f := range frames {
				switch {
				case left < 0 && len(f) >= 5 && f[0] == serverpackets.OpcodeSystemMessage &&
					binary.LittleEndian.Uint32(f[1:5]) == msgLeftCombatZone:
					left = i
				case userInfo < 0 && f[0] == serverpackets.OpcodeUserInfo:
					userInfo = i
				case compass < 0 && len(f) >= 3 && bytes.Equal(f[:3], []byte{0xfe, 0x32, 0}):
					compass = i
				}
			}
			if left < 0 || userInfo <= left || compass <= userInfo {
				t.Fatalf("left-combat-zone index = %d, UserInfo index = %d, compass index = %d; want 284, then UserInfo, then compass", left, userInfo, compass)
			}
			if character.PvPFlagState() == 0 {
				t.Fatal("leaving the battlefield did not flag the player")
			}
		})
	}
}
