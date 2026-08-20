//go:build integration

package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestGameClientLinkPickupGroundItemFullClientFlow is the regression test
// for two reported bugs, both observed after picking up a ground item: the
// item stayed visible on the ground until its unrelated auto-destroy timer
// eventually cleared it, and the character stopped responding to movement
// afterward — the same "accepted action packet answered with nothing"
// failure shape fixed for attacking (see the second-click Action tests in
// targeting_test.go), reached through the pickup path this time. It drives
// the real dispatch loop with a real TCP client end to end, rather than
// calling pickupLiveGroundItem directly, so it exercises the same Action
// opcode routing a live client relies on.
func TestGameClientLinkPickupGroundItemFullClientFlow(t *testing.T) {
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, nil, nil, 0)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := sqlSoleObjectID(t, chars)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	playerObj, ok := state.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	live := playerObj.(*livePlayer)

	px, py, pz := live.Position()

	adenaTmpl, ok := testItemTemplates().Get(item.AdenaID)
	if !ok {
		t.Fatal("missing test adena template")
	}
	ground, err := grounditem.New(item.Instance{ObjectID: 5000, TemplateID: item.AdenaID, Count: 40, ManaLeft: -1}, adenaTmpl)
	if err != nil {
		t.Fatalf("ground item: %v", err)
	}
	state.Spawn(ground, px+groundPickupInteractionDistance+1, py, pz, 0)
	if reply := c.read(); reply[0] != serverpackets.OpcodeSpawnItem {
		t.Fatalf("ground item spawn opcode = %#x, want SpawnItem (%#x)", reply[0], serverpackets.OpcodeSpawnItem)
	}

	origin := location.Location{X: px, Y: py, Z: pz}
	c.send(encodeAction(ground.ObjectID(), origin, false))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("Action opcode = %#x, want ActionFailed (%#x) — an out-of-range pickup must release the client's pending action before approaching", reply[0], serverpackets.OpcodeActionFailed)
	}
	reply = c.read()
	if reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("Action opcode = %#x, want MoveToLocation (%#x) — an out-of-range pickup must approach before collecting", reply[0], serverpackets.OpcodeMoveToLocation)
	}
	reply = c.read()
	if reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("Action opcode = %#x, want ActionFailed (%#x) — a successful pickup must still release the client's pending action", reply[0], serverpackets.OpcodeActionFailed)
	}
	reply = c.read()
	if reply[0] != serverpackets.OpcodeGetItem {
		t.Fatalf("Action opcode = %#x, want GetItem (%#x) — the pickup click was silently dropped", reply[0], serverpackets.OpcodeGetItem)
	}
	reply = c.read()
	if reply[0] != serverpackets.OpcodeDeleteObject {
		t.Fatalf("pickup follow-up opcode = %#x, want DeleteObject (%#x) — the item never disappears from the ground", reply[0], serverpackets.OpcodeDeleteObject)
	}
	inventoryUpdatesFor(t, state).Tick()
	reply = c.read()
	if reply[0] != serverpackets.OpcodeInventoryUpdate {
		t.Fatalf("pickup follow-up opcode = %#x, want InventoryUpdate (%#x)", reply[0], serverpackets.OpcodeInventoryUpdate)
	}

	if _, ok := state.Object(ground.ObjectID()); ok {
		t.Fatalf("world.Object(%d) still present after pickup", ground.ObjectID())
	}

	// Movement must still work after the pickup resolves.
	x, y, z := live.Position()
	c.send(encodeMoveBackwardToLocation(origin, location.Location{X: x, Y: y, Z: z}, 1))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("movement after pickup opcode = %#x, want MoveToLocation (%#x) — client is unresponsive to move commands", reply[0], serverpackets.OpcodeMoveToLocation)
	}
}

