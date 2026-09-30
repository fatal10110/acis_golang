package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/rs/zerolog"
)

// TestWalkToCastTargetImmobileParksNothing pins the immobile caster's
// answer: out of cast range, a player whose movement is disabled is
// answered ActionFailed alone, the cast is not parked (so no later arrival
// can fire it) and no walk starts.
func TestWalkToCastTargetImmobileParksNothing(t *testing.T) {
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	target := newTestLivePlayer(t, 9, &testsupport.FrameCapture{})
	live.Character.Live.SetImmobilized(true)
	if !live.MovementDisabled() {
		t.Fatal("MovementDisabled() = false after SetImmobilized(true)")
	}
	testsupport.ResetCapture(frames)

	parked := false
	gcl := &GameClientLink{log: zerolog.Nop()}
	if !gcl.walkToCastTarget(live, target.Character, 300, false, func() { parked = true }) {
		t.Fatal("walkToCastTarget() = false for an immobile caster out of range, want the cast held back")
	}
	if parked {
		t.Fatal("immobile caster parked the cast, want nothing parked")
	}
	if live.hasDeferredMagicSkill() || live.hasDeferredItemAICast() {
		t.Fatal("immobile caster left a cast approach parked")
	}
	if live.IsMoving() {
		t.Fatal("immobile caster started a walk")
	}
	if got := testsupport.FrameOpcodes(frames.Frames()); len(got) != 1 || got[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("opcodes = %x, want [ActionFailed]", got)
	}
}
