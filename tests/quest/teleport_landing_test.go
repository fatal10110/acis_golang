package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestScriptTeleportAfterTakeWaitsForTheLanding pins the owner-approved
// retry of a script's teleport refused while the player's own teleport is
// still landing (#5121), in the order the boss and charm teleporters use:
// the entry item is taken, then the jump asked. The jump is held, not
// dropped, and made once the client reports the first teleport landed, so
// the item is not lost for nothing.
func TestScriptTeleportAfterTakeWaitsForTheLanding(t *testing.T) {
	t.Parallel()
	w := bootTeleportOrder(t)
	first := script.Loc{X: w.dest.X, Y: w.dest.Y, Z: w.dest.Z}
	second := location.Location{X: w.dest.X + 100, Y: w.dest.Y, Z: w.dest.Z}
	w.srv.RunScript(t, w.player, teleportQuest, func(sc *script.Script, p *script.Player) {
		sc.GiveItems(p, teleportItemID, 1)
		p.TeleportTo(first, 0)
	})
	w.srv.ReadQueued(t, w.srv.Client)
	if !w.char.Teleporting() {
		t.Fatal("the first teleport is not in flight")
	}

	w.srv.RunScript(t, w.player, teleportQuest, func(sc *script.Script, p *script.Player) {
		sc.TakeItems(p, teleportItemID, 1)
		p.TeleportTo(script.Loc{X: second.X, Y: second.Y, Z: second.Z}, 0)
	})
	if frames := w.srv.ReadQueued(t, w.srv.Client); index(frames, serverpackets.OpcodeTeleportToLocation) >= 0 {
		t.Fatalf("second teleport during the first's landing = %x, want it held", testsupport.FrameOpcodes(frames))
	}
	if n := w.char.ItemCount(teleportItemID); n != 0 {
		t.Fatalf("entry item held = %d, want taken", n)
	}

	w.srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if index(frames, serverpackets.OpcodeTeleportToLocation) < 0 {
		t.Fatalf("landing frames = %x, want the held teleport", testsupport.FrameOpcodes(frames))
	}
	w.srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	w.srv.ReadQueued(t, w.srv.Client)
	if x, y, z := w.srv.PlayerPosition(t, w.player); (location.Location{X: x, Y: y, Z: z}) != second {
		t.Fatalf("player at %d,%d,%d, want the held destination %v", x, y, z, second)
	}
}

// index is the position of the first frame of opcode op, -1 for none.
func index(frames [][]byte, op byte) int {
	for i, f := range frames {
		if f[0] == op {
			return i
		}
	}
	return -1
}
