package social

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestTellToLingeringPlayerNotFound whispers to a player whose connection
// dropped in combat and who still lingers in the world: the reference
// answers TARGET_IS_NOT_FOUND_IN_THE_GAME for a detached client
// (ChatTell.java:26) and delivers nothing.
func TestTellToLingeringPlayerNotFound(t *testing.T) {
	t.Parallel()
	p := bootPair(t)
	p.enterAll(t)

	p.srv.DropInCombat(t, p.bobbyID, p.bobby)
	p.alice.Send(encodeTell("psst", "Bobby"))
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageTargetNotFound)
	if _, ok := p.srv.State.Player(p.bobbyID); !ok {
		t.Fatal("Bobby left the world before the whisper was refused")
	}
}
