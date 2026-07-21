package network

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
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
	state.Spawn(ground, px+30, py, pz, 0)
	if reply := c.read(); reply[0] != serverpackets.OpcodeSpawnItem {
		t.Fatalf("ground item spawn opcode = %#x, want SpawnItem (%#x)", reply[0], serverpackets.OpcodeSpawnItem)
	}

	origin := location.Location{X: px, Y: py, Z: pz}
	c.send(encodeAction(ground.ObjectID(), origin, false))
	if reply := c.read(); reply[0] != serverpackets.OpcodeMyTargetSelected {
		t.Fatalf("first Action opcode = %#x, want MyTargetSelected (%#x)", reply[0], serverpackets.OpcodeMyTargetSelected)
	}

	c.send(encodeAction(ground.ObjectID(), origin, false))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeGetItem {
		t.Fatalf("second Action opcode = %#x, want GetItem (%#x) — the pickup click was silently dropped", reply[0], serverpackets.OpcodeGetItem)
	}
	reply = c.read()
	if reply[0] != serverpackets.OpcodeDeleteObject {
		t.Fatalf("pickup follow-up opcode = %#x, want DeleteObject (%#x) — the item never disappears from the ground", reply[0], serverpackets.OpcodeDeleteObject)
	}
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

// fakeAfterFuncs records scheduled delayed calls so a test can trigger them
// deterministically instead of waiting on a real timer.
type fakeAfterFuncs struct {
	calls []struct {
		delay time.Duration
		fn    func()
	}
}

func (f *fakeAfterFuncs) schedule(d time.Duration, fn func()) {
	f.calls = append(f.calls, struct {
		delay time.Duration
		fn    func()
	}{d, fn})
}

func (f *fakeAfterFuncs) fireAll() {
	for _, c := range f.calls {
		c.fn()
	}
}

func TestPickupLiveGroundItemLocksAndReleasesTransientParalysis(t *testing.T) {
	templates := petTestTemplates()
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	state := world.New()
	state.Spawn(live, 100, 0, 0, 0)
	drops := task.NewGroundItems(state, task.GroundItemOptions{ItemAutoDestroy: time.Hour}, time.Now)
	tmpl, _ := templates.Get(item.AdenaID)
	ground := dropTestGround(t, state, drops, item.Instance{ObjectID: 900, TemplateID: item.AdenaID, Count: 40, ManaLeft: -1}, tmpl, 100, 0, 0)

	store := &recordingEnchantItemStore{}
	fake := &fakeAfterFuncs{}
	gcl := &GameClientLink{world: state, groundItems: drops, items: store, afterFunc: fake.schedule}

	if live.Paralyzed() {
		t.Fatal("Paralyzed() = true before any pickup")
	}

	if !gcl.pickupLiveGroundItem(context.Background(), live, ground) {
		t.Fatal("pickupLiveGroundItem returned false for a ground item target")
	}

	if !live.Paralyzed() {
		t.Fatal("Paralyzed() = false immediately after a successful pickup")
	}
	if len(fake.calls) != 1 || fake.calls[0].delay != pickupParalyzeLock {
		t.Fatalf("scheduled calls = %+v, want exactly one call after %s", fake.calls, pickupParalyzeLock)
	}

	fake.fireAll()

	if live.Paralyzed() {
		t.Fatal("Paralyzed() = true after the scheduled release ran")
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

func TestPickupLiveGroundItemBroadcastsAttentionForWeaponAndArmor(t *testing.T) {
	for _, tc := range []struct {
		name         string
		templateID   int32
		enchantLevel int
		messageID    int
	}{
		{name: "weapon", templateID: 2375, messageID: serverpackets.SystemMessageAttentionS1PickedUpS2},
		{name: "enchanted armor", templateID: 1146, enchantLevel: 3, messageID: serverpackets.SystemMessageAttentionS1PickedUpS2S3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			templates := pickupAttentionTestTemplates()
			pickerFrames := &frameCapture{}
			observerFrames := &frameCapture{}
			farFrames := &frameCapture{}
			picker := newEquipTestLivePlayer(t, 1, pickerFrames, templates, nil)
			observer := newEquipTestLivePlayer(t, 2, observerFrames, templates, nil)
			far := newEquipTestLivePlayer(t, 3, farFrames, templates, nil)
			state := world.New()
			state.Spawn(picker, 100, 0, 0, 0)
			state.Spawn(observer, 200, 0, 0, 0)
			state.Spawn(far, 100+pickupAttentionRadius+1, 0, 0, 0)
			drops := task.NewGroundItems(state, task.GroundItemOptions{ItemAutoDestroy: time.Hour}, time.Now)
			tmpl, _ := templates.Get(tc.templateID)
			ground := dropTestGround(t, state, drops, item.Instance{
				ObjectID:     900,
				TemplateID:   tc.templateID,
				Count:        1,
				EnchantLevel: tc.enchantLevel,
				ManaLeft:     -1,
			}, tmpl, 100, 0, 0)

			pickerFrames.frames = nil
			observerFrames.frames = nil
			farFrames.frames = nil
			gcl := &GameClientLink{world: state, groundItems: drops}

			gcl.pickupLiveGroundItem(context.Background(), picker, ground)

			assertPickupAttentionFrame(t, firstSystemMessageFrame(pickerFrames.frames), tc.messageID, picker.Name, tc.enchantLevel, tc.templateID)
			assertPickupAttentionFrame(t, firstSystemMessageFrame(observerFrames.frames), tc.messageID, picker.Name, tc.enchantLevel, tc.templateID)
			if frame := firstSystemMessageFrame(farFrames.frames); frame != nil {
				t.Fatalf("far observer received attention SystemMessage frame: % x", frame)
			}
		})
	}
}

func TestPickupLiveGroundItemSkipsAttentionForEtcItem(t *testing.T) {
	templates := pickupAttentionTestTemplates()
	pickerFrames := &frameCapture{}
	observerFrames := &frameCapture{}
	picker := newEquipTestLivePlayer(t, 1, pickerFrames, templates, nil)
	observer := newEquipTestLivePlayer(t, 2, observerFrames, templates, nil)
	state := world.New()
	state.Spawn(picker, 100, 0, 0, 0)
	state.Spawn(observer, 200, 0, 0, 0)
	drops := task.NewGroundItems(state, task.GroundItemOptions{ItemAutoDestroy: time.Hour}, time.Now)
	tmpl, _ := templates.Get(item.AdenaID)
	ground := dropTestGround(t, state, drops, item.Instance{ObjectID: 900, TemplateID: item.AdenaID, Count: 40, ManaLeft: -1}, tmpl, 100, 0, 0)

	pickerFrames.frames = nil
	observerFrames.frames = nil
	gcl := &GameClientLink{world: state, groundItems: drops}

	gcl.pickupLiveGroundItem(context.Background(), picker, ground)

	if frame := firstSystemMessageFrame(pickerFrames.frames); frame != nil {
		t.Fatalf("etc pickup sent picker attention SystemMessage frame: % x", frame)
	}
	if frame := firstSystemMessageFrame(observerFrames.frames); frame != nil {
		t.Fatalf("etc pickup broadcast SystemMessage frame: % x", frame)
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

	// ActionFailed must follow the system message, or the client's pending
	// pickup action never resolves — the same "stuck, unresponsive to
	// movement" bug reported for a rejected pickup click.
	assertOpcodeSequence(t, capture.frames, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed)
	if _, ok := state.Object(ground.ObjectID()); !ok {
		t.Fatal("ground item removed after an out-of-range pickup attempt")
	}
}

func pickupAttentionTestTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: item.AdenaID, Name: "Adena", Kind: item.KindEtcItem, Duration: -1, Stackable: true, Dropable: true, Tradable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{}},
		{ID: 2375, Name: "Wolf Tooth", Kind: item.KindWeapon, Slot: item.SlotWolf, Duration: -1, Dropable: true, Tradable: true, Destroyable: true, Weapon: &item.WeaponDetail{Type: item.WeaponPet}},
		{ID: 1146, Name: "Cotton Shirt", Kind: item.KindArmor, Slot: item.SlotChest, Duration: -1, Dropable: true, Tradable: true, Destroyable: true, Armor: &item.ArmorDetail{Type: item.ArmorLight}},
	})
}

