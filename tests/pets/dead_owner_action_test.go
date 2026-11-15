package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestDeadOwnerPetCommandsOnlyFail pins RequestActionUse.java:84-96: a dead
// owner's pet commands (return, attack, stop, follow toggle) and pet skill
// actions are answered ActionFailed and nothing else; the pet stays out and
// unordered.
func TestDeadOwnerPetCommandsOnlyFail(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	wolf, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from the world")
	}
	owner, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("owner %T is not an online character", obj)
	}
	if !owner.MarkDead() {
		t.Fatal("MarkDead reported no change")
	}

	// 15/21 follow toggle, 16/22 attack, 17/23 stop, 19 return, 32 a pet
	// skill action.
	for _, action := range []int32{15, 16, 17, 19, 21, 22, 23, 32} {
		frames := testsupport.SyncBarrierFrames(t, h.client, func() {
			h.client.Send(encodeRequestActionUse(action, false))
			h.client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
		}, serverpackets.OpcodeItemList)
		if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
			ops := make([]byte, len(frames))
			for i, f := range frames {
				ops[i] = f[0]
			}
			t.Fatalf("dead owner action %d answered opcodes %x, want only ActionFailed", action, ops)
		}
		if got, ok := h.srv.State.Summon(h.ownerID); !ok || got != wolf {
			t.Fatalf("dead owner action %d: pet left the world", action)
		}
	}
}
