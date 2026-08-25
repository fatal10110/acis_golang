package loginserver

import (
	"context"
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/netutil"
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
