package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestSelectDoorColorsByLevelUnderSiege pins MyTargetSelected's color on a
// door (Player.java:2480, DoorStatus.getLevel): the selector's level minus
// the door's template level while the door may be attacked, neutral
// otherwise.
func TestSelectDoorColorsByLevelUnderSiege(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		siege bool
		want  uint16
	}{
		{"outside siege", false, 0},
		{"under siege", true, uint16(5 - 2)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tmpl := combatDoorTemplate(door.KindDoor)
			tmpl.Level = 2
			srv, _, gate := bootDoorCombat(t, tmpl, nil)
			if tc.siege {
				srv.BeginDoorSiege(t, gate.DoorID())
			}
			c := srv.Client
			c.Send(encodeAction(gate.ObjectID(), 0, 0, 0, false))
			for range 10 {
				frame := c.Read()
				if frame[0] != serverpackets.OpcodeMyTargetSelected {
					continue
				}
				r := wire.NewReader(frame[1:])
				if id, color := r.ReadInt32(), r.ReadUint16(); id != gate.ObjectID() || color != tc.want {
					t.Fatalf("MyTargetSelected = (%d, %d), want (%d, %d)", id, color, gate.ObjectID(), tc.want)
				}
				drainUntilQuiet(t, c)
				return
			}
			t.Fatal("door was never selected")
		})
	}
}
