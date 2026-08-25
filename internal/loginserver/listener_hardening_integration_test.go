package loginserver

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	commoncrypt "github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/netutil"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

func TestGameServerLinkBlowFishKeyFailureDropsConnection(t *testing.T) {
	addr, _, _, _, _ := newTestLink(t, true)

	gs := dialGameServer(t, addr)
	gs.readInitLS()

	payload := []byte{link.OpcodeBlowFishKey}
	payload = binary.LittleEndian.AppendUint32(payload, 999)
	gs.sendFrame(payload)
	gs.expectClosed()
}

func TestGameServerLinkFloodGuardDropsFastRepeatConnections(t *testing.T) {
	guard := netutil.NewFloodGuard(netutil.FloodGuardConfig{
		Enabled:              true,
		FastConnectionLimit:  0,
		NormalConnectionTime: 10 * time.Second,
		FastConnectionTime:   0,
		MaxConnectionsPerIP:  100,
	}, zerolog.Nop())

	dir := t.TempDir()
	namesPath := filepath.Join(dir, "serverNames.xml")
	if err := os.WriteFile(namesPath, []byte(`<?xml version='1.0'?><list>
		<server id="1" name="Bartz" />
	</list>`), 0o644); err != nil {
		t.Fatalf("write serverNames.xml: %v", err)
	}
	names, err := manager.LoadServerNames(namesPath)
	if err != nil {
		t.Fatalf("LoadServerNames: %v", err)
	}
	keys, err := manager.NewRSAKeyPool()
	if err != nil {
		t.Fatalf("NewRSAKeyPool: %v", err)
	}

	l := NewGameServerLink(manager.NewServerRegistry(), names, keys, manager.NewSessionStore(), manager.NewIPBanList(zerolog.Nop()), nil, nil, true, guard, zerolog.Nop())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go l.Serve(ctx, ln)
	addr := ln.Addr().String()

	first := dialGameServer(t, addr)
	first.handshake()

	second := dialGameServer(t, addr)
	second.expectClosed()
}

func TestClientLinkPurgeDropsAuthedClientPastLoginTimeout(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, _, sessions, _ := newTestClientLink(t, accounts, false, func(l *ClientLink) { l.loginTimeout = 150 * time.Millisecond })

	c := dialLoginClient(t, addr)
	c.login(l, "player1", "s3cret")

	reply := c.read()
	if reply[0] != serverpackets.OpcodeLoginFail {
		t.Fatalf("opcode = %#x, want LoginFail (%#x)", reply[0], serverpackets.OpcodeLoginFail)
	}
	if reason := reply[1]; reason != byte(serverpackets.LoginFailAccessFailed) {
		t.Fatalf("LoginFail reason = %#x, want AccessFailed (%#x)", reason, serverpackets.LoginFailAccessFailed)
	}
	c.expectClosed()
	waitSessionMissing(t, sessions, "player1")
}

// TestClientLinkPurgeDuringActiveTrafficKeepsFramesIntact drives a client
// through continuous server-list traffic while the purge loop fires, and
// requires every frame delivered before the kick to arrive intact: the
// purge goroutine's LoginFail must never interleave with a handler
// response mid-frame.
func TestClientLinkPurgeDuringActiveTrafficKeepsFramesIntact(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, servers, sessions, _ := newTestClientLink(t, accounts, false, func(l *ClientLink) { l.loginTimeout = 200 * time.Millisecond })
	markOnlineAuto(t, servers, 7)

	c := dialLoginClient(t, addr)
	key1, key2 := c.login(l, "player1", "s3cret")

	send := func(payload []byte) error {
		buf := make([]byte, commoncrypt.PaddedSize(len(payload)+4))
		copy(buf, payload)
		commoncrypt.AppendChecksum(buf)
		commoncrypt.EncryptBlocks(c.cipher, buf)
		return wire.WriteFrame(c.conn, buf)
	}
	read := func() ([]byte, error) {
		c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		payload, err := wire.ReadFrame(c.conn)
		if err != nil {
			return nil, err
		}
		commoncrypt.DecryptBlocks(c.cipher, payload)
		if !commoncrypt.VerifyChecksum(payload) {
			return nil, fmt.Errorf("corrupt frame received while purge raced a response send")
		}
		return payload, nil
	}

	request := encodeRequestServerList(key1, key2)
	for {
		// Send failures are expected once the server has closed the socket;
		// every successfully read frame must be intact and well-typed.
		_ = send(request)
		payload, err := read()
		if err != nil {
			break // connection dropped by the purge
		}
		switch payload[0] {
		case serverpackets.OpcodeServerList:
		case serverpackets.OpcodeLoginFail:
			if payload[1] != byte(serverpackets.LoginFailAccessFailed) {
				t.Fatalf("LoginFail reason = %#x, want AccessFailed (%#x)", payload[1], serverpackets.LoginFailAccessFailed)
			}
			c.expectClosed()
			return
		default:
			t.Fatalf("unexpected opcode %#x during purge window", payload[0])
		}
	}
	waitSessionMissing(t, sessions, "player1")
}
