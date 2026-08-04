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
)

func TestGameClientLinkWaitsForProtocolVersion(t *testing.T) {
	addr, _, _, _ := newTestGameClientLink(t, func() *LoginLink { return nil }, NewSessionValidator())
	c := dialGameClient(t, addr)

	c.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, err := wire.ReadFrame(c.conn); err == nil {
		t.Fatal("server sent data before ProtocolVersion")
	} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		t.Fatalf("read before ProtocolVersion error = %v, want timeout", err)
	}
}

func TestGameClientLinkSendsVersionCheckAfterProtocolVersion(t *testing.T) {
	addr, _, _, _ := newTestGameClientLink(t, func() *LoginLink { return nil }, NewSessionValidator())
	c := dialGameClient(t, addr)
	c.sendProtocolVersion(746)
}

func TestGameClientLinkBadProtocolVersionClosesSilently(t *testing.T) {
	addr, _, _, _ := newTestGameClientLink(t, func() *LoginLink { return nil }, NewSessionValidator())
	c := dialGameClient(t, addr)
	if err := wire.WriteFrame(c.conn, encodeProtocolVersion(1)); err != nil {
		t.Fatalf("write ProtocolVersion: %v", err)
	}
	c.expectClosed()
}

func TestGameClientLinkUnknownOpcodeTolerantAfterAuthDisconnectsPastThreshold(t *testing.T) {
	c, _, _, _ := newLinkedGameClient(t)

	// OpcodeRequestItemList is only valid in-game, not from the char-select
	// (AUTHED) state this client is in: maxUnknownPerMin (5) rejections
	// tolerate, the 6th disconnects.
	for range maxUnknownPerMin {
		c.send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	}
	c.send(encodeRequestPledgeCrest(1))
	reply := c.read()
	if reply[0] != serverpackets.OpcodePledgeCrest {
		t.Fatalf("post-tolerated-unknown opcode = %#x, want PledgeCrest (%#x)", reply[0], serverpackets.OpcodePledgeCrest)
	}

	c.send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	c.expectClosed()
}

func TestGameClientLinkOpcodeBeforeAuthCloses(t *testing.T) {
	addr, _, _, _ := newTestGameClientLink(t, func() *LoginLink { return nil }, NewSessionValidator())
	c := dialGameClient(t, addr)
	c.sendProtocolVersion(746)

	c.send(encodeEnterWorld())
	c.expectClosed()
}

func TestGameClientLinkAuthLoginServerDownFails(t *testing.T) {
	addr, _, _, _ := newTestGameClientLink(t, func() *LoginLink { return nil }, NewSessionValidator())
	c := dialGameClient(t, addr)
	c.sendProtocolVersion(746)

	c.send(encodeAuthLogin("player1", link.SessionKey{}))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeAuthLoginFail {
		t.Fatalf("opcode = %#x, want AuthLoginFail (%#x)", reply[0], serverpackets.OpcodeAuthLoginFail)
	}
	c.expectClosed()
}

func TestGameClientLinkSendTimeCheckIsNoOpInGame(t *testing.T) {
	c, _, _, _ := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeSendTimeCheck(17, 34))
	c.send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	reply := c.read()
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
			c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
			c.read() // CharCreateOk
			c.read() // CharSelectInfo
			c.send(encodeRequestGameStart(0))
			c.read() // SSQInfo
			c.read() // CharSelected
			c.send(encodeEnterWorld())
			readEnterWorldBurst(t, c, false)

			c.send(encodeSingleOpcode(opcode))
			c.send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
			reply := c.read()
			if reply[0] != serverpackets.OpcodeItemList {
				t.Fatalf("post-malformed opcode = %#x, want ItemList (%#x)", reply[0], serverpackets.OpcodeItemList)
			}
		})
	}
}

func TestGameClientLinkMalformedCharacterSelectPacketToleratesFirstDisconnectsOnSecond(t *testing.T) {
	c, _, _, _ := newLinkedGameClient(t)

	c.send(encodeSingleOpcode(clientpackets.OpcodeRequestPledgeCrest))
	c.send(encodeRequestPledgeCrest(1))
	reply := c.read()
	if reply[0] != serverpackets.OpcodePledgeCrest {
		t.Fatalf("post-first-malformed opcode = %#x, want PledgeCrest (%#x)", reply[0], serverpackets.OpcodePledgeCrest)
	}

	c.send(encodeSingleOpcode(clientpackets.OpcodeRequestPledgeCrest))
	c.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, err := c.conn.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("malformed character-select packet read error = %v, want EOF", err)
	}
}
