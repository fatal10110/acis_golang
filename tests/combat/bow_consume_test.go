package combat

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	bowTemplateID         int32 = 14
	woodenArrowTemplateID int32 = 17
	bowArrowStack               = 10
	bowMPConsume                = 1
	bowReuseDelayMS             = 1500
)

type liveBowPlayer interface {
	CurrentMP() int
	AttackSpeed() int
	Inventory() *itemcontainer.Inventory
}

// TestBowFireConsumesArrowAndMPAndSendsGauge pins bow fire-time: one equipped
// arrow and the weapon MP cost leave the inventory and vitals, the client
// gets the ready-to-shoot message plus a red SetupGauge covering attack time
// plus scaled reuse, then the Attack animation. InventoryUpdate is batched.
func TestBowFireConsumesArrowAndMPAndSendsGauge(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	bow := srv.GiveItem(t, objID, bowTemplateID, 1)
	arrows := srv.GiveItem(t, objID, woodenArrowTemplateID, bowArrowStack)
	startInWorld(t, c)

	equipAndFlush(t, srv, c, bow)
	equipAndFlush(t, srv, c, arrows)

	live := mustLiveBowPlayer(t, srv, objID)
	mpBefore := live.CurrentMP()
	if mpBefore < bowMPConsume {
		t.Fatalf("CurrentMP() = %d, want at least %d to spend", mpBefore, bowMPConsume)
	}
	atkSpd := live.AttackSpeed()
	if atkSpd <= 0 {
		t.Fatal("AttackSpeed() = 0")
	}
	// Independent oracle: max(100, 500000/pAtkSpd) + reuse*345/pAtkSpd.
	sAtk := 500000 / atkSpd
	if sAtk < 100 {
		sAtk = 100
	}
	wantGauge := sAtk + bowReuseDelayMS*345/atkSpd

	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	c.Send(encodeAttackRequest(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertFrameOpcode(t, mustRead(t, c, "select ValidateLocation"), serverpackets.OpcodeValidateLocation, "select ValidateLocation")
	assertFrameOpcode(t, mustRead(t, c, "MyTargetSelected"), serverpackets.OpcodeMyTargetSelected, "MyTargetSelected")
	assertFrameOpcode(t, mustRead(t, c, "selection StatusUpdate"), serverpackets.OpcodeStatusUpdate, "selection StatusUpdate")

	c.Send(encodeAttackRequest(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertAutoAttackStart(t, c, objID)

	status := mustRead(t, c, "bow MP StatusUpdate")
	assertPlayerMPStatus(t, status, objID, mpBefore-bowMPConsume)

	msg := mustRead(t, c, "ready-to-shoot SystemMessage")
	assertStaticBowSystemMessage(t, msg, serverpackets.SystemMessageGettingReadyToShootAnArrow)

	gauge := mustRead(t, c, "bow SetupGauge")
	assertBowSetupGauge(t, gauge, serverpackets.GaugeRed, wantGauge)

	assertAttackBy(t, c, objID)

	if got := live.CurrentMP(); got != mpBefore-bowMPConsume {
		t.Fatalf("CurrentMP() after bow fire = %d, want %d", got, mpBefore-bowMPConsume)
	}
	held := live.Inventory().ItemByObjectID(arrows)
	if held == nil {
		t.Fatal("arrow stack missing after bow fire")
	}
	if held.Count != bowArrowStack-1 {
		t.Fatalf("live arrow count = %d, want %d", held.Count, bowArrowStack-1)
	}

	srv.InventoryUpdates.Tick()
	e := readBowInventoryUpdateFor(t, c, arrows, bowArrowStack-1)
	if e.state != uint16(itemcontainer.UpdateModified) {
		t.Fatalf("InventoryUpdate state = %d, want modified (%d)", e.state, itemcontainer.UpdateModified)
	}
	srv.FlushItems(t)
	if inst := mustPersistedBowItem(t, srv, objID, arrows); inst.Count != bowArrowStack-1 {
		t.Fatalf("persisted arrow count = %d, want %d", inst.Count, bowArrowStack-1)
	}
}

func equipAndFlush(t *testing.T, srv *gameservertest.Server, c *scriptedClient, objectID int32) {
	t.Helper()
	c.Send(encodeUseItem(objectID, false))
	drainUntilQuiet(t, c)
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)
}

func mustLiveBowPlayer(t *testing.T, srv *gameservertest.Server, objID int32) liveBowPlayer {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	live, ok := obj.(liveBowPlayer)
	if !ok {
		t.Fatalf("world.Player(%d) = %T missing MP/inventory surface", objID, obj)
	}
	return live
}

func assertPlayerMPStatus(t *testing.T, frame []byte, objectID int32, wantMP int) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeStatusUpdate, "bow MP StatusUpdate")
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != objectID {
		t.Fatalf("StatusUpdate object id = %d, want %d", got, objectID)
	}
	count := r.ReadInt32()
	curMP := int32(-1)
	for i := int32(0); i < count; i++ {
		typ, val := r.ReadInt32(), r.ReadInt32()
		if typ == int32(serverpackets.StatusCurrentMP) {
			curMP = val
		}
	}
	if int(curMP) != wantMP {
		t.Fatalf("StatusUpdate CUR_MP = %d, want %d", curMP, wantMP)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read StatusUpdate: %v", err)
	}
}

func assertStaticBowSystemMessage(t *testing.T, frame []byte, messageID int) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(messageID) {
		t.Fatalf("SystemMessage id = %d, want %d", got, messageID)
	}
	if params := r.ReadInt32(); params != 0 {
		t.Fatalf("SystemMessage params = %d, want 0", params)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
}

func assertBowSetupGauge(t *testing.T, frame []byte, color serverpackets.GaugeColor, wantMs int) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSetupGauge, "SetupGauge")
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(color) {
		t.Fatalf("SetupGauge color = %d, want %d", got, color)
	}
	timeMs := r.ReadInt32()
	maxMs := r.ReadInt32()
	if int(timeMs) != wantMs || int(maxMs) != wantMs {
		t.Fatalf("SetupGauge time/max = %d/%d, want %d/%d", timeMs, maxMs, wantMs, wantMs)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SetupGauge: %v", err)
	}
}