func firstSystemMessageFrame(frames [][]byte) []byte {
	for _, frame := range frames {
		if len(frame) > 0 && frame[0] == serverpackets.OpcodeSystemMessage {
			return frame
		}
	}
	return nil
}

func assertPickupAttentionFrame(t *testing.T, frame []byte, wantID int, wantName string, wantEnchant int, wantItemID int32) {
	t.Helper()
	if frame == nil {
		t.Fatal("missing attention SystemMessage frame")
	}
	if frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want SystemMessage", frame[0])
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(wantID) {
		t.Fatalf("SystemMessage id = %d, want %d", got, wantID)
	}
	wantParams := int32(2)
	if wantEnchant > 0 {
		wantParams = 3
	}
	if got := r.ReadInt32(); got != wantParams {
		t.Fatalf("SystemMessage param count = %d, want %d", got, wantParams)
	}
	if got := r.ReadInt32(); got != serverpackets.SystemMessageParamText {
		t.Fatalf("param[0] type = %d, want text", got)
	}
	if got := r.ReadString(); got != wantName {
		t.Fatalf("param[0] text = %q, want %q", got, wantName)
	}
	if wantEnchant > 0 {
		if got := r.ReadInt32(); got != serverpackets.SystemMessageParamNumber {
			t.Fatalf("param[1] type = %d, want number", got)
		}
		if got := r.ReadInt32(); got != int32(wantEnchant) {
			t.Fatalf("param[1] number = %d, want %d", got, wantEnchant)
		}
	}
	if got := r.ReadInt32(); got != serverpackets.SystemMessageParamItemName {
		t.Fatalf("item param type = %d, want item name", got)
	}
	if got := r.ReadInt32(); got != wantItemID {
		t.Fatalf("item id = %d, want %d", got, wantItemID)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("decode attention SystemMessage: %v", err)
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

	assertOpcodeSequence(t, capture.frames, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed)
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
	// This is the regression case for the reported bug: a full inventory
	// (an easy state to reach while playtesting pickup) previously answered
	// only with the system message, leaving the client's action pending
	// forever — matching "item never disappears" (the pickup click that
	// would have retried never got a chance) and "character stops
	// responding to movement".
	if got := frameOpcodes(capture.frames); string(got) != string([]byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed}) {
		t.Fatalf("slots-full opcodes = %x, want SystemMessage, ActionFailed", got)
	}
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
