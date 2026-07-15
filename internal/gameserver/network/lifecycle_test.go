package network

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func TestGameClientLinkNormalDisconnectLogsDebug(t *testing.T) {
	logs := &safeLogBuffer{}
	logger := zerolog.New(logs).Level(zerolog.DebugLevel)
	addr, _, _, _ := newTestGameClientLinkWithLog(t, func() *LoginLink { return nil }, NewSessionValidator(), logger)
	c := dialGameClient(t, addr)
	c.sendProtocolVersion(746)

	if err := c.conn.Close(); err != nil {
		t.Fatalf("close client conn: %v", err)
	}
	got := waitForLog(t, logs, `"message":"Read frame"`)
	if strings.Contains(got, `"level":"error"`) {
		t.Fatalf("normal disconnect logged as error: %s", got)
	}
	if !strings.Contains(got, `"level":"debug"`) {
		t.Fatalf("normal disconnect log level = %s, want debug", got)
	}
}

func TestDetachLivePlayerSavesWithUncancelledBoundedContext(t *testing.T) {
	chars := newFakeCharStore()
	items := newFakeItemStore()
	roster := gamemanager.NewRoster(chars, items, nil, testTemplates(t), testItemTemplates(), npc.NewTable(nil), &sequentialIDs{next: 100}, gamemanager.DefaultDeleteAfter, time.Now)
	live := newTestLivePlayer(t, 101, &frameCapture{})
	savedAt := location.Location{X: 46160, Y: 41237, Z: -3534}
	live.Character.Location = savedAt
	live.Character.Heading = 32768
	if err := chars.Create(context.Background(), live.Character); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	parent, cancel := context.WithCancel(context.Background())
	cancel()
	gcl := &GameClientLink{roster: roster, log: zerolog.Nop()}
	gcl.detachLivePlayer(parent, live)

	pos := chars.savedPosition(t, live.ObjectID())
	if pos.ctxErr != nil {
		t.Fatalf("save context error = %v, want nil despite canceled parent", pos.ctxErr)
	}
	if !pos.hasDeadline {
		t.Fatal("save context has no deadline")
	}
	if ttl := time.Until(pos.deadline); ttl <= 0 || ttl > 3*time.Second {
		t.Fatalf("save context deadline in %s, want a short future timeout", ttl)
	}
	if pos.location != savedAt || pos.heading != 32768 {
		t.Fatalf("saved position = %+v/%d, want %+v/32768", pos.location, pos.heading, savedAt)
	}
}

func TestGameClientLinkLogoutLeavesWorld(t *testing.T) {
	c, chars, _, state := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	savedAt := location.Location{X: 46160, Y: 41237, Z: -3534}
	c.send(encodeValidatePosition(savedAt, 32768))
	c.send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeLeaveWorld {
		t.Fatalf("logout opcode = %#x, want LeaveWorld (%#x)", reply[0], serverpackets.OpcodeLeaveWorld)
	}
	c.expectClosed()
	if _, ok := state.Player(objID); ok {
		t.Fatalf("world.Player(%d) still present after logout", objID)
	}
	pos := chars.savedPosition(t, objID)
	if pos.location != savedAt || pos.heading != 32768 {
		t.Fatalf("saved position after logout = %+v/%d, want %+v/32768", pos.location, pos.heading, savedAt)
	}
}

func TestGameClientLinkRestartReturnsToCharacterSelect(t *testing.T) {
	c, chars, _, state := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	savedAt := location.Location{X: 46160, Y: 41237, Z: -3534}
	c.send(encodeValidatePosition(savedAt, 32768))
	c.send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeRestartResponse {
		t.Fatalf("restart opcode = %#x, want RestartResponse (%#x)", reply[0], serverpackets.OpcodeRestartResponse)
	}
	if ok := wire.NewReader(reply[1:]).ReadInt32(); ok != 1 {
		t.Fatalf("RestartResponse result = %d, want 1", ok)
	}
	reply = c.read()
	if reply[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("post-restart opcode = %#x, want CharSelectInfo (%#x)", reply[0], serverpackets.OpcodeCharSelectInfo)
	}
	if _, ok := state.Player(objID); ok {
		t.Fatalf("world.Player(%d) still present after restart", objID)
	}
	pos := chars.savedPosition(t, objID)
	if pos.location != savedAt || pos.heading != 32768 {
		t.Fatalf("saved position after restart = %+v/%d, want %+v/32768", pos.location, pos.heading, savedAt)
	}

	c.send(encodeRequestGameStart(0))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("second select opcode = %#x, want SSQInfo (%#x)", reply[0], serverpackets.OpcodeSSQInfo)
	}
}