type bowInventoryUpdateEntry struct {
	state    uint16
	objectID int32
	count    int
}

func readBowInventoryUpdateFor(t *testing.T, c *scriptedClient, objectID int32, wantCount int) bowInventoryUpdateEntry {
	t.Helper()
	frame := mustRead(t, c, "InventoryUpdate")
	assertFrameOpcode(t, frame, serverpackets.OpcodeInventoryUpdate, "InventoryUpdate")
	r := wire.NewReader(frame[1:])
	n := r.ReadUint16()
	var found bowInventoryUpdateEntry
	for i := uint16(0); i < n; i++ {
		e := bowInventoryUpdateEntry{state: r.ReadUint16()}
		_ = r.ReadUint16() // item category
		e.objectID = r.ReadInt32()
		_ = r.ReadInt32() // template id
		e.count = int(r.ReadInt32())
		_ = r.ReadUint16() // subCategory
		_ = r.ReadUint16() // CustomType1
		_ = r.ReadUint16() // equipped
		_ = r.ReadInt32()  // paperdoll slot
		_ = r.ReadUint16() // enchant
		_ = r.ReadUint16() // CustomType2
		_ = r.ReadInt32()  // augmentation
		_ = r.ReadInt32()  // mana left
		if e.objectID == objectID {
			found = e
		}
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read InventoryUpdate: %v", err)
	}
	if found.objectID != objectID {
		t.Fatalf("InventoryUpdate missing object %d", objectID)
	}
	if found.count != wantCount {
		t.Fatalf("InventoryUpdate count = %d, want %d", found.count, wantCount)
	}
	return found
}

func mustPersistedBowItem(t *testing.T, srv *gameservertest.Server, ownerID, objectID int32) *item.Instance {
	t.Helper()
	instances, err := srv.Items.ListByOwner(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	for _, inst := range instances {
		if inst.ObjectID == objectID {
			return inst
		}
	}
	t.Fatalf("no persisted item row for object %d (owner %d)", objectID, ownerID)
	return nil
}
