package network

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
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
	live.Character.LastHeading = 32768
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

func TestDetachLivePlayerPersistsOfflineRecency(t *testing.T) {
	chars := newFakeCharStore()
	items := newFakeItemStore()
	fixedNow := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	roster := gamemanager.NewRoster(chars, items, nil, testTemplates(t), testItemTemplates(), npc.NewTable(nil), &sequentialIDs{next: 100}, gamemanager.DefaultDeleteAfter, func() time.Time { return fixedNow })
	live := newTestLivePlayer(t, 101, &frameCapture{})
	if err := chars.Create(context.Background(), live.Character); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	gcl := &GameClientLink{roster: roster, log: zerolog.Nop()}
	gcl.detachLivePlayer(context.Background(), live)

	lastAccess, ok := chars.lastOffline(live.ObjectID())
	if !ok {
		t.Fatal("detachLivePlayer did not persist offline recency")
	}
	if want := fixedNow.UnixMilli(); lastAccess != want {
		t.Fatalf("lastAccess = %d, want %d", lastAccess, want)
	}
}

// TestDetachLivePlayerSavesFullStats pins detachLivePlayer to
// GameClient.closeNetConnection calling store() on disconnect: level, exp,
// sp, and cur/max HP/CP/MP must be persisted alongside the existing
// position/death-penalty/offline-recency saves, or progress since the last
// autosave is lost on logout.
func TestDetachLivePlayerSavesFullStats(t *testing.T) {
	chars := newFakeCharStore()
	items := newFakeItemStore()
	roster := gamemanager.NewRoster(chars, items, nil, testTemplates(t), testItemTemplates(), npc.NewTable(nil), &sequentialIDs{next: 100}, gamemanager.DefaultDeleteAfter, time.Now)
	live := newTestLivePlayer(t, 101, &frameCapture{})
	if err := chars.Create(context.Background(), live.Character); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	gcl := &GameClientLink{roster: roster, log: zerolog.Nop()}
	gcl.detachLivePlayer(context.Background(), live)

	if got := chars.saves(live.ObjectID()); got != 1 {
		t.Fatalf("full-stat saves after detach = %d, want 1", got)
	}
}

// TestDetachLivePlayerStopsAttackIntention is the regression test for the
// "disconnect mid-fight leaves timers running" review finding: detaching a
// live player mid-swing must stop the attack intention before the frame
// sender/broadcaster hooks are nulled, or a timer goroutine can still fire
// against a half-torn-down player.
func TestDetachLivePlayerStopsAttackIntention(t *testing.T) {
	state := world.New()
	attackerFrames := &frameCapture{}
	attacker := newTestLivePlayer(t, 1, attackerFrames)
	attacker.Character.SetWorld(state)
	attacker.Character.SetRollSource(func(int) int { return 0 })
	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	wireLiveAttackHooks(gcl, attacker)
	target := newTestHostileNPC(t, 3007)
	target.Instance.Template.PDef = 1
	target.Instance.Template.DEX = 30
	target.SetRollSource(func(int) int { return 0 })

	state.Spawn(attacker, 0, 0, 0, 0)
	state.Spawn(target, 30, 0, 0, 0)
	if !gcl.attackLiveTarget(attacker, target) {
		t.Fatal("attackLiveTarget returned false for an in-range target")
	}
	if !attacker.attack.AttackingNow() {
		t.Fatal("attack controller is not tracking the active swing before detach")
	}

	gcl.detachLivePlayer(context.Background(), attacker)

	if attacker.attack.AttackingNow() {
		t.Fatal("attack controller still tracking a swing after detach")
	}
	if attacker.combat.Target() != nil {
		t.Fatalf("attack intention target = %v after detach, want nil", attacker.combat.Target())
	}
}

