package items

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// itemLogSword is the harness's D-grade "Sword" the item log tests move
// around: a non-stackable item.
const itemLogSword = 30

// itemRecord is one item log line.
type itemRecord struct {
	State    string `json:"message"`
	Owner    string `json:"owner"`
	OwnerID  int32  `json:"owner_id"`
	Item     string `json:"item"`
	ObjectID int32  `json:"object_id"`
	ItemID   int32  `json:"item_id"`
	Location string `json:"location"`
	Count    int    `json:"count"`
	Enchant  int    `json:"enchant"`
}

// itemLogSink is the item log the server writes while the test reads it.
type itemLogSink struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *itemLogSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

// take returns every record written since the last take.
func (s *itemLogSink) take(t *testing.T) []itemRecord {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []itemRecord
	for _, line := range strings.Split(strings.TrimSpace(s.buf.String()), "\n") {
		if line == "" {
			continue
		}
		var r itemRecord
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("item log record %q: %v", line, err)
		}
		out = append(out, r)
	}
	s.buf.Reset()
	return out
}

func assertItemRecords(t *testing.T, sink *itemLogSink, what string, want ...itemRecord) {
	t.Helper()
	if got := sink.take(t); !slices.Equal(got, want) {
		t.Fatalf("item log after %s = %+v, want %+v", what, got, want)
	}
}

// TestItemLogRecordsNonStackableChanges pins LogItems: every add, change and
// removal of a non-stackable item in a player's inventory writes one record
// naming its state, owner, location, count and enchant level, while the same
// moves of a stackable item write nothing. Equip and unequip are changes,
// a drop removes the item (its location already cleared to VOID) and the
// pickup adds it back.
func TestItemLogRecordsNonStackableChanges(t *testing.T) {
	t.Parallel()
	var sink itemLogSink
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1),
		gameservertest.WithItemLog(zerolog.New(&sink)))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	sword := srv.GiveItem(t, objID, itemLogSword, 1)
	inst := mustFindItem(t, srv, objID, sword)
	inst.EnchantLevel = 3
	if err := srv.Items.Update(context.Background(), inst); err != nil {
		t.Fatalf("seed enchant level: %v", err)
	}
	adena := srv.GiveItem(t, objID, item.AdenaID, 100)
	startInWorld(t, c)
	assertItemRecords(t, &sink, "a login with nothing worn")

	named := func(state, location string) itemRecord {
		return itemRecord{
			State: state, Owner: "Newbie", OwnerID: objID, Item: "Sword",
			ObjectID: sword, ItemID: itemLogSword, Location: location, Count: 1, Enchant: 3,
		}
	}

	c.Send(encodeUseItem(sword, false))
	assertFrameOpcode(t, readSkippingEquipNoise(t, c, "equip UserInfo"), serverpackets.OpcodeUserInfo, "equip UserInfo")
	drainUntilQuiet(t, c)
	assertItemRecords(t, &sink, "the equip", named("MODIFIED", "PAPERDOLL"))

	c.Send(encodeRequestUnEquipItem(int32(item.SlotRHand)))
	assertFrameOpcode(t, readSkippingEquipNoise(t, c, "unequip UserInfo"), serverpackets.OpcodeUserInfo, "unequip UserInfo")
	drainUntilQuiet(t, c)
	assertItemRecords(t, &sink, "the unequip", named("MODIFIED", "INVENTORY"))

	drop := func(objectID, count int32, what string) int32 {
		t.Helper()
		c.Send(encodeRequestDropItem(objectID, count, spawnX, spawnY, spawnZ))
		frame := c.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodeDropItem, what+" DropItem")
		r := wire.NewReader(frame[1:])
		r.ReadInt32() // dropper
		groundID := r.ReadInt32()
		drainUntilQuiet(t, c)
		return groundID
	}
	pickUp := func(groundID int32, what string) {
		t.Helper()
		c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, what+" pending-action release")
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeGetItem, what+" GetItem")
		srv.SettlePickup(t, objID)
		drainUntilQuiet(t, c)
	}

	partAdena := drop(adena, 40, "partial adena")
	assertItemRecords(t, &sink, "a partial stackable drop")

	groundSword := drop(sword, 1, "sword")
	assertItemRecords(t, &sink, "the sword drop", named("REMOVED", "VOID"))

	pickUp(groundSword, "sword pickup")
	picked := named("ADDED", "INVENTORY")
	picked.ObjectID = groundSword
	assertItemRecords(t, &sink, "the sword pickup", picked)

	// The rest of the adena leaves as a whole stack, then both ground stacks
	// come back: the first creates a new carried stack, the second merges
	// into it. None of these stackable moves is recorded.
	restAdena := drop(adena, 60, "whole adena")
	assertItemRecords(t, &sink, "a whole stackable drop")
	if got := carriedCount(t, srv, objID, item.AdenaID); got != 0 {
		t.Fatalf("carried adena after dropping it all = %d, want 0", got)
	}
	pickUp(partAdena, "first adena pickup")
	assertItemRecords(t, &sink, "a stackable pickup into an empty slot")
	if got := carriedCount(t, srv, objID, item.AdenaID); got != 40 {
		t.Fatalf("carried adena after the first pickup = %d, want 40", got)
	}
	pickUp(restAdena, "second adena pickup")
	assertItemRecords(t, &sink, "a stackable pickup onto a carried stack")
	if got := carriedCount(t, srv, objID, item.AdenaID); got != 100 {
		t.Fatalf("carried adena after the second pickup = %d, want 100", got)
	}
}

// TestItemLogRecordsWornItemsAtLogin pins the login side of LogItems: putting
// each worn non-stackable item back on the paperdoll as the inventory is
// restored is a change and is recorded, while a carried one is not.
func TestItemLogRecordsWornItemsAtLogin(t *testing.T) {
	t.Parallel()
	var sink itemLogSink
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1),
		gameservertest.WithItemLog(zerolog.New(&sink)))
	objID := srv.SoleObjectID(t)
	sword := srv.GiveItem(t, objID, itemLogSword, 1)
	srv.GiveItem(t, objID, 40, 1)
	if _, err := srv.DB.ExecContext(context.Background(),
		"UPDATE items SET loc = 'PAPERDOLL', loc_data = ? WHERE object_id = ?", itemcontainer.RHand, sword); err != nil {
		t.Fatalf("wear the sword: %v", err)
	}
	startInWorld(t, srv.Client)
	assertItemRecords(t, &sink, "the login", itemRecord{
		State: "MODIFIED", Owner: "Newbie", OwnerID: objID, Item: "Sword",
		ObjectID: sword, ItemID: itemLogSword, Location: "PAPERDOLL", Count: 1,
	})
}
