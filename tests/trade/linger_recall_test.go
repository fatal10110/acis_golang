package trade

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestRecalledLingeringTraderLandsBeforeTradeCancel pins the order a
// teleport of a lingering player follows (Creature.teleportTo, then
// Player.teleportTo's cancels): with no client to report it appeared, the
// player lands inside the teleport, before its open trade is cancelled. A
// trade partner in sight of the landing point gets the player's CharInfo,
// then the trade cancel.
func TestRecalledLingeringTraderLandsBeforeTradeCancel(t *testing.T) {
	t.Parallel()
	data, err := gamexml.LoadAdminData(datapack.Path(t, "data", "xml"))
	if err != nil {
		t.Fatalf("load admin data: %v", err)
	}
	h := bootTraders(t, gameservertest.WithAdmin(data))
	h.enterAll(t)

	// A game master waits a short hop away, in sight of both traders.
	gmID := h.srv.SeedCharacterFor(t, "player3", "Warden", 1, 0).ID
	if _, err := h.srv.DB.ExecContext(context.Background(), "UPDATE characters SET accesslevel = 7 WHERE obj_Id = ?", gmID); err != nil {
		t.Fatalf("set access level: %v", err)
	}
	gm := h.srv.DialClient(t, "player3", 1)
	startInWorld(t, gm)
	gmObj, ok := h.srv.State.Player(gmID)
	if !ok {
		t.Fatal("game master missing from the world")
	}
	gmChar, ok := network.OnlineCharacter(gmObj)
	if !ok {
		t.Fatalf("game master = %T, want an online character", gmObj)
	}
	gmChar.TeleportTo(spawnX+300, spawnY, spawnZ, 0)
	readUntil(t, gm, serverpackets.OpcodeTeleportToLocation)
	gm.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	readUntil(t, gm, serverpackets.OpcodeUserInfo)
	drainAllTraders(t, h.first, h.second, gm)

	h.startTrade(t)
	drainAllTraders(t, h.first, h.second, gm)

	// The first trader drops its connection in combat and lingers, its
	// trade still open.
	h.srv.DropInCombat(t, h.firstID, h.first)
	drainAllTraders(t, h.second, gm)

	gm.Send(encodeTraderBuildCmd("recall TraderOne"))
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
		t.Fatalf("partner frames = %x: want the recalled trader's CharInfo (at %d) before the trade cancel (at %d)",
			testsupport.FrameOpcodes(frames), charInfo, tradeDone)
	}
	if x, _, _ := h.srv.PlayerPosition(t, h.firstID); x != spawnX+300 {
		t.Fatalf("recalled trader at X %d, want the GM's %d", x, spawnX+300)
	}
}

func encodeTraderBuildCmd(command string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSendBypassBuildCmd)
	w.WriteString(command)
	return w.Bytes()
}

// readUntil reads c until a frame with opcode want, failing when none comes.
func readUntil(t *testing.T, c *testsupport.ScriptedClient, want byte) {
	t.Helper()
	for range 100 {
		if c.Read()[0] == want {
			return
		}
	}
	t.Fatalf("no %#x within 100 frames", want)
}

func drainAllTraders(t *testing.T, clients ...*testsupport.ScriptedClient) {
	t.Helper()
	for _, c := range clients {
		drainUntilQuiet(t, c)
	}
}

// drainFramesUntilQuiet collects c's frames until the server goes quiet.
func drainFramesUntilQuiet(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	var frames [][]byte
	for _, f := range c.ReadQueued() {
		frames = append(frames, f)
	}
	return frames
}
