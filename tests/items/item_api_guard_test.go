package items

import (
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/itemapi"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/clientwire"
)

const (
	apiCursedID int32 = 32000 // a configured cursed weapon, a copy of the sword
	apiCollarID int32 = 9600  // the catalog's wolf collar
)

// apiCursedBoot boots a character against the shared catalog plus a cursed
// weapon copied from the sword.
func apiCursedBoot(t *testing.T) *gameservertest.Server {
	t.Helper()
	templates := gameservertest.ItemTemplates().All()
	for _, tmpl := range templates {
		if tmpl.ID == apiSwordID {
			cursed := *tmpl
			cursed.ID = apiCursedID
			cursed.Name = "Cursed Sword"
			templates = append(templates, &cursed)
			break
		}
	}
	table, err := entity.NewCursedWeaponTable([]entity.CursedWeapon{{ItemID: apiCursedID}})
	if err != nil {
		t.Fatalf("NewCursedWeaponTable() error: %v", err)
	}
	return gameservertest.Boot(t,
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCursedWeapons(table),
	)
}

// expectRefused posts body to path and fails unless it is refused with 422.
func (a itemAPI) expectRefused(path string, body map[string]any, what string) {
	a.t.Helper()
	if status, _, raw := a.post(path, body); status != http.StatusUnprocessableEntity {
		a.t.Fatalf("%s: status %d %s, want 422", what, status, raw)
	}
}

// TestItemAPIRefusesCursedWeaponChanges: a cursed weapon is not set, taken
// off, put on or destroyed through the API, in the world or out of it, and
// its holder puts no other weapon on. Nothing changes and the client hears
// nothing.
func TestItemAPIRefusesCursedWeaponChanges(t *testing.T) {
	t.Parallel()
	srv := apiCursedBoot(t)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	cursed := srv.GiveItem(t, objID, apiCursedID, 1)
	sword := srv.GiveItem(t, objID, apiSwordID, 1)
	startInWorld(t, c)
	c.Send(encodeUseItem(cursed, false))
	drainUntilQuiet(t, c)
	api := newItemAPI(t, srv, objID)
	if got := api.list().item(t, cursed); got.Location != "PAPERDOLL" {
		t.Fatalf("cursed weapon = %+v, want worn", got)
	}

	worn := target(api.list().item(t, cursed))
	api.expectRefused(itemapi.UnequipPath, worn, "online unequip of the cursed weapon")
	api.expectRefused(itemapi.DeletePath, worn, "online delete of the cursed weapon")
	api.expectRefused(itemapi.EditPath, merge(worn, map[string]any{"enchant": 3}), "online enchant of the cursed weapon")
	holder := onlineLivePlayer(t, srv, objID)
	holder.SetCursedWeapon(apiCursedID, 1)
	api.expectRefused(itemapi.EquipPath, target(api.list().item(t, sword)), "online sword equip of the holder")
	holder.SetCursedWeapon(0, 0)
	c.ExpectNoFrame()
	assertCursedUntouched(t, api.list(), cursed, sword)

	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	srv.AdvanceUntil(t, "player left world", func() bool {
		_, ok := srv.State.Player(objID)
		return !ok
	})
	offline := api.list()
	if offline.Online {
		t.Fatal("still online after the logout")
	}
	worn = target(offline.item(t, cursed))
	api.expectRefused(itemapi.DeletePath, worn, "offline delete of the cursed weapon")
	api.expectRefused(itemapi.UnequipPath, worn, "offline unequip of the cursed weapon")
	api.expectRefused(itemapi.EquipPath, target(offline.item(t, sword)), "offline sword equip of the holder")
	assertCursedUntouched(t, api.list(), cursed, sword)
	stored := false
	for _, inst := range persistedItems(t, srv, objID) {
		stored = stored || (inst.ObjectID == cursed && inst.Equipped())
	}
	if !stored {
		t.Fatal("the cursed weapon's row is no longer worn")
	}
}

func assertCursedUntouched(t *testing.T, got apiItems, cursed, sword int32) {
	t.Helper()
	if it := got.item(t, cursed); it.Location != "PAPERDOLL" || it.Enchant != 0 {
		t.Fatalf("cursed weapon = %+v, want worn at +0", it)
	}
	if it := got.item(t, sword); it.Location != "INVENTORY" {
		t.Fatalf("sword = %+v, want in the inventory", it)
	}
}

// TestItemAPIRefusesSummonedPetCollarDelete: the collar of the pet summoned
// out is not destroyed under it; once the pet is gone it is.
func TestItemAPIRefusesSummonedPetCollarDelete(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	collar := srv.GiveItem(t, objID, apiCollarID, 1)
	startInWorld(t, c)
	pet, err := summon.NewPet(summon.PetConfig{ObjectID: srv.NewObjectID(), ControlItemID: collar, Level: 1})
	if err != nil {
		t.Fatal(err)
	}
	srv.State.AddSummon(objID, pet)
	t.Cleanup(func() { srv.State.RemoveSummon(objID, pet) })
	api := newItemAPI(t, srv, objID)

	api.expectRefused(itemapi.DeletePath, target(api.list().item(t, collar)), "delete of the summoned pet's collar")
	c.ExpectNoFrame()
	if !api.list().has(collar) {
		t.Fatal("the refused delete took the collar")
	}

	srv.State.RemoveSummon(objID, pet)
	if got := api.mustPost(itemapi.DeletePath, target(api.list().item(t, collar))); got.has(collar) {
		t.Fatalf("delete with no pet out = %+v, want the collar gone", got)
	}
}

// TestItemAPILoginWaitsOutAnOfflineEdit selects a character while an edit of
// it out of the world holds it: the selection aborts silently, the edit
// completes, and the next selection restores what the edit wrote.
func TestItemAPILoginWaitsOutAnOfflineEdit(t *testing.T) {
	t.Parallel()
	var armed atomic.Bool
	entered := make(chan struct{})
	release := make(chan struct{})
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		// The refused selection starts the reuse window like any other.
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithCharacterLoadHold(func(int32) {
			if armed.CompareAndSwap(true, false) {
				close(entered)
				<-release
			}
		}))
	objID := srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, apiPotionID, 5)
	api := newItemAPI(t, srv, objID)

	type answer struct {
		status int
		items  apiItems
		raw    string
	}
	done := make(chan answer, 1)
	armed.Store(true)
	go func() {
		status, items, raw := api.post(itemapi.DeletePath, target(apiItem{ObjectID: potion, ItemID: apiPotionID, Count: 5, Location: "INVENTORY"}))
		done <- answer{status, items, raw}
	}()
	<-entered

	srv.Client.Send(clientwire.RequestGameStart(0))
	srv.Client.ExpectNoFrame()
	close(release)
	got := <-done
	if got.status != http.StatusOK || got.items.Online || got.items.has(potion) {
		t.Fatalf("edit held over a login = %d %s, want 200 offline without the potion", got.status, got.raw)
	}

	if _, ok := loginItems(t, startInWorld(t, srv.Client))[potion]; ok {
		t.Fatal("the login restored the potion the edit deleted")
	}
}
