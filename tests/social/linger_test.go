package social

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// dropInCombat closes objID's connection while it is in combat: the
// reference keeps such a player in the world 15 s before cleanMe removes
// it, its client already marked detached (GameClient.onDisconnection,
// GameClient.java:201-213). It returns once the server noticed the drop,
// the player still in the world.
func (p *pair) dropInCombat(t *testing.T, objID int32, close func() error) {
	t.Helper()
	p.srv.SetPlayerInCombat(t, objID, true)
	if err := close(); err != nil {
		t.Fatal(err)
	}
	obj, ok := p.srv.State.Player(objID)
	if !ok {
		t.Fatal("the dropped player left the world at once")
	}
	for deadline := time.Now().Add(5 * time.Second); !network.ClientDetached(obj); {
		if time.Now().After(deadline) {
			t.Fatal("the server never noticed the connection drop")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestTellToLingeringPlayerNotFound whispers to a player whose connection
// dropped in combat and who still lingers in the world: the reference
// answers TARGET_IS_NOT_FOUND_IN_THE_GAME for a detached client
// (ChatTell.java:26) and delivers nothing.
func TestTellToLingeringPlayerNotFound(t *testing.T) {
	p := bootPair(t)
	p.enterAll(t)

	p.dropInCombat(t, p.bobbyID, p.bobby.Close)
	p.alice.Send(encodeTell("psst", "Bobby"))
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageTargetNotFound)
	if _, ok := p.srv.State.Player(p.bobbyID); !ok {
		t.Fatal("Bobby left the world before the whisper was refused")
	}
}
