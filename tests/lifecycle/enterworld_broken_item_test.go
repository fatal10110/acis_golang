package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// unloadedTemplateID is an item id the catalog has no template for. A
// carried row with one is what a datapack downgrade leaves behind.
const unloadedTemplateID = 65000

// TestBrokenItemTemplateCostsOneItemNotTheLogin covers the datapack-downgrade
// row #2420 names as the repeatable trigger the EnterWorld window has to be
// safe against. The reference drops such a row during the inventory restore
// and logs the player in anyway, so the login must not abort on it.
//
// Aborting is not merely a worse experience: the client retries, and every
// attempt that reaches attachLivePlayer charges an equipped shadow item the
// repeated-equip mana penalty and persists it, so a retry loop drains the
// item to zero and the tracker then destroys it. The per-login mana asserted
// here is what pins that down: one penalty per login the player actually
// completes, which is what the reference charges too.
func TestBrokenItemTemplateCostsOneItemNotTheLogin(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithCapturedLog(),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	sword := giveShadowSword(t, srv, objID)

	// Equip and burn one second, so the weapon's mana leaves the template's
	// full duration and every later login pays the repeated-equip penalty.
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

	broken := seedUnloadedTemplateItem(t, srv, objID)

	// Two logins in a row with the row still broken: both complete, and each
	// costs one repeated-equip penalty — not one per attempt.
	for _, step := range []struct{ login, wantMana int }{{1, 239}, {2, 179}} {
		frames := startInWorld(t, c)
		entries := readItemListEntries(t, burstFrame(t, frames, serverpackets.OpcodeItemList))
		if findItemListEntry(entries, broken) != nil {
			t.Fatalf("login %d: the row with no loaded template reached the client's ItemList", step.login)
		}
		e := findItemListEntry(entries, sword)
		if e == nil {
			t.Fatalf("login %d: shadow weapon missing from the ItemList", step.login)
		}
		if e.equipped != 1 {
			t.Fatalf("login %d: shadow weapon equipped flag = %d, want 1", step.login, e.equipped)
		}
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
		readUntilOpcode(t, c, serverpackets.OpcodeCharSelectInfo)
		if got := shadowSwordMana(t, srv, objID, sword); got != step.wantMana {
			t.Fatalf("mana after login %d = %d, want %d (one repeated-equip penalty per completed login)", step.login, got, step.wantMana)
		}
	}

	// The row is skipped by the restore, not deleted: the item comes back
	// when its template does.
	if !persistedRowExists(t, srv, objID, broken) {
		t.Fatal("the row with no loaded template was deleted; the restore must only skip it")
	}
	// The other drop in this filter is already greppable; this one has to be
	// too, or an inventory quietly loses an item on every login.
	if !strings.Contains(srv.LogText(), "skipped item rows with no loaded template") {
		t.Fatal("no log line for the skipped template-less row")
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
