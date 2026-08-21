package network

import (
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func TestGameClientLinkWaitsForProtocolVersion(t *testing.T) {
	addr, _, _, _ := newTestGameClientLink(t, func() *LoginLink { return nil }, NewSessionValidator())
	c := testsupport.Dial(t, addr)

	c.Conn().SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, err := wire.ReadFrame(c.Conn()); err == nil {
		t.Fatal("server sent data before ProtocolVersion")
	} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		t.Fatalf("read before ProtocolVersion error = %v, want timeout", err)
	}
}

func TestGameClientLinkSendsVersionCheckAfterProtocolVersion(t *testing.T) {
	addr, _, _, _ := newTestGameClientLink(t, func() *LoginLink { return nil }, NewSessionValidator())
	c := testsupport.Dial(t, addr)
	c.SendProtocolVersion(746)
}

func TestGameClientLinkBadProtocolVersionClosesSilently(t *testing.T) {
	addr, _, _, _ := newTestGameClientLink(t, func() *LoginLink { return nil }, NewSessionValidator())
	c := testsupport.Dial(t, addr)
	if err := wire.WriteFrame(c.Conn(), encodeProtocolVersion(1)); err != nil {
		t.Fatalf("write ProtocolVersion: %v", err)
	}
	c.ExpectClosed()
}

func TestGameClientLinkUnknownOpcodeTolerantAfterAuthDisconnectsPastThreshold(t *testing.T) {
	c, _, _, _ := newLinkedGameClient(t)

	// OpcodeRequestItemList is only valid in-game, not from the char-select
	// (AUTHED) state this client is in: maxUnknownPerMin (5) rejections
	// tolerate, the 6th disconnects.
	for range maxUnknownPerMin {
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	}
	c.Send(encodeRequestPledgeCrest(1))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodePledgeCrest {
		t.Fatalf("post-tolerated-unknown opcode = %#x, want PledgeCrest (%#x)", reply[0], serverpackets.OpcodePledgeCrest)
	}

	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	c.ExpectClosed()
}

func TestGameClientLinkUnknownExtendedOpcodeTolerantDisconnectsPastThreshold(t *testing.T) {
	c, _, _, _ := newLinkedGameClient(t)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	// An unmapped extended second-opcode counts toward the same sliding-60s
	// maxUnknownPerMin threshold as a top-level unknown opcode: the first
	// maxUnknownPerMin are tolerated, the next one disconnects.
	for range maxUnknownPerMin {
		c.Send(encodeUnknownExtendedOpcode())
	}
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeItemList {
		t.Fatalf("post-tolerated-unknown-extended opcode = %#x, want ItemList (%#x)", reply[0], serverpackets.OpcodeItemList)
	}

	c.Send(encodeUnknownExtendedOpcode())
	if reply := c.Read(); reply[0] != serverpackets.OpcodeActionFailed {
		// detachLivePlayer's Stop() now reaches the cast controller
		// (Player.cleanup -> abortAll(true) -> _cast.stop(),
		// Creature.java:1298-1302), and PlayerCast.stop() sends
		// clientActionFailed unconditionally, cast or no cast in flight
		// (PlayerCast.java:382-387).
		t.Fatalf("pre-close opcode = %#x, want ActionFailed from detach's unconditional cast-stop ack (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}
	c.ExpectClosed()
}

func TestGameClientLinkOpcodeBeforeAuthCloses(t *testing.T) {
	addr, _, _, _ := newTestGameClientLink(t, func() *LoginLink { return nil }, NewSessionValidator())
	c := testsupport.Dial(t, addr)
	c.SendProtocolVersion(746)

	c.Send(encodeEnterWorld())
	c.ExpectClosed()
}

func TestGameClientLinkAuthLoginServerDownFails(t *testing.T) {
	addr, _, _, _ := newTestGameClientLink(t, func() *LoginLink { return nil }, NewSessionValidator())
	c := testsupport.Dial(t, addr)
	c.SendProtocolVersion(746)

	c.Send(encodeAuthLogin("player1", link.SessionKey{}))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeAuthLoginFail {
		t.Fatalf("opcode = %#x, want AuthLoginFail (%#x)", reply[0], serverpackets.OpcodeAuthLoginFail)
	}
	c.ExpectClosed()
}

func TestGameClientLinkSendTimeCheckIsNoOpInGame(t *testing.T) {
	c, _, _, _ := newLinkedGameClient(t)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.Send(encodeSendTimeCheck(17, 34))
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeItemList {
		t.Fatalf("post-SendTimeCheck opcode = %#x, want ItemList (%#x)", reply[0], serverpackets.OpcodeItemList)
	}
}

func TestGameClientLinkMalformedLivePacketsDoNotDisconnect(t *testing.T) {
	for _, opcode := range []byte{
		clientpackets.OpcodeUseItem,
		clientpackets.OpcodeAction,
		clientpackets.OpcodeSendTimeCheck,
	} {
		t.Run(fmt.Sprintf("opcode_%02x", opcode), func(t *testing.T) {
			c, _, _, _ := newLinkedGameClient(t)
			c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
			c.Read() // CharCreateOk
			c.Read() // CharSelectInfo
			c.Send(encodeRequestGameStart(0))
			c.Read() // SSQInfo
			c.Read() // CharSelected
			c.Send(encodeEnterWorld())
			readEnterWorldBurst(t, c, false)

			c.Send(encodeSingleOpcode(opcode))
			c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
			reply := c.Read()
			if reply[0] != serverpackets.OpcodeItemList {
				t.Fatalf("post-malformed opcode = %#x, want ItemList (%#x)", reply[0], serverpackets.OpcodeItemList)
			}
		})
	}
}

func TestGameClientLinkMalformedCharacterSelectPacketToleratesFirstDisconnectsOnSecond(t *testing.T) {
	c, _, _, _ := newLinkedGameClient(t)

	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestPledgeCrest))
	c.Send(encodeRequestPledgeCrest(1))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodePledgeCrest {
		t.Fatalf("post-first-malformed opcode = %#x, want PledgeCrest (%#x)", reply[0], serverpackets.OpcodePledgeCrest)
	}

	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestPledgeCrest))
	c.Conn().SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, err := c.Conn().Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("malformed character-select packet read error = %v, want EOF", err)
	}
}
