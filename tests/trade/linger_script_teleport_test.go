package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestScriptTeleportedLingeringTraderLandsBeforeTradeCancel pins a
// script's teleport of a lingering player asked from another player's
// queue: the teleport runs on the moved player's own queue, so it follows
// the order of any teleport there (Creature.teleportTo, then
// Player.teleportTo's cancels). With no client to report it appeared, the
// player lands inside the teleport, before its open trade is cancelled: the
// trade partner in sight of the landing point gets the player's CharInfo,
// then the trade cancel.
func TestScriptTeleportedLingeringTraderLandsBeforeTradeCancel(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	h.enterAll(t)
	h.startTrade(t)
	drainAllTraders(t, h.first, h.second)

	h.srv.DropInCombat(t, h.firstID, h.first)
	drainAllTraders(t, h.second)

	obj, ok := h.srv.State.Object(h.firstID)
	if !ok {
		t.Fatal("the lingering trader left the world")
	}
	lingerer := script.PlayerOf(obj.(attackable.Combatant))
	done := make(chan struct{})
	if !h.srv.PlayerQueue(t, h.secondID).Post(func() {
		defer close(done)
		lingerer.TeleportTo(script.Loc{X: spawnX + 300, Y: spawnY, Z: spawnZ}, 0)
	}) {
		t.Fatal("the partner's queue is closed")
	}
	<-done
	h.srv.Settle(t)
	frames := drainFramesUntilQuiet(t, h.second)
	charInfo, tradeDone := -1, -1
	for i, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeCharInfo:
			r := wire.NewReader(f[1:])
			r.ReadInt32() // x
			r.ReadInt32() // y
			r.ReadInt32() // z
			r.ReadInt32() // boat
			if r.ReadInt32() == h.firstID && charInfo < 0 {
				charInfo = i
			}
		case serverpackets.OpcodeSendTradeDone:
			if tradeDone < 0 {
				tradeDone = i
			}
		}
	}
	if charInfo < 0 || tradeDone < 0 || charInfo > tradeDone {
		t.Fatalf("partner frames = %x: want the teleported trader's CharInfo (at %d) before the trade cancel (at %d)",
			testsupport.FrameOpcodes(frames), charInfo, tradeDone)
	}
	if x, _, _ := h.srv.PlayerPosition(t, h.firstID); x != spawnX+300 {
		t.Fatalf("teleported trader at X %d, want %d", x, spawnX+300)
	}
}
