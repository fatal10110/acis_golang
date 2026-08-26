package network

import (
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestRestartLivePlayerWithNoRestartTableSendsActionFailed pins the
// data-missing fallback: when the restart-point table didn't load at all, the
// dead player can't be revived or teleported, and silently answering nothing
// would strand them on the death screen. The reference path always resolves
// at least the nearest town, so this case is a Go-side data-loading gap, not a
// rejection the reference makes — it falls back to ActionFailed so the client
// can dismiss the pending death action and stays dead past the warn in the log.
func TestRestartLivePlayerWithNoRestartTableSendsActionFailed(t *testing.T) {
	state := world.New()
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	state.Spawn(live, 0, 0, 0, 0)
	live.SetHP(1)
	if !live.Die(nil) {
		t.Fatal("Die() = false for a live player with HP left")
	}
	testsupport.ResetCapture(frames)

	gcl := &GameClientLink{world: state, geo: testGeo{}, log: zerolog.Nop()}
	gcl.restartLivePlayer(live, clientpackets.RequestRestartPoint{})

	if got := testsupport.FrameOpcodes(frames.Frames()); len(got) != 1 || got[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("opcodes = %x, want [ActionFailed]", got)
	}
	if !live.Dead() {
		t.Fatal("Dead() = false with no restart destination resolved, want still dead")
	}
}
