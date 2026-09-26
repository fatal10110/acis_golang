package combat

import (
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPlayerTeleportLeavesAndRejoinsTheGrid pins a short player teleport
// that lands inside the same region neighborhood (Creature.teleportTo,
// Creature.java:386-429). After TeleportToLocation the player leaves the
// grid: its selected monster is cleared (ActionFailed, TargetUnselected)
// before that monster's DeleteObject, every other nearby object is deleted
// too, and a watcher sees TeleportToLocation, the player's TargetUnselected
// (the old region is still set while the old area is forgotten,
// WorldObject.setRegion) and then DeleteObject for the player. Nothing is
// re-sent until the client's Appearing, which rejoins the grid (Appearing.java
// → Player.onTeleported): the player gets NpcInfo and CharInfo again before
// its UserInfo, the watcher gets the player's CharInfo, the selection stays
// cleared, and the facing the player had before the jump is kept
// (Creature.teleportTo only sets x/y/z).
func TestPlayerTeleportLeavesAndRejoinsTheGrid(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	w := joinWatcher(t, srv)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, w)
	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, w)
	watcherID := watcherObjectID(t, srv, objID)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	player, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}

	// A live-only facing, as an attack swing leaves it: the saved heading is
	// not updated.
	const facing = 12345
	player.Presence.SetHeading(facing)

	player.TeleportTo(playerOrigin.X+300, playerOrigin.Y, playerOrigin.Z, 0)
	frames := readQuiet(c)
	tp := indexOf(frames, 0, serverpackets.OpcodeTeleportToLocation, objID)
	if tp < 0 {
		t.Fatalf("no TeleportToLocation for the player in %s", opcodes(frames))
	}
	failed := indexOf(frames, tp, serverpackets.OpcodeActionFailed, -1)
	unselected := indexOf(frames, tp, serverpackets.OpcodeTargetUnselected, objID)
	hostileDeleted := indexOf(frames, tp, serverpackets.OpcodeDeleteObject, hostile.ObjectID())
	watcherDeleted := indexOf(frames, tp, serverpackets.OpcodeDeleteObject, watcherID)
	if failed < 0 || unselected < failed || hostileDeleted < unselected || watcherDeleted < 0 {
		t.Fatalf("after TeleportToLocation got %s, want ActionFailed → TargetUnselected → DeleteObject(monster) and DeleteObject(watcher)", opcodes(frames[tp:]))
	}
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeNPCInfo || f[0] == serverpackets.OpcodeCharInfo || f[0] == serverpackets.OpcodeUserInfo {
			t.Fatalf("opcode %#x re-sent before Appearing: %s", f[0], opcodes(frames))
		}
	}
	if got := onlineTarget(t, srv, objID); got != nil {
		t.Fatalf("Target() = %d after the teleport, want none", got.ObjectID())
	}
	if hostile.Knows(player) {
		t.Fatal("monster still knows the player before Appearing")
	}

	watched := readQuiet(w)
	wtp := indexOf(watched, 0, serverpackets.OpcodeTeleportToLocation, objID)
	wUnselected := indexOf(watched, wtp+1, serverpackets.OpcodeTargetUnselected, objID)
	if wtp < 0 || wUnselected < 0 || indexOf(watched, wUnselected, serverpackets.OpcodeDeleteObject, objID) < 0 {
		t.Fatalf("watcher got %s, want TeleportToLocation → TargetUnselected → DeleteObject for the player", opcodes(watched))
	}

	c.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	frames = readQuiet(c)
	info := indexOf(frames, 0, serverpackets.OpcodeNPCInfo, hostile.ObjectID())
	charInfo := indexOfCharInfo(frames, watcherID)
	user := indexOf(frames, 0, serverpackets.OpcodeUserInfo, -1)
	if info < 0 || charInfo < 0 || user < info || user < charInfo {
		t.Fatalf("after Appearing got %s, want NpcInfo(monster) and CharInfo(watcher) before UserInfo", opcodes(frames))
	}
	if got := int32(binary.LittleEndian.Uint32(frames[user][13:17])); got != facing {
		t.Fatalf("UserInfo heading = %d after the teleport, want %d kept", got, facing)
	}
	if !hostile.Knows(player) {
		t.Fatal("monster does not know the player after Appearing")
	}
	if got := onlineTarget(t, srv, objID); got != nil {
		t.Fatalf("Target() = %d after Appearing, want none", got.ObjectID())
	}
	if indexOfCharInfo(readQuiet(w), objID) < 0 {
		t.Fatal("watcher never got the player's CharInfo after Appearing")
	}
}

