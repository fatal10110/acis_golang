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
// ends, notice ahead of UserInfo, and the GM is left vulnerable. Chat, an
// acting frame that fails to decode, and an extended frame that names no
// handler leave the protection on.
func TestAdminInvulWhileSpawnProtected(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithSpawnProtection(time.Minute))
	enterWorld(t, srv.Client)
	gm := onlineCharacter(t, srv, gmID)
	if !gm.SpawnProtected() {
		t.Fatal("GM entered the world without spawn protection")
	}

	// None of these reaches a handler that acts: a build command cut short
	// fails to decode (the one underflow a minute tolerates), an extended
	// frame without its sub-opcode or with an unknown one names no handler,
	// and chat never counts as acting. The chat line is the barrier:
	// exchange's own barrier packet would act.
	unknownExtended := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	unknownExtended.WriteUint16(0xffff)
	chat := testsupport.SyncBarrierFrames(t, srv.Client, func() {
		srv.Client.Send(wire.NewPacketWriter(clientpackets.OpcodeSendBypassBuildCmd).Bytes())
		srv.Client.Send([]byte{clientpackets.OpcodeExtended, 0x01})
		srv.Client.Send(unknownExtended.Bytes())
		srv.Client.Send(encodeSay("hello"))
	}, serverpackets.OpcodeCreatureSay)
	if len(chat) != 0 {
		t.Fatalf("frames that never acted answered %x ahead of the chat line", testsupport.FrameOpcodes(chat))
	}
	if !gm.SpawnProtected() {
		t.Fatal("chat, a malformed frame or an unknown extended frame ended spawn protection")
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