// TestAttachLivePlayerBuildsCastControllerEagerly is the regression test for
// issue #1183: live.cast used to be built lazily on the read-loop goroutine's
// first cast, while pickup-lock's scheduleAfter timer goroutine reads it
// unguarded (livePickupBlockedDeferrable). attachLivePlayer now builds it
// eagerly, alongside attackCtl, so it is fully initialized — with a
// happens-before edge to every later goroutine that touches live — before any
// read-loop or timer goroutine can race its first write.
func TestAttachLivePlayerBuildsCastControllerEagerly(t *testing.T) {
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, nil, nil, 0)

	c.send(encodeRequestCharacterCreate("Caster", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := sqlSoleObjectID(t, chars)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	playerObj, ok := state.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	live := playerObj.(*livePlayer)

	if live.cast == nil {
		t.Fatal("live.cast is nil after attach — cast controller must be built eagerly, not lazily on first cast, to avoid racing the pickup-lock timer goroutine")
	}
}

//	adena picked up while the player already holds an adena stack, so
//
// Container.Add takes the absorbed (merge) branch instead of inserting a new
// instance. Same end-to-end shape as
// TestGameClientLinkPickupGroundItemFullClientFlow.
func TestGameClientLinkPickupAdenaMergeFullClientFlow(t *testing.T) {
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, nil, nil, 0)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := sqlSoleObjectID(t, chars)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	playerObj, ok := state.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	live := playerObj.(*livePlayer)
	px, py, pz := live.Position()

	// Pre-existing adena stack -> pickup must merge.
	inv := live.Inventory()
	if inv == nil {
		t.Fatal("inventory missing")
	}
	if got := inv.AddNew(item.AdenaID, 100, 800); got == nil {
		t.Fatal("failed to seed existing adena stack")
	}
	inv.DrainUpdates() // discard the seeding update

	adenaTmpl, ok := testItemTemplates().Get(item.AdenaID)
	if !ok {
		t.Fatal("missing test adena template")
	}
	ground, err := grounditem.New(item.Instance{ObjectID: 5000, TemplateID: item.AdenaID, Count: 40, ManaLeft: -1}, adenaTmpl)
	if err != nil {
		t.Fatalf("ground item: %v", err)
	}
	state.Spawn(ground, px+30, py, pz, 0)
	if reply := c.read(); reply[0] != serverpackets.OpcodeSpawnItem {
		t.Fatalf("ground spawn opcode = %#x, want SpawnItem (%#x)", reply[0], serverpackets.OpcodeSpawnItem)
	}

	origin := location.Location{X: px, Y: py, Z: pz}
	c.send(encodeAction(ground.ObjectID(), origin, false))
	if reply := c.read(); reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("pickup opcode = %#x, want ActionFailed (%#x) — the client's pending action is never released", reply[0], serverpackets.OpcodeActionFailed)
	}
	if reply := c.read(); reply[0] != serverpackets.OpcodeGetItem {
		t.Fatalf("second Action opcode = %#x, want GetItem (%#x)", reply[0], serverpackets.OpcodeGetItem)
	}
	if reply := c.read(); reply[0] != serverpackets.OpcodeDeleteObject {
		t.Fatalf("follow-up opcode = %#x, want DeleteObject (%#x) — item stays on the ground", reply[0], serverpackets.OpcodeDeleteObject)
	}
	inventoryUpdatesFor(t, state).Tick()
	if reply := c.read(); reply[0] != serverpackets.OpcodeInventoryUpdate {
		t.Fatalf("follow-up opcode = %#x, want InventoryUpdate (%#x)", reply[0], serverpackets.OpcodeInventoryUpdate)
	}

	if _, ok := state.Object(ground.ObjectID()); ok {
		t.Fatalf("world.Object(%d) still present after pickup", ground.ObjectID())
	}
	if stack := inv.ItemByTemplateID(item.AdenaID); stack == nil || stack.Snapshot().Count != 140 {
		t.Fatalf("merged adena stack = %+v, want count 140", stack)
	}

	// Must still respond to movement.
	x, y, z := live.Position()
	c.send(encodeMoveBackwardToLocation(origin, location.Location{X: x, Y: y, Z: z}, 1))
	if reply := c.read(); reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("movement after pickup opcode = %#x, want MoveToLocation — character unresponsive", reply[0])
	}
}
