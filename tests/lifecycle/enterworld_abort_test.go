package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// unloadedTemplateID is an item id the shared catalog has no template for.
// A row carrying it is what a datapack downgrade leaves behind: the restore
// keeps it (it has no unflushed change to look stale by), and the ItemList
// build then refuses to encode around it.
const unloadedTemplateID = 65000

// TestEnterWorldAbortAfterAttachReleasesThePlayer drives the one repeatable
// trigger for a login that fails after the live player is already attached:
// an inventory row whose item template is not loaded. By then the login owns
// an actor queue and a shadow-item registration, so the abort has to detach
// as thoroughly as a logout would.
//
// The shadow item is the destructive half. A leaked registration keeps
// decaying one second per tick for a character who is not online and
// destroys the item at zero, so the test runs the tracker past the weapon's
// whole remaining duration and requires the row to come back untouched.
func TestEnterWorldAbortAfterAttachReleasesThePlayer(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	sword := giveShadowSword(t, srv, objID)

	// Equip it and burn one second, so the mana no longer sits at the
	// template's full duration: every later login then pays the repeated
	// equip penalty, which is what makes the next login's attach visible.
	startInWorld(t, c)
	c.Send(encodeUseItem(sword, false))
	assertFrameOpcode(t, readSkippingEquipNoise(t, c, "equip UserInfo"), serverpackets.OpcodeUserInfo, "equip UserInfo")
	drainUntilQuiet(t, c)
	srv.ShadowItems.Tick()

	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, c, serverpackets.OpcodeCharSelectInfo)
	if got := shadowSwordMana(t, srv, objID, sword); got != 299 {
		t.Fatalf("mana after the first logout = %d, want 299", got)
	}
	// Baseline taken after a clean logout: whatever queues this boot keeps
	// open, the aborted login below must not add one.
	openQueues := srv.OpenActorQueues()

	// The datapack downgrade: one carried row the item table cannot explain.
	broken := seedUnloadedTemplateItem(t, srv, objID)

	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "game start SSQInfo")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "game start CharSelected")
	c.Send(encodeEnterWorld())
	if !c.AwaitClose(5 * time.Second) {
		t.Fatal("aborted login kept the connection open; the ItemList build was expected to fail")
	}

	if _, online := srv.State.Player(objID); online {
		t.Fatal("character stayed in world state after the aborted login")
	}
	if got := srv.OpenActorQueues(); got != openQueues {
		t.Fatalf("open actor queues after the aborted login = %d, want %d: the attached player's queue was never closed", got, openQueues)
	}
	// 299 - 60: the attach that ran before the abort tracked the weapon and
	// charged the repeated-equip penalty, so this run really did enter the
	// post-attach window the fix is about.
	manaAtAbort := shadowSwordMana(t, srv, objID, sword)
	if manaAtAbort != 239 {
		t.Fatalf("mana after the aborted login = %d, want 239 (299 minus the repeated-equip penalty)", manaAtAbort)
	}

	// Well past the weapon's remaining duration: a leaked registration would
	// have decayed it to zero and destroyed the row somewhere in here.
	for i := 0; i < manaAtAbort+10; i++ {
		srv.ShadowItems.Tick()
	}
	if got := shadowSwordMana(t, srv, objID, sword); got != manaAtAbort {
		t.Fatalf("mana after %d offline ticks = %d, want %d unchanged", manaAtAbort+10, got, manaAtAbort)
	}

	// The character is still loginable, and the weapon is still on it.
	deleteItemRow(t, srv, broken)
	c2 := srv.DialClient(t, srv.Account(), 1)
	frames := startInWorld(t, c2)
	entries := readItemListEntries(t, burstFrame(t, frames, serverpackets.OpcodeItemList))
	e := findItemListEntry(entries, sword)
	if e == nil {
		t.Fatal("shadow weapon missing from the ItemList of the login after the abort")
	}
	if e.equipped != 1 {
		t.Fatalf("shadow weapon equipped flag = %d, want 1", e.equipped)
	}
}

// shadowSwordMana returns the persisted mana of ownerID's item objectID,
// flushing the lazy item persistence first so the row reflects every decay
// applied in memory.
func shadowSwordMana(t *testing.T, srv *gameservertest.Server, ownerID, objectID int32) int {
	t.Helper()
	srv.FlushItems(t)
	for _, inst := range persistedItems(t, srv, ownerID) {
		if inst.ObjectID == objectID {
			return inst.ManaLeft
		}
	}
	t.Fatalf("no items row for object %d", objectID)
	return 0
}

// seedUnloadedTemplateItem persists one carried row whose template the item
// table does not hold, and returns its object id.
func seedUnloadedTemplateItem(t *testing.T, srv *gameservertest.Server, ownerID int32) int32 {
	t.Helper()
	objectID := srv.NewObjectID()
	inst := item.Instance{
		ObjectID:   objectID,
		TemplateID: unloadedTemplateID,
		OwnerID:    ownerID,
		Count:      1,
		Location:   item.LocationInventory,
		ManaLeft:   -1,
	}
	if err := srv.Items.Create(context.Background(), ownerID, inst); err != nil {
		t.Fatalf("seed unloaded-template item: %v", err)
	}
	return objectID
}

func deleteItemRow(t *testing.T, srv *gameservertest.Server, objectID int32) {
	t.Helper()
	if _, err := srv.DB.ExecContext(context.Background(), `DELETE FROM items WHERE object_id = ?`, objectID); err != nil {
		t.Fatalf("delete item row %d: %v", objectID, err)
	}
}
