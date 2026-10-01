package items

import (
	"context"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: PaganKeys.useItem (handler/itemhandlers/PaganKeys.java:18-82)
// opens doors directly, with no cast. The target must be a Door
// (INVALID_TARGET + ActionFailed otherwise) within Npc.INTERACTION_DISTANCE
// (150, 3D, exclusive) of the player (DIST_TOO_FAR_CASTING_STOPPED +
// ActionFailed otherwise); both refusals keep the key. Then
// Player.destroyItem(objectId, 1, true) spends one key, naming it with
// S1_DISAPPEARED (Player.java:1927-1949), before the door id is matched:
// 8056 opens both 23150003 and 23150004 from either one, 8273 opens
// 19160002-19160009, 8275 opens 19160012 and 19160013. A key used on any
// other door is still spent and answers S1_CANNOT_BE_USED naming it.
// Every key and door below is the shipped datapack's
// (aCis_datapack/data/xml/items/8000-8099.xml:399-405,
// 8200-8299.xml:854-874, doors.xml).
const (
	keyOfSplendorRoomID int32 = 8056
	anteroomKeyID       int32 = 8273
	keyOfDarknessID     int32 = 8275

	splendorDoorA   = 23150003
	splendorDoorB   = 23150004
	corridorDoor    = 19160002
	altarSecretDoor = 19160012
	sanctuaryGate   = 19160010
)

var shippedDoors = sync.OnceValues(func() (*door.Table, error) {
	dir, _ := datapack.Find()
	return xmldata.LoadDoors(filepath.Join(dir, "data", "xml", "doors.xml"), zerolog.Nop())
})

// bootPaganKeys boots a character carrying held units of keyID beside the
// shipped door target, offset dy from its position, with the shipped doors
// in doorIDs spawned. It returns the server, the key's object id, and the
// target door.
func bootPaganKeys(t *testing.T, keyID int32, held int32, target int, dy int, doorIDs ...int) (*gameservertest.Server, int32, *door.Object) {
	t.Helper()
	datapack.Require(t)
	_, shippedItems := shippedData()
	keyTmpl, ok := shippedItems.Get(keyID)
	if !ok {
		t.Fatalf("shipped item %d missing", keyID)
	}
	doorTable, err := shippedDoors()
	if err != nil {
		t.Fatalf("load shipped doors: %v", err)
	}
	var doors []*door.Template
	for _, id := range doorIDs {
		tmpl, ok := doorTable.Get(id)
		if !ok {
			t.Fatalf("shipped door %d missing", id)
		}
		doors = append(doors, tmpl)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithItemTemplates(item.NewTable(append(gameservertest.ItemTemplates().All(), keyTmpl))),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithDoors(doors...),
	)
	objID := srv.SoleObjectID(t)
	gate, ok := srv.WorldObjects.Door(target)
	if !ok {
		t.Fatalf("door %d not spawned", target)
	}
	at := gate.Template.Position
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET x = ?, y = ?, z = ? WHERE obj_Id = ?", at.X, at.Y+dy, at.Z, objID); err != nil {
		t.Fatalf("place character: %v", err)
	}
	key := srv.GiveItem(t, objID, keyID, held)

	// The doors known at spawn add their own frames to the EnterWorld
	// reply, so this entry drains the burst rather than matching it.
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "CharSelected")
	c.Send(encodeEnterWorld())
	drainUntilQuiet(t, c)
	return srv, key, gate
}

// selectDoor clicks gate and drains the selection answer.
func selectDoor(t *testing.T, c *testsupport.ScriptedClient, gate *door.Object) {
	t.Helper()
	c.Send(encodeAction(gate.ObjectID(), 0, 0, 0, false))
	for range 10 {
		frame := c.Read()
		if frame[0] == serverpackets.OpcodeMyTargetSelected {
			if got := wire.NewReader(frame[1:]).ReadInt32(); got != gate.ObjectID() {
				t.Fatalf("MyTargetSelected object id = %d, want %d", got, gate.ObjectID())
			}
			drainUntilQuiet(t, c)
			return
		}
	}
	t.Fatalf("door %d was never selected", gate.DoorID())
}

// useKey sends UseItem on key and returns every frame up to the client going
// quiet.
func useKey(t *testing.T, c *testsupport.ScriptedClient, key int32) [][]byte {
	t.Helper()
	c.Send(encodeUseItem(key, false))
	var frames [][]byte
	for {
		frame := c.ReadWithTimeout(500 * time.Millisecond)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
}

func keysLeft(t *testing.T, srv *gameservertest.Server, key int32) int {
	t.Helper()
	inst := srv.PlayerInventory(t, srv.SoleObjectID(t)).ItemByObjectID(key)
	if inst == nil {
		return 0
	}
	return inst.CountValue()
}

// doorStatusUpdates returns the door object ids each DoorStatusUpdate in
// frames reports open.
func doorStatusUpdates(t *testing.T, frames [][]byte) []int32 {
	t.Helper()
	var opened []int32
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeDoorStatusUpdate {
			continue
		}
		r := wire.NewReader(f[1:])
		id := r.ReadInt32()
		if closed := r.ReadInt32(); closed != 0 {
			t.Fatalf("DoorStatusUpdate for %d reports closed, want open", id)
		}
		opened = append(opened, id)
	}
	return opened
}

