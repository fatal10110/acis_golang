package admin

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const actedNotice = "As you acted, you are no longer under spawn protection."

// TestAdminInvulWhileSpawnProtected pins where an acting packet ends spawn
// protection (L2GameClientPacket.run: runImpl, then onActionRequest): after
// its handler. A spawn-protected GM with its flag off reads invulnerable, so
// //invul turns the flag off and says "invulnerable"; the protection then
// ends, notice ahead of UserInfo, and the GM is left vulnerable. Chat and a
// frame that fails to decode leave the protection on.
func TestAdminInvulWhileSpawnProtected(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithSpawnProtection(time.Minute))
	enterWorld(t, srv.Client)
	gm := onlineCharacter(t, srv, gmID)
	if !gm.SpawnProtected() {
		t.Fatal("GM entered the world without spawn protection")
	}

	// Neither a Say2 cut short, which never reaches a handler, nor a Say2
	// counts as acting. The chat line is the barrier: exchange's own barrier
	// packet would act.
	chat := testsupport.SyncBarrierFrames(t, srv.Client, func() {
		srv.Client.Send(wire.NewPacketWriter(clientpackets.OpcodeSay2).Bytes())
		srv.Client.Send(encodeSay("hello"))
	}, serverpackets.OpcodeCreatureSay)
	if len(chat) != 0 {
		t.Fatalf("malformed chat and chat answered %x ahead of the chat line", testsupport.FrameOpcodes(chat))
	}
	if !gm.SpawnProtected() {
		t.Fatal("chat or a malformed frame ended spawn protection")
	}

	frames := exchange(t, srv.Client, encodeBuildCmd("invul"))
	if got := testsupport.FrameOpcodes(frames); !slices.Equal(got, []byte{
		serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeUserInfo,
	}) {
		t.Fatalf("//invul frames = %x, want two messages then UserInfo", got)
	}
	assertTexts(t, frames[:2], "You are now invulnerable.", actedNotice)
	if gm.SpawnProtected() {
		t.Fatal("//invul left spawn protection on")
	}
	if gm.Invul() {
		t.Fatal("//invul under spawn protection left the GM invulnerable once protection ended")
	}
}
