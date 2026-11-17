package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// teleportTarget returns the destination of the TeleportToLocation among
// frames, false when there is none.
func teleportTarget(frames [][]byte) (location.Location, bool) {
	f, ok := firstOpcode(frames, serverpackets.OpcodeTeleportToLocation)
	if !ok {
		return location.Location{}, false
	}
	r := wire.NewReader(f[1:])
	r.ReadInt32() // object id
	return location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}, true
}

// TestSignsPriestHuntingGroundTeleport pins "SevenSigns 11 <x> <y> <z>
// <price>" at the Dawn priest, the talker holding 150 ancient adena: the
// price is taken and the talker teleported to the coordinates, no page
// opening; a free ride takes nothing; a talker holding less than the price
// is told it has not enough adena and stays; a command short of a
// coordinate or the price, or naming one that is no number, does nothing.
func TestSignsPriestHuntingGroundTeleport(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		held    int32
		command string
		to      *location.Location
		message int32 // 0 for none
		left    int
	}{
		{"paid", 150, "SevenSigns 11 100 200 -3000 100", &location.Location{X: 100, Y: 200, Z: -3000}, serverpackets.SystemMessageS2S1Disappeared, 50},
		{"free", 150, "SevenSigns 11 500 600 -3200 0", &location.Location{X: 500, Y: 600, Z: -3200}, 0, 150},
		{"too little ancient adena", 50, "SevenSigns 11 300 400 -3100 100", nil, serverpackets.SystemMessageYouNotEnoughAdena, 50},
		{"no price", 150, "SevenSigns 11 700 800 -3300", nil, 0, 150},
		{"coordinate not a number", 150, "SevenSigns 11 700 lots -3300 0", nil, 0, 150},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootPriestWorld(t, map[int32]int32{item.AncientAdenaID: tt.held})
			dawn := w.selectedPriest(t, "DawnPriest", dawnPriestID, 30)
			frames := w.ask(t, dawn, tt.command)

			at, teleported := teleportTarget(frames)
			if teleported != (tt.to != nil) || (tt.to != nil && at != *tt.to) {
				t.Fatalf("teleport %v %v among %x, want %v", at, teleported, opcodes(frames), tt.to)
			}
			got := messages(frames)
			if tt.message == 0 && len(got) != 0 || tt.message != 0 && (len(got) != 1 || got[0] != tt.message) {
				t.Fatalf("messages %v among %x, want %d", got, opcodes(frames), tt.message)
			}
			if _, shown := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); shown {
				t.Fatalf("a page opened: %x", opcodes(frames))
			}
			w.assertHeld(t, map[int32]int{item.AncientAdenaID: tt.left})
		})
	}
}