// TestPaganKeyOpensItsDoors uses each key on a door it fits: one key is
// spent and named, and exactly the reference doors open with their status
// broadcast. The Key of Splendor Room opens both splendor room doors from
// either one.
func TestPaganKeyOpensItsDoors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		key      int32
		target   int
		spawned  []int
		wantOpen []int
	}{
		{name: "splendor key on door A", key: keyOfSplendorRoomID, target: splendorDoorA, spawned: []int{splendorDoorA, splendorDoorB}, wantOpen: []int{splendorDoorA, splendorDoorB}},
		{name: "splendor key on door B", key: keyOfSplendorRoomID, target: splendorDoorB, spawned: []int{splendorDoorA, splendorDoorB}, wantOpen: []int{splendorDoorA, splendorDoorB}},
		{name: "anteroom key", key: anteroomKeyID, target: corridorDoor, spawned: []int{corridorDoor, 19160003}, wantOpen: []int{corridorDoor}},
		{name: "anteroom key last corridor door", key: anteroomKeyID, target: 19160009, spawned: []int{19160009, 19160008}, wantOpen: []int{19160009}},
		{name: "key of darkness", key: keyOfDarknessID, target: altarSecretDoor, spawned: []int{altarSecretDoor, 19160013}, wantOpen: []int{altarSecretDoor}},
		{name: "key of darkness other altar door", key: keyOfDarknessID, target: 19160013, spawned: []int{19160013, altarSecretDoor}, wantOpen: []int{19160013}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, key, gate := bootPaganKeys(t, tc.key, 2, tc.target, 100, tc.spawned...)
			c := srv.Client
			selectDoor(t, c, gate)

			frames := useKey(t, c, key)

			msgs := systemMessages(frames)
			if len(msgs) != 1 {
				t.Fatalf("system messages = %d, want only the key's S1_DISAPPEARED", len(msgs))
			}
			assertSystemMessageItem(t, msgs[0], serverpackets.SystemMessageS1Disappeared, tc.key)
			for _, f := range frames {
				if f[0] == serverpackets.OpcodeActionFailed || f[0] == serverpackets.OpcodeMagicSkillUse {
					t.Fatalf("key use sent opcode %#x, want no cast and no ActionFailed", f[0])
				}
			}
			if got := keysLeft(t, srv, key); got != 1 {
				t.Fatalf("keys left = %d, want 1", got)
			}
			var wantUpdates []int32
			for _, id := range tc.spawned {
				obj, _ := srv.WorldObjects.Door(id)
				want := slices.Contains(tc.wantOpen, id)
				if obj.Opened() != want {
					t.Fatalf("door %d opened = %v, want %v", id, obj.Opened(), want)
				}
				if want {
					wantUpdates = append(wantUpdates, obj.ObjectID())
				}
			}
			got := doorStatusUpdates(t, frames)
			slices.Sort(got)
			slices.Sort(wantUpdates)
			if !slices.Equal(got, wantUpdates) {
				t.Fatalf("DoorStatusUpdate doors = %v, want %v", got, wantUpdates)
			}
		})
	}
}

// TestPaganKeyWrongDoorSpendsKey uses each key on a door it does not fit:
// the key is still spent and named, then S1_CANNOT_BE_USED names it, and the
// door stays closed.
func TestPaganKeyWrongDoorSpendsKey(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		key    int32
		target int
	}{
		{name: "splendor key on a corridor door", key: keyOfSplendorRoomID, target: corridorDoor},
		{name: "anteroom key on the sanctuary gate", key: anteroomKeyID, target: sanctuaryGate},
		{name: "anteroom key on an altar door", key: anteroomKeyID, target: altarSecretDoor},
		{name: "key of darkness on a splendor door", key: keyOfDarknessID, target: splendorDoorA},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, key, gate := bootPaganKeys(t, tc.key, 1, tc.target, 100, tc.target)
			c := srv.Client
			selectDoor(t, c, gate)

			frames := useKey(t, c, key)

			msgs := systemMessages(frames)
			if len(msgs) != 2 {
				t.Fatalf("system messages = %d, want S1_DISAPPEARED then S1_CANNOT_BE_USED", len(msgs))
			}
			assertSystemMessageItem(t, msgs[0], serverpackets.SystemMessageS1Disappeared, tc.key)
			assertSystemMessageItem(t, msgs[1], serverpackets.SystemMessageS1CannotBeUsed, tc.key)
			if got := keysLeft(t, srv, key); got != 0 {
				t.Fatalf("keys left = %d, want the only key spent", got)
			}
			if gate.Opened() {
				t.Fatal("wrong door opened")
			}
			if got := doorStatusUpdates(t, frames); len(got) != 0 {
				t.Fatalf("DoorStatusUpdate doors = %v, want none", got)
			}
		})
	}
}

// TestPaganKeyRefusalsKeepKey refuses a key used with no door selected and
// one used from 150 units away: each answers its message then ActionFailed,
// spends nothing and opens nothing.
func TestPaganKeyRefusalsKeepKey(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		dy        int
		selectIt  bool
		wantMsgID int
	}{
		{name: "no door selected", dy: 100, wantMsgID: serverpackets.SystemMessageInvalidTarget},
		{name: "door at the interaction distance", dy: 150, selectIt: true, wantMsgID: serverpackets.SystemMessageDistTooFarCastingStopped},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, key, gate := bootPaganKeys(t, anteroomKeyID, 1, corridorDoor, tc.dy, corridorDoor)
			c := srv.Client
			if tc.selectIt {
				selectDoor(t, c, gate)
			}

			frames := useKey(t, c, key)

			if len(frames) != 2 {
				t.Fatalf("frames = %d, want the refusal and ActionFailed", len(frames))
			}
			assertStaticSystemMessage(t, frames[0], tc.wantMsgID)
			assertFrameOpcode(t, frames[1], serverpackets.OpcodeActionFailed, "ActionFailed")
			if got := keysLeft(t, srv, key); got != 1 {
				t.Fatalf("keys left = %d, want the key kept", got)
			}
			if gate.Opened() {
				t.Fatal("door opened by a refused key")
			}
		})
	}
}
