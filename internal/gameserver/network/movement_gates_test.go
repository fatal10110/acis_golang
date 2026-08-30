package network

import (
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestMoveLivePlayerRejectsBeyond9900Units pins MoveBackwardToLocation.java:
// 109-114: a target farther than 9900 units from the packet's own origin is
// rejected with ActionFailed instead of starting a walk.
func TestMoveLivePlayerRejectsBeyond9900Units(t *testing.T) {
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	testsupport.ResetCapture(frames)

	gcl := &GameClientLink{log: zerolog.Nop()}
	origin := location.Location{X: 0, Y: 0, Z: 0}
	target := location.Location{X: 10000, Y: 0, Z: 0} // 10000 > 9900 cap
	gcl.moveLivePlayer(live, target, origin)

	if got := testsupport.FrameOpcodes(frames.Frames()); len(got) != 1 || got[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("opcodes = %x, want [ActionFailed]", got)
	}
}

// TestMoveLivePlayerCancelsEnchantBefore9900Reject pins that cancel runs
// after the out-of-control / zero-speed gates but before the 9900-distance
// cap: a too-far click still closes the enchant window, then ActionFailed.
func TestMoveLivePlayerCancelsEnchantBefore9900Reject(t *testing.T) {
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	gcl := &GameClientLink{log: zerolog.Nop()}
	if !gcl.enchantStateStore().Select(live.ObjectID(), 600) {
		t.Fatal("Select returned false")
	}
	testsupport.ResetCapture(frames)

	origin := location.Location{X: 0, Y: 0, Z: 0}
	target := location.Location{X: 10000, Y: 0, Z: 0}
	gcl.moveLivePlayer(live, target, origin)

	got := frames.Frames()
	if opcodes := testsupport.FrameOpcodes(got); len(opcodes) != 3 ||
		opcodes[0] != serverpackets.OpcodeEnchantResult ||
		opcodes[1] != serverpackets.OpcodeSystemMessage ||
		opcodes[2] != serverpackets.OpcodeActionFailed {
		t.Fatalf("opcodes = %x, want [EnchantResult, SystemMessage, ActionFailed]", opcodes)
	}
	assertEnchantResultFrame(t, got[0], serverpackets.EnchantResultCancelled)
	assertSystemMessageIDFrame(t, got[1], serverpackets.SystemMessageEnchantScrollCancelled)
	if got := gcl.enchantStateStore().Active(live.ObjectID()); got != 0 {
		t.Fatalf("active scroll after too-far cancel = %d, want 0", got)
	}
}

// TestMoveLivePlayerRejectsZeroMoveSpeed pins MoveBackwardToLocation.java:
// 82-87: a fully overloaded player (weight-penalty band 4,
// PlayerStatus.getMoveSpeed() == 0) rejects the walk with both ActionFailed
// and CANT_MOVE_TOO_ENCUMBERED, distinct from the arrow-key (MoveMovement
// == 0) rejection the caller already handles.
func TestMoveLivePlayerRejectsZeroMoveSpeed(t *testing.T) {
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)

	live.Character.SetWeightLimitMultiplier(1)
	live.Character.Inventory().AddNew(9500, 100000, 999) // Heavy Ingot, weight 10 each
	live.Character.Inventory().UpdateWeight()
	live.Character.RefreshWeightPenalty()
	if got := live.Character.WeightPenalty(); got != 4 {
		t.Fatalf("WeightPenalty() = %d, want 4 (fully overloaded)", got)
	}
	testsupport.ResetCapture(frames)

	gcl := &GameClientLink{log: zerolog.Nop()}
	target := location.Location{X: 100, Y: 0, Z: 0}
	origin := location.Location{X: 0, Y: 0, Z: 0}
	gcl.moveLivePlayer(live, target, origin)

	if got := testsupport.FrameOpcodes(frames.Frames()); len(got) != 2 ||
		got[0] != serverpackets.OpcodeActionFailed || got[1] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcodes = %x, want [ActionFailed, SystemMessage]", got)
	}
}

// TestMoveLivePlayerZeroSpeedKeepsActiveEnchant pins that the zero-speed
// gate returns before cancelActiveEnchant: an overloaded player keeps the
// selection and never receives EnchantResult.
func TestMoveLivePlayerZeroSpeedKeepsActiveEnchant(t *testing.T) {
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)

	live.Character.SetWeightLimitMultiplier(1)
	live.Character.Inventory().AddNew(9500, 100000, 999) // Heavy Ingot, weight 10 each
	live.Character.Inventory().UpdateWeight()
	live.Character.RefreshWeightPenalty()
	if got := live.Character.WeightPenalty(); got != 4 {
		t.Fatalf("WeightPenalty() = %d, want 4 (fully overloaded)", got)
	}

	gcl := &GameClientLink{log: zerolog.Nop()}
	if !gcl.enchantStateStore().Select(live.ObjectID(), 600) {
		t.Fatal("Select returned false")
	}
	testsupport.ResetCapture(frames)

	target := location.Location{X: 100, Y: 0, Z: 0}
	origin := location.Location{X: 0, Y: 0, Z: 0}
	gcl.moveLivePlayer(live, target, origin)

	if got := testsupport.FrameOpcodes(frames.Frames()); len(got) != 2 ||
		got[0] != serverpackets.OpcodeActionFailed || got[1] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcodes = %x, want [ActionFailed, SystemMessage]", got)
	}
	if got := gcl.enchantStateStore().Active(live.ObjectID()); got != 600 {
		t.Fatalf("active scroll after zero-speed reject = %d, want 600", got)
	}
}
