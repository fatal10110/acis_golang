package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// assertNoGM requires the /gmlist answer that no game master is online:
// NO_GM_PROVIDING_SERVICE_NOW, then its sound.
func assertNoGM(t *testing.T, frames [][]byte) {
	t.Helper()
	if len(frames) != 2 {
		t.Fatalf("/gmlist frames = %x, want SystemMessage then PlaySound", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageNoGMProvidingServiceNow)
	if frames[1][0] != serverpackets.OpcodePlaySound {
		t.Fatalf("frame 1 opcode = %#x, want PlaySound", frames[1][0])
	}
	r := wire.NewReader(frames[1][1:])
	if typ, file := r.ReadInt32(), r.ReadString(); typ != 0 || file != "systemmsg_e.702" {
		t.Fatalf("PlaySound = %d %q, want 0 systemmsg_e.702", typ, file)
	}
}

// assertGMs requires the /gmlist answer naming names, in order.
func assertGMs(t *testing.T, frames [][]byte, names ...string) {
	t.Helper()
	if len(frames) != 1+len(names) {
		t.Fatalf("/gmlist frames = %x, want GM_LIST then %d name(s)", testsupport.FrameOpcodes(frames), len(names))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageGMList)
	for i, name := range names {
		id, text := systemText(t, frames[1+i])
		if id != serverpackets.SystemMessageGMS1 || text != name {
			t.Fatalf("/gmlist entry %d = %d %q, want GM_S1 %q", i, id, text, name)
		}
	}
}

// TestGMList pins the online game-master roster (AdminData.java's GM list,
// EnterWorld.java:79-91, RequestGmList.java): a GM logs in listed when its
// level may use //gmlist, /gmlist names the listed GMs to everyone and the
// hidden ones too to a GM, //gmlist toggles the GM off and back on, and a
// GM that leaves the world leaves the list.
func TestGMList(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	user, _ := addPlayer(t, srv, "player2", "Player", userLevel)
	assertNoGM(t, exchange(t, user, encodeGmList()))

	enterWorld(t, gm)
	drain(t, user)
	assertGMs(t, exchange(t, user, encodeGmList()), "Admin")
	assertGMs(t, exchange(t, gm, encodeGmList()), "Admin")

	assertTexts(t, exchange(t, gm, encodeBuildCmd("gmlist")), "Removed from GMList.")
	assertNoGM(t, exchange(t, user, encodeGmList()))
	assertGMs(t, exchange(t, gm, encodeGmList()), "Admin (invis)")

	assertTexts(t, exchange(t, gm, encodeBuildCmd("gmlist")), "Registered into GMList.")
	assertGMs(t, exchange(t, user, encodeGmList()), "Admin")

	logout(t, gm)
	drain(t, user)
	assertNoGM(t, exchange(t, user, encodeGmList()))
}

// TestGMListUnlistedAtLogin pins GMStartupAutoList = False: a GM logs in on
// the list, but hidden from every player that is not a GM.
func TestGMListUnlistedAtLogin(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithGMStartupUnlisted())
	enterWorld(t, srv.Client)
	user, _ := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, srv.Client)
	assertNoGM(t, exchange(t, user, encodeGmList()))
	assertGMs(t, exchange(t, srv.Client, encodeGmList()), "Admin (invis)")
}

// logout leaves the world and waits for the server to close the
// connection, which it does once the character has been detached.
func logout(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeLogout).Bytes())
	for range 100 {
		if c.Read()[0] == serverpackets.OpcodeLeaveWorld {
			c.ExpectClosed()
			return
		}
	}
	t.Fatal("no LeaveWorld within 100 frames")
}