func TestDetachLivePlayerRacesNoHookAccess(t *testing.T) {
	state := world.New()
	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	attacker := newTestLivePlayer(t, 1, &frameCapture{})
	target := newTestLivePlayer(t, 2, &frameCapture{})
	state.Spawn(attacker, 0, 0, 0, 0)
	state.Spawn(target, 30, 0, 0, 0)
	state.AddPlayer(attacker)
	state.AddPlayer(target)

	snapshot := attack.Snapshot{AttackerID: attacker.ObjectID(), Hits: []attack.SnapshotHit{{TargetID: target.ObjectID(), Damage: 1}}}
	send := func(wire.Frame) bool { return true }

	const iterations = 1000
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			gcl.broadcastAttack(attacker, snapshot)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			target.Character.SetFrameSender(send)
			target.Character.SetAttackBroadcaster(func(attack.Snapshot) {})
			target.Character.SetMoveBroadcaster(func(move.Event) {})
			target.Character.SetStopBroadcaster(func() {})
			target.Character.SetFrameSender(nil)
			target.Character.SetAttackBroadcaster(nil)
			target.Character.SetMoveBroadcaster(nil)
			target.Character.SetStopBroadcaster(nil)
		}
	}()
	wg.Wait()
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

	savedAt := location.Location{X: 80, Y: 70, Z: 30}
	spawn := location.Location{X: 10, Y: 20, Z: 30}
	c.send(encodeMoveBackwardToLocation(savedAt, spawn, 1))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation (%#x)", reply[0], serverpackets.OpcodeMoveToLocation)
	}
	waitForWorldPosition(t, state, objID, savedAt)
	walkHeading := spawn.HeadingTo(savedAt)
	c.send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeLeaveWorld {
		t.Fatalf("logout opcode = %#x, want LeaveWorld (%#x)", reply[0], serverpackets.OpcodeLeaveWorld)
	}
	c.expectClosed()
	if _, ok := state.Player(objID); ok {
		t.Fatalf("world.Player(%d) still present after logout", objID)
	}
	pos := chars.savedPosition(t, objID)
	if pos.location != savedAt || pos.heading != walkHeading {
		t.Fatalf("saved position after logout = %+v/%d, want %+v/%d", pos.location, pos.heading, savedAt, walkHeading)
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

	savedAt := location.Location{X: 80, Y: 70, Z: 30}
	spawn := location.Location{X: 10, Y: 20, Z: 30}
	c.send(encodeMoveBackwardToLocation(savedAt, spawn, 1))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation (%#x)", reply[0], serverpackets.OpcodeMoveToLocation)
	}
	waitForWorldPosition(t, state, objID, savedAt)
	walkHeading := spawn.HeadingTo(savedAt)
	c.send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	reply = c.read()
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
	if pos.location != savedAt || pos.heading != walkHeading {
		t.Fatalf("saved position after restart = %+v/%d, want %+v/%d", pos.location, pos.heading, savedAt, walkHeading)
	}

	c.send(encodeRequestGameStart(0))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("second select opcode = %#x, want SSQInfo (%#x)", reply[0], serverpackets.OpcodeSSQInfo)
	}
}

// TestDetachLivePlayerStopsAutosave pins detachLivePlayer to
// GameClient.closeNetConnection stopping the periodic autosave alongside
// every other per-connection timer: a session that already logged out must
// never resurface in a later autosave tick.
func TestDetachLivePlayerStopsAutosave(t *testing.T) {
	saveCount := 0
	effects := autosaveCountingEffects(func(task.AutosaveActor) { saveCount++ })
	now := time.UnixMilli(0)
	autosave, err := task.NewAutosave(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewAutosave() error = %v", err)
	}
	live := newTestLivePlayer(t, 101, &frameCapture{})
	autosave.Add(live)

	gcl := &GameClientLink{autosave: autosave, log: zerolog.Nop()}
	gcl.detachLivePlayer(context.Background(), live)

	now = now.Add(task.AutosaveInitialDelay)
	autosave.Tick()
	if saveCount != 0 {
		t.Fatalf("saves after detach = %d, want 0", saveCount)
	}
}

type autosaveCountingEffects func(task.AutosaveActor)

func (f autosaveCountingEffects) Save(actor task.AutosaveActor) { f(actor) }

// waitForWorldPosition polls until the world-grid presence reaches want,
// which happens when the simulated walk arrives. The walk duration is
// driven by the move controller's arrival timer, so the test waits
// event-style instead of sleeping a fixed amount.
func waitForWorldPosition(t *testing.T, state *world.State, objID int32, want location.Location) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		obj, ok := state.Player(objID)
		if !ok {
			t.Fatal("world player missing while waiting for walk arrival")
		}
		positioned, ok := obj.(interface{ Position() (int, int, int) })
		if !ok {
			t.Fatal("world player has no Position method")
		}
		x, y, z := positioned.Position()
		if x == want.X && y == want.Y && z == want.Z {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("player position after walk = (%d,%d,%d), want %+v", x, y, z, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