// joinBystander logs a second player into the shared spawn region, which may
// already hold NPCs. Its presence keeps the region active while the primary
// player is off the grid, so region-sleep resets don't mask what a teleport
// alone does.
func joinBystander(t *testing.T, srv *gameservertest.Server) {
	t.Helper()
	srv.SeedCharacterFor(t, "bystander", "Bystander", 1, 0)
	b := srv.DialClient(t, "bystander", 1)
	b.Send(encodeRequestGameStart(0))
	b.Send(encodeEnterWorld())
	drainUntilQuiet(t, b)
	drainUntilQuiet(t, srv.Client)
}

func watcherObjectID(t *testing.T, srv *gameservertest.Server, objID int32) int32 {
	t.Helper()
	for _, p := range srv.State.Players() {
		if p.ObjectID() != objID {
			return p.ObjectID()
		}
	}
	t.Fatal("watcher missing from world state")
	return 0
}

func readQuiet(c *scriptedClient) [][]byte {
	var frames [][]byte
	for f := c.ReadWithTimeout(readQuietWindow); f != nil; f = c.ReadWithTimeout(readQuietWindow) {
		frames = append(frames, f)
	}
	return frames
}

// indexOf finds the first frame at or after from with opcode op whose
// leading object id is id (any id when id is -1).
func indexOf(frames [][]byte, from int, op byte, id int32) int {
	for i := from; i < len(frames); i++ {
		f := frames[i]
		if f[0] != op {
			continue
		}
		if id == -1 || (len(f) >= 5 && int32(binary.LittleEndian.Uint32(f[1:5])) == id) {
			return i
		}
	}
	return -1
}

// indexOfCharInfo finds CharInfo for id; its object id follows x, y, z and
// the boat id.
func indexOfCharInfo(frames [][]byte, id int32) int {
	for i, f := range frames {
		if f[0] == serverpackets.OpcodeCharInfo && len(f) >= 21 && int32(binary.LittleEndian.Uint32(f[17:21])) == id {
			return i
		}
	}
	return -1
}

func opcodes(frames [][]byte) []string {
	out := make([]string, len(frames))
	for i, f := range frames {
		out[i] = fmt.Sprintf("%#x", f[0])
	}
	return out
}

// TestAppearingWaitsForAnOffQueueTeleport parks a teleport driven off the
// player's queue, as a summon-friend cast drives it from the caster's, right
// after its TeleportToLocation. An Appearing answered at once must not finish
// the teleport early: its UserInfo only goes out after the teleport is done,
// and carries the destination.
func TestAppearingWaitsForAnOffQueueTeleport(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	player, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	parker := &teleportParker{id: 1 << 30, subject: objID, parked: make(chan struct{}), release: make(chan struct{})}
	srv.State.Spawn(parker, playerOrigin.X+10, playerOrigin.Y, playerOrigin.Z, 0)
	t.Cleanup(func() { srv.State.Despawn(parker) })
	drainUntilQuiet(t, c)

	destX := playerOrigin.X + 300
	done := make(chan struct{})
	go func() {
		defer close(done)
		player.TeleportTo(destX, playerOrigin.Y, playerOrigin.Z, 0)
	}()
	select {
	case <-parker.parked:
	case <-time.After(5 * time.Second):
		close(parker.release)
		t.Fatal("teleport never broadcast TeleportToLocation to the parker")
	}
	readUntil(t, c, serverpackets.OpcodeTeleportToLocation, "TeleportToLocation")
	c.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	for f := c.ReadWithTimeout(readQuietWindow); f != nil; f = c.ReadWithTimeout(readQuietWindow) {
		if f[0] == serverpackets.OpcodeUserInfo {
			close(parker.release)
			t.Fatalf("UserInfo at (%d, ...) before the parked teleport finished", int32(binary.LittleEndian.Uint32(f[1:5])))
		}
	}
	close(parker.release)
	<-done

	user := readUntil(t, c, serverpackets.OpcodeUserInfo, "Appearing UserInfo")[0]
	if x := int32(binary.LittleEndian.Uint32(user[1:5])); x != int32(destX) {
		t.Fatalf("UserInfo x = %d, want the destination %d", x, destX)
	}
	if player.Teleporting() {
		t.Fatal("player still teleporting after Appearing")
	}
}

// teleportParker is a world object that sees the player and blocks the
// player's own TeleportToLocation broadcast until released.
type teleportParker struct {
	world.Presence
	id, subject int32
	parked      chan struct{}
	release     chan struct{}
	once        sync.Once
}

func (p *teleportParker) ObjectID() int32  { return p.id }
func (p *teleportParker) Kind() actor.Kind { return actor.KindStatic }

func (p *teleportParker) BroadcastFrame(f wire.Frame) bool {
	b := f.Bytes()
	if len(b) > wire.FrameHeaderSize {
		b = b[wire.FrameHeaderSize:]
	}
	if len(b) >= 5 && b[0] == serverpackets.OpcodeTeleportToLocation && int32(binary.LittleEndian.Uint32(b[1:5])) == p.subject {
		p.once.Do(func() {
			close(p.parked)
			<-p.release
		})
	}
	f.Release()
	return true
}
