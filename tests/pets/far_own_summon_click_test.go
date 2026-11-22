package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestSelectFarOwnSummon pins the own-summon exception of #4993: an owner
// clicking its own pet outside its known list (its pet window keeps it
// clickable) selects it with no ActionFailed, as it does nearby.
func TestSelectFarOwnSummon(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, nil)
	petActor, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	far := location.Location{X: x + 8000, Y: y, Z: z}
	petActor.TeleportJump(location.Jump{To: far})
	h.srv.AdvanceUntil(t, "the pet landed far away", func() bool {
		px, _, _ := petActor.Position()
		return px == far.X
	})
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeAction(petActor.ObjectID(), 0, 0, 0, false))
	frames := drainFrames(t, h.client)
	selected := false
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeActionFailed:
			t.Fatalf("far own pet click = % x, want no ActionFailed", opcodes(frames))
		case serverpackets.OpcodeMyTargetSelected:
			selected = wire.NewReader(f[1:]).ReadInt32() == petActor.ObjectID()
		}
	}
	if !selected {
		t.Fatalf("far own pet click = % x, want MyTargetSelected(pet)", opcodes(frames))
	}
}
