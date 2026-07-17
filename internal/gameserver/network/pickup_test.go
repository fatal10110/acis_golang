package network

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func dropTestGround(t *testing.T, state *world.State, drops *task.GroundItems, inst item.Instance, tmpl *item.Template, x, y, z int) *grounditem.Item {
	t.Helper()
	ground, err := grounditem.New(inst, tmpl)
	if err != nil {
		t.Fatalf("ground item: %v", err)
	}
	drops.Drop(ground, task.DropOptions{X: x, Y: y, Z: z})
	return ground
}

func TestPickupLiveGroundItemMovesItemAndDespawns(t *testing.T) {
	templates := petTestTemplates()
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	state := world.New()
	state.Spawn(live, 100, 0, 0, 0)
	drops := task.NewGroundItems(state, task.GroundItemOptions{ItemAutoDestroy: time.Hour}, time.Now)
	tmpl, _ := templates.Get(item.AdenaID)
	ground := dropTestGround(t, state, drops, item.Instance{ObjectID: 900, TemplateID: item.AdenaID, Count: 40, ManaLeft: -1}, tmpl, 100, 0, 0)

	capture.frames = nil
	store := &recordingEnchantItemStore{}
	gcl := &GameClientLink{world: state, groundItems: drops, items: store}

	if !gcl.pickupLiveGroundItem(context.Background(), live, ground) {
		t.Fatal("pickupLiveGroundItem returned false for a ground item target")
	}

	assertOpcodeSequence(t, capture.frames,
		serverpackets.OpcodeGetItem,
		serverpackets.OpcodeDeleteObject,
		serverpackets.OpcodeInventoryUpdate,
	)
	if _, ok := state.Object(ground.ObjectID()); ok {
		t.Fatalf("world.Object(%d) still present after pickup", ground.ObjectID())
	}
	if got := drops.Len(); got != 0 {
		t.Fatalf("ground item tracker Len = %d, want 0", got)
	}
	stack := live.Inventory().ItemByTemplateID(item.AdenaID)
	if stack == nil || stack.ObjectID != ground.ObjectID() || stack.Count != 40 || stack.OwnerID != live.ObjectID() {
		t.Fatalf("inventory stack = %+v, want picked up ground adena", stack)
	}
	if len(store.saved) != 1 || store.saved[0].ObjectID != ground.ObjectID() {
		t.Fatalf("saved rows = %+v, want ground row moved to inventory", store.saved)
	}
}

func TestPickupLiveGroundItemMergesStackAndDeletesGroundRow(t *testing.T) {
	templates := petTestTemplates()
	existing := &item.Instance{ObjectID: 800, TemplateID: item.AdenaID, OwnerID: 1, Count: 10, Location: item.LocationInventory}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{existing})
	state := world.New()
	state.Spawn(live, 100, 0, 0, 0)
	drops := task.NewGroundItems(state, task.GroundItemOptions{ItemAutoDestroy: time.Hour}, time.Now)
	tmpl, _ := templates.Get(item.AdenaID)
	ground := dropTestGround(t, state, drops, item.Instance{ObjectID: 900, TemplateID: item.AdenaID, Count: 40, ManaLeft: -1}, tmpl, 100, 0, 0)

	capture.frames = nil
	store := &recordingEnchantItemStore{}
	gcl := &GameClientLink{world: state, groundItems: drops, items: store}

	gcl.pickupLiveGroundItem(context.Background(), live, ground)

	stack := live.Inventory().ItemByTemplateID(item.AdenaID)
	if stack != existing || stack.Count != 50 {
		t.Fatalf("inventory stack = %+v, want merged 50 adena", stack)
	}
	if len(store.updated) != 1 || store.updated[0].ObjectID != existing.ObjectID || store.updated[0].Count != 50 {
		t.Fatalf("updated rows = %+v, want merged inventory stack", store.updated)
	}
	if len(store.deleted) != 1 || store.deleted[0] != ground.ObjectID() {
		t.Fatalf("deleted rows = %+v, want absorbed ground row", store.deleted)
	}
	if len(store.saved) != 0 {
		t.Fatalf("saved rows = %+v, want none for absorbed ground stack", store.saved)
	}
}

func TestPickupLiveGroundItemRejectsOutOfRange(t *testing.T) {
	templates := petTestTemplates()
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	state := world.New()
	state.Spawn(live, 100, 0, 0, 0)
	drops := task.NewGroundItems(state, task.GroundItemOptions{ItemAutoDestroy: time.Hour}, time.Now)
	tmpl, _ := templates.Get(item.AdenaID)
	ground := dropTestGround(t, state, drops, item.Instance{ObjectID: 900, TemplateID: item.AdenaID, Count: 40, ManaLeft: -1}, tmpl, 100+groundPickupInteractionDistance+1, 0, 0)

	capture.frames = nil
	gcl := &GameClientLink{world: state, groundItems: drops}

	gcl.pickupLiveGroundItem(context.Background(), live, ground)

	assertOpcodeSequence(t, capture.frames, serverpackets.OpcodeSystemMessage)
	if _, ok := state.Object(ground.ObjectID()); !ok {
		t.Fatal("ground item removed after an out-of-range pickup attempt")
	}
}

func TestPickupLiveGroundItemRejectsLootLockedByOtherOwner(t *testing.T) {
	templates := petTestTemplates()
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	state := world.New()
	state.Spawn(live, 100, 0, 0, 0)
	drops := task.NewGroundItems(state, task.GroundItemOptions{ItemAutoDestroy: time.Hour}, time.Now)
	tmpl, _ := templates.Get(item.AdenaID)
	ground := dropTestGround(t, state, drops, item.Instance{ObjectID: 900, TemplateID: item.AdenaID, OwnerID: 99, Count: 40, ManaLeft: -1}, tmpl, 100, 0, 0)

	capture.frames = nil
	gcl := &GameClientLink{world: state, groundItems: drops}

	gcl.pickupLiveGroundItem(context.Background(), live, ground)

	assertOpcodeSequence(t, capture.frames, serverpackets.OpcodeSystemMessage)
	if _, ok := state.Object(ground.ObjectID()); !ok {
		t.Fatal("loot-locked ground item removed by a non-owner pickup attempt")
	}
}

func TestPickupLiveGroundItemRejectsWhenSlotsFull(t *testing.T) {
	templates := petTestTemplates()
	held := &item.Instance{ObjectID: 800, TemplateID: 2375, OwnerID: 1, Count: 1, Location: item.LocationInventory}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{held})
	live.Inventory().SlotLimit = 1
	state := world.New()
	state.Spawn(live, 100, 0, 0, 0)
	drops := task.NewGroundItems(state, task.GroundItemOptions{ItemAutoDestroy: time.Hour}, time.Now)
	tmpl, _ := templates.Get(int32(2375))
	ground := dropTestGround(t, state, drops, item.Instance{ObjectID: 900, TemplateID: 2375, Count: 1, ManaLeft: -1}, tmpl, 100, 0, 0)

	capture.frames = nil
	gcl := &GameClientLink{world: state, groundItems: drops}

	gcl.pickupLiveGroundItem(context.Background(), live, ground)

	assertSystemMessageIDFrame(t, capture.frames[0], serverpackets.SystemMessageSlotsFull)
	if _, ok := state.Object(ground.ObjectID()); !ok {
		t.Fatal("ground item removed after a slots-full pickup attempt")
	}
}

func assertSystemMessageIDFrame(t *testing.T, frame []byte, want int) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want SystemMessage", frame[0])
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(want) {
		t.Fatalf("SystemMessage id = %d, want %d", got, want)
	}
}
