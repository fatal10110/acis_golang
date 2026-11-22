package items

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/itemapi"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	apiSwordID  int32 = 30
	apiPotionID int32 = 20
	apiShadowID int32 = 7884 // five minutes of mana
)

type apiItem struct {
	ObjectID  int32  `json:"object_id"`
	ItemID    int32  `json:"item_id"`
	Count     int    `json:"count"`
	Enchant   int    `json:"enchant"`
	Location  string `json:"location"`
	Slot      int    `json:"slot"`
	Augmented bool   `json:"augmented"`
	Mana      int    `json:"mana"`
}

type apiItems struct {
	CharacterID int32     `json:"character_id"`
	Name        string    `json:"name"`
	Online      bool      `json:"online"`
	Items       []apiItem `json:"items"`
}

func (l apiItems) item(t *testing.T, objectID int32) apiItem {
	t.Helper()
	for _, it := range l.Items {
		if it.ObjectID == objectID {
			return it
		}
	}
	t.Fatalf("no item %d in %+v", objectID, l.Items)
	return apiItem{}
}

func (l apiItems) has(objectID int32) bool {
	for _, it := range l.Items {
		if it.ObjectID == objectID {
			return true
		}
	}
	return false
}

// newIDs returns the object ids of item itemID in l that are not in old.
func (l apiItems) newIDs(old apiItems, itemID int32) []int32 {
	var ids []int32
	for _, it := range l.Items {
		if it.ItemID == itemID && !old.has(it.ObjectID) {
			ids = append(ids, it.ObjectID)
		}
	}
	return ids
}

// itemAPI drives the item API as the admin panel would.
type itemAPI struct {
	t   *testing.T
	url string
	id  int32
}

func newItemAPI(t *testing.T, srv *gameservertest.Server, characterID int32) itemAPI {
	t.Helper()
	ts := httptest.NewServer(srv.ItemAPI())
	t.Cleanup(ts.Close)
	return itemAPI{t: t, url: ts.URL, id: characterID}
}

func (a itemAPI) list() apiItems {
	a.t.Helper()
	resp, err := http.Get(a.url + itemapi.Path + "?character_id=" + jsonString(a.t, a.id))
	if err != nil {
		a.t.Fatalf("GET %s: %v", itemapi.Path, err)
	}
	defer resp.Body.Close()
	var out apiItems
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || resp.StatusCode != http.StatusOK {
		a.t.Fatalf("GET %s: status %d err %v", itemapi.Path, resp.StatusCode, err)
	}
	return out
}

// post sends body, with this character's id, to path and returns the status
// and the decoded answer.
func (a itemAPI) post(path string, body map[string]any) (int, apiItems, string) {
	a.t.Helper()
	body["character_id"] = a.id
	raw, err := json.Marshal(body)
	if err != nil {
		a.t.Fatal(err)
	}
	resp, err := http.Post(a.url+path, "application/json", bytes.NewReader(raw))
	if err != nil {
		a.t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		a.t.Fatal(err)
	}
	var out apiItems
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
			a.t.Fatalf("POST %s: %v", path, err)
		}
	}
	return resp.StatusCode, out, buf.String()
}

func (a itemAPI) mustPost(path string, body map[string]any) apiItems {
	a.t.Helper()
	status, out, raw := a.post(path, body)
	if status != http.StatusOK {
		a.t.Fatalf("POST %s %v: status %d %s", path, body, status, raw)
	}
	return out
}

// target is a request body naming it as the caller last listed it.
func target(it apiItem) map[string]any {
	return map[string]any{
		"object_id": it.ObjectID,
		"expect":    map[string]any{"item_id": it.ItemID, "count": it.Count, "enchant": it.Enchant, "location": it.Location},
	}
}

func jsonString(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// loginItem is one ItemList row as the client reads it.
type loginItem struct {
	itemID   int32
	count    int32
	equipped uint16
	enchant  uint16
	mana     int32
}

// loginItems reads the ItemList among frames by object id.
func loginItems(t *testing.T, frames [][]byte) map[int32]loginItem {
	t.Helper()
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeItemList {
			continue
		}
		r := wire.NewReader(frame[1:])
		r.ReadUint16() // showWindow
		n := r.ReadUint16()
		out := make(map[int32]loginItem, n)
		for range n {
			r.ReadUint16() // item category
			objID := r.ReadInt32()
			var e loginItem
			e.itemID = r.ReadInt32()
			e.count = r.ReadInt32()
			r.ReadUint16() // subCategory
			r.ReadUint16() // CustomType1
			e.equipped = r.ReadUint16()
			r.ReadInt32() // body slot
			e.enchant = r.ReadUint16()
			r.ReadUint16() // CustomType2
			r.ReadInt32()  // augmentation
			e.mana = r.ReadInt32()
			out[objID] = e
		}
		if err := r.Err(); err != nil {
			t.Fatalf("read ItemList: %v", err)
		}
		return out
	}
	t.Fatal("no ItemList in the EnterWorld burst")
	return nil
}

// TestItemAPIOfflineEditsReachTheLogin runs every operation on a character
// out of the world: each answer and each items row carries it at once, and
// the next login's ItemList shows the result, paperdoll included.
func TestItemAPIOfflineEditsReachTheLogin(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	objID := srv.SoleObjectID(t)
	sword := srv.GiveItem(t, objID, apiSwordID, 1)
	spare := srv.GiveItem(t, objID, apiSwordID, 1)
	potion := srv.GiveItem(t, objID, apiPotionID, 5)
	api := newItemAPI(t, srv, objID)

	start := api.list()
	if start.Online || start.Name != "Newbie" || len(start.Items) != 3 {
		t.Fatalf("list = %+v, want Newbie offline with 3 items", start)
	}

	// A stackable joins its stack; a non-stackable arrives one instance per
	// unit at the enchant asked for; a shadow item starts with its duration.
	got := api.mustPost(itemapi.AddPath, map[string]any{"item_id": apiPotionID, "count": 3})
	if p := got.item(t, potion); p.Count != 8 {
		t.Fatalf("potion after add = %+v, want 8 on the held stack", p)
	}
	before := got
	got = api.mustPost(itemapi.AddPath, map[string]any{"item_id": apiSwordID, "count": 2, "enchant": 3})
	added := got.newIDs(before, apiSwordID)
	if len(added) != 2 {
		t.Fatalf("add 2 swords created %v, want 2 instances", added)
	}
	for _, id := range added {
		if s := got.item(t, id); s.Count != 1 || s.Enchant != 3 || s.Location != "INVENTORY" {
			t.Fatalf("added sword = %+v, want x1 +3 in the inventory", s)
		}
	}
	before = got
	got = api.mustPost(itemapi.AddPath, map[string]any{"item_id": apiShadowID, "count": 1})
	shadow := got.newIDs(before, apiShadowID)
	if len(shadow) != 1 || got.item(t, shadow[0]).Mana != 300 {
		t.Fatalf("shadow item = %v in %+v, want one with 300 s of mana", shadow, got.Items)
	}

	got = api.mustPost(itemapi.EditPath, merge(target(got.item(t, potion)), map[string]any{"count": 10}))
	got = api.mustPost(itemapi.EditPath, merge(target(got.item(t, sword)), map[string]any{"enchant": 7}))
	got = api.mustPost(itemapi.DeletePath, target(got.item(t, spare)))
	if got.has(spare) {
		t.Fatalf("spare sword still listed after delete: %+v", got.Items)
	}
	// Equipping another sword sends the worn one back; unequip takes it off.
	got = api.mustPost(itemapi.EquipPath, target(got.item(t, sword)))
	got = api.mustPost(itemapi.EquipPath, target(got.item(t, added[0])))
	if s := got.item(t, sword); s.Location != "INVENTORY" {
		t.Fatalf("sword after another went on = %+v, want back in the inventory", s)
	}
	got = api.mustPost(itemapi.UnequipPath, target(got.item(t, added[0])))
	if s := got.item(t, added[0]); s.Location != "INVENTORY" || s.Slot != -1 {
		t.Fatalf("sword after unequip = %+v, want in the inventory", s)
	}
	got = api.mustPost(itemapi.EquipPath, target(got.item(t, sword)))
	if s := got.item(t, sword); s.Location != "PAPERDOLL" || s.Slot != itemcontainer.RHand || s.Enchant != 7 {
		t.Fatalf("sword = %+v, want worn in the right hand at +7", s)
	}
	if got.Online {
		t.Fatal("answers report the character online")
	}

	rows := map[int32]*item.Instance{}
	for _, inst := range persistedItems(t, srv, objID) {
		rows[inst.ObjectID] = inst
	}
	if _, ok := rows[spare]; ok {
		t.Fatal("deleted sword still has its row")
	}
	if r := rows[potion]; r == nil || r.Count != 10 {
		t.Fatalf("potion row = %+v, want 10", r)
	}
	if r := rows[sword]; r == nil || r.Location != item.LocationPaperdoll || r.LocationData != itemcontainer.RHand || r.EnchantLevel != 7 {
		t.Fatalf("sword row = %+v, want worn in the right hand at +7", r)
	}
	if r := rows[shadow[0]]; r == nil || r.ManaLeft != 300 {
		t.Fatalf("shadow row = %+v, want 300 s of mana", r)
	}
	for _, id := range added {
		if r := rows[id]; r == nil || r.EnchantLevel != 3 || r.Location != item.LocationInventory {
			t.Fatalf("added sword row = %+v, want +3 in the inventory", r)
		}
	}

	login := loginItems(t, startInWorld(t, srv.Client))
	if len(login) != len(got.Items) {
		t.Fatalf("login ItemList has %d items, want the %d the API answered", len(login), len(got.Items))
	}
	if e := login[sword]; e.itemID != apiSwordID || e.equipped != 1 || e.enchant != 7 {
		t.Fatalf("login sword = %+v, want worn at +7", e)
	}
	if e := login[potion]; e.count != 10 {
		t.Fatalf("login potion = %+v, want 10", e)
	}
	if e, ok := login[added[0]]; !ok || e.equipped != 0 || e.enchant != 3 {
		t.Fatalf("login added sword = %+v, want +3 not worn", e)
	}
	if online := api.list(); !online.Online || len(online.Items) != len(got.Items) {
		t.Fatalf("list after login = %+v, want the same items online", online)
	}
}

func merge(a, b map[string]any) map[string]any {
	for k, v := range b {
		a[k] = v
	}
	return a
}

// TestItemAPIOfflineBowEquipPullsArrows equips a bow on a character out of
// the world under the player's own equip rules: it takes the held arrows of
// its grade into the left hand, and unequipping it releases them.
func TestItemAPIOfflineBowEquipPullsArrows(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithItemTemplates(handSlotCatalog()),
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1))
	objID := srv.SoleObjectID(t)
	bow := srv.GiveItem(t, objID, fixtureBowID, 1)
	arrows := srv.GiveItem(t, objID, fixtureArrowID, 10)
	api := newItemAPI(t, srv, objID)

	got := api.mustPost(itemapi.EquipPath, target(api.list().item(t, bow)))
	if a := got.item(t, arrows); a.Location != "PAPERDOLL" || a.Slot != itemcontainer.LHand {
		t.Fatalf("arrows after the bow went on = %+v, want in the left hand", a)
	}
	assertWorn(t, srv, objID, arrows, itemcontainer.LHand)

	got = api.mustPost(itemapi.UnequipPath, target(got.item(t, bow)))
	if a := got.item(t, arrows); a.Location != "INVENTORY" {
		t.Fatalf("arrows after the bow came off = %+v, want in the inventory", a)
	}
	assertWorn(t, srv, objID, arrows, -1)
	assertWorn(t, srv, objID, bow, -1)
}

// TestItemAPIOnlineEditsReachTheClient runs every operation on a character
// in the world: the client hears of each through InventoryUpdate, a paperdoll
// change through UserInfo too, and the item persistence writes the rows.
func TestItemAPIOnlineEditsReachTheClient(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	sword := srv.GiveItem(t, objID, apiSwordID, 1)
	potion := srv.GiveItem(t, objID, apiPotionID, 5)
	startInWorld(t, c)
	api := newItemAPI(t, srv, objID)

	requireUserInfo := func(what string) {
		t.Helper()
		for _, f := range collectUntilQuiet(t, c) {
			if f[0] == serverpackets.OpcodeUserInfo {
				return
			}
		}
		t.Fatalf("%s sent no UserInfo", what)
	}
	row := func(objectID int32) *item.Instance {
		t.Helper()
		srv.FlushItems(t)
		return mustFindItem(t, srv, objID, objectID)
	}

	got := api.mustPost(itemapi.EquipPath, target(api.list().item(t, sword)))
	if !got.Online {
		t.Fatal("answer reports the character offline")
	}
	requireUserInfo("equip")
	if e := inventoryUpdateAfterTick(t, srv, c)[sword]; e.equipped != 1 {
		t.Fatalf("InventoryUpdate after equip = %+v, want the sword worn", e)
	}
	assertWorn(t, srv, objID, sword, itemcontainer.RHand)

	got = api.mustPost(itemapi.EditPath, merge(target(got.item(t, sword)), map[string]any{"enchant": 5}))
	requireUserInfo("enchant of a worn sword")
	if e := inventoryUpdateAfterTick(t, srv, c)[sword]; e.enchant != 5 {
		t.Fatalf("InventoryUpdate after enchant = %+v, want +5", e)
	}
	if r := row(sword); r.EnchantLevel != 5 {
		t.Fatalf("sword row enchant = %d, want 5", r.EnchantLevel)
	}

	got = api.mustPost(itemapi.EditPath, merge(target(got.item(t, potion)), map[string]any{"count": 9}))
	if e := inventoryUpdateAfterTick(t, srv, c)[potion]; e.count != 9 {
		t.Fatalf("InventoryUpdate after count edit = %+v, want 9", e)
	}
	if r := row(potion); r.Count != 9 {
		t.Fatalf("potion row count = %d, want 9", r.Count)
	}

	before := got
	got = api.mustPost(itemapi.AddPath, map[string]any{"item_id": apiSwordID, "count": 2, "enchant": 1})
	added := got.newIDs(before, apiSwordID)
	if len(added) != 2 {
		t.Fatalf("add created %v, want 2 swords", added)
	}
	// Each new sword is announced as added, then at its enchant.
	srv.InventoryUpdates.Tick()
	var entries []inventoryEntry
	for _, f := range collectUntilQuiet(t, c) {
		if f[0] == serverpackets.OpcodeInventoryUpdate {
			entries = append(entries, readInventoryUpdateEntries(t, f)...)
		}
	}
	for _, id := range added {
		var states []uint16
		var last inventoryEntry
		for _, e := range entries {
			if e.objID == id {
				states, last = append(states, e.state), e
			}
		}
		if len(states) == 0 || states[0] != 1 || last.enchant != 1 {
			t.Fatalf("InventoryUpdate for added sword %d: states %v, last %+v, want added first and +1 last", id, states, last)
		}
		if r := row(id); r.EnchantLevel != 1 || r.Location != item.LocationInventory {
			t.Fatalf("added sword row = %+v, want +1 in the inventory", r)
		}
	}

	got = api.mustPost(itemapi.DeletePath, target(got.item(t, sword)))
	requireUserInfo("delete of a worn sword")
	if e := inventoryUpdateAfterTick(t, srv, c)[sword]; e.state != 3 {
		t.Fatalf("InventoryUpdate after delete = %+v, want removed", e)
	}
	srv.FlushItems(t)
	for _, inst := range persistedItems(t, srv, objID) {
		if inst.ObjectID == sword {
			t.Fatal("deleted sword still has its row")
		}
	}

	got = api.mustPost(itemapi.EquipPath, target(got.item(t, added[0])))
	collectUntilQuiet(t, c)
	inventoryUpdateAfterTick(t, srv, c)
	got = api.mustPost(itemapi.UnequipPath, target(got.item(t, added[0])))
	requireUserInfo("unequip")
	if e := inventoryUpdateAfterTick(t, srv, c)[added[0]]; e.equipped != 0 {
		t.Fatalf("InventoryUpdate after unequip = %+v, want not worn", e)
	}
	assertWorn(t, srv, objID, added[0], -1)
	if s := got.item(t, added[0]); s.Location != "INVENTORY" {
		t.Fatalf("answer after unequip = %+v, want in the inventory", s)
	}
}

// TestItemAPIStaleExpectConflicts changes nothing when the caller's view of
// the item is out of date, in or out of the world, and answers 409.
func TestItemAPIStaleExpectConflicts(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	objID := srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, apiPotionID, 5)
	api := newItemAPI(t, srv, objID)

	stale := api.list().item(t, potion)
	stale.Count = 4
	for _, online := range []bool{false, true} {
		if online {
			startInWorld(t, srv.Client)
		}
		for _, path := range []string{itemapi.EditPath, itemapi.DeletePath} {
			body := target(stale)
			if path == itemapi.EditPath {
				body["count"] = 1
			}
			if status, _, raw := api.post(path, body); status != http.StatusConflict {
				t.Fatalf("online=%v %s with a stale count: status %d %s, want 409", online, path, status, raw)
			}
		}
		if r := mustFindItem(t, srv, objID, potion); r.Count != 5 {
			t.Fatalf("online=%v potion row = %d, want 5 untouched", online, r.Count)
		}
		if p := api.list().item(t, potion); p.Count != 5 {
			t.Fatalf("online=%v potion = %+v, want 5 untouched", online, p)
		}
	}
}

// TestItemAPIEditWaitsOutALogin refuses an offline edit while a login is
// restoring the character, so the login never loads rows the edit is about
// to change; once the character is in the world the edit goes through its
// session instead.
func TestItemAPIEditWaitsOutALogin(t *testing.T) {
	t.Parallel()
	var api itemAPI
	var potion int32
	during := make(chan int, 1)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSelectionHold(func(int32) {
			status, _, _ := api.post(itemapi.DeletePath, target(apiItem{ObjectID: potion, ItemID: apiPotionID, Count: 5, Location: "INVENTORY"}))
			during <- status
		}))
	objID := srv.SoleObjectID(t)
	potion = srv.GiveItem(t, objID, apiPotionID, 5)
	api = newItemAPI(t, srv, objID)

	login := loginItems(t, startInWorld(t, srv.Client))
	if status := <-during; status != http.StatusServiceUnavailable {
		t.Fatalf("edit during the login: status %d, want 503", status)
	}
	if e := login[potion]; e.count != 5 {
		t.Fatalf("login potion = %+v, want 5", e)
	}
	got := api.mustPost(itemapi.DeletePath, target(api.list().item(t, potion)))
	if !got.Online || got.has(potion) {
		t.Fatalf("delete after the login = %+v, want the potion gone online", got)
	}
}

// TestItemAPIOfflineSeesHeldItemWrites lists and edits a character whose
// last session left an item change unwritten: a stack destroyed in the
// world keeps its row until the persistence tick, and the API, like the next
// login, treats the held change as the truth.
func TestItemAPIOfflineSeesHeldItemWrites(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	objID := srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, apiPotionID, 5)
	sword := srv.GiveItem(t, objID, apiSwordID, 1)
	startInWorld(t, srv.Client)
	srv.Client.Send(encodeRequestDestroyItem(potion, 5))
	collectUntilQuiet(t, srv.Client)
	if err := srv.Client.Close(); err != nil {
		t.Fatal(err)
	}
	srv.AdvanceUntil(t, "player left world", func() bool {
		_, ok := srv.State.Player(objID)
		return !ok
	})
	held := false
	for _, inst := range persistedItems(t, srv, objID) {
		held = held || inst.ObjectID == potion
	}
	if !held {
		t.Fatal("the destroyed stack's row is gone before the tick: nothing held to resolve")
	}

	api := newItemAPI(t, srv, objID)
	if got := api.list(); got.Online || got.has(potion) {
		t.Fatalf("list = %+v, want offline without the destroyed stack", got)
	}
	got := api.mustPost(itemapi.EditPath, merge(target(api.list().item(t, sword)), map[string]any{"enchant": 2}))
	if got.has(potion) {
		t.Fatalf("edit answer = %+v, want no destroyed stack", got)
	}

	c := srv.DialClient(t, srv.Account(), 1)
	login := loginItems(t, startInWorld(t, c))
	if _, ok := login[potion]; ok {
		t.Fatal("the destroyed stack came back at login")
	}
	if e := login[sword]; e.enchant != 2 {
		t.Fatalf("login sword = %+v, want +2", e)
	}
}

// TestItemAPIRefusesBadRequests answers a request that can never succeed
// with 400, and one naming nothing stored with 404, changing nothing.
func TestItemAPIRefusesBadRequests(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	objID := srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, apiPotionID, 5)
	api := newItemAPI(t, srv, objID)
	held := api.list().item(t, potion)

	for _, tc := range []struct {
		name string
		path string
		body map[string]any
		want int
	}{
		{"stack past int32", itemapi.AddPath, map[string]any{"item_id": apiPotionID, "count": 2147483643}, http.StatusBadRequest},
		{"too many instances", itemapi.AddPath, map[string]any{"item_id": apiSwordID, "count": 101}, http.StatusBadRequest},
		{"enchanted stackable", itemapi.AddPath, map[string]any{"item_id": apiPotionID, "count": 1, "enchant": 1}, http.StatusBadRequest},
		{"unknown item", itemapi.AddPath, map[string]any{"item_id": 999999, "count": 1}, http.StatusBadRequest},
		{"zero count", itemapi.AddPath, map[string]any{"item_id": apiPotionID, "count": 0}, http.StatusBadRequest},
		{"no expect", itemapi.DeletePath, map[string]any{"object_id": potion}, http.StatusBadRequest},
		{"count zero", itemapi.EditPath, merge(target(held), map[string]any{"count": 0}), http.StatusBadRequest},
		{"no change", itemapi.EditPath, target(held), http.StatusBadRequest},
		{"not equipment", itemapi.EquipPath, target(held), http.StatusBadRequest},
		{"not worn", itemapi.UnequipPath, target(held), http.StatusBadRequest},
		{"unknown object", itemapi.DeletePath, merge(target(held), map[string]any{"object_id": potion + 1000}), http.StatusNotFound},
	} {
		if status, _, raw := api.post(tc.path, tc.body); status != tc.want {
			t.Fatalf("%s: status %d %s, want %d", tc.name, status, raw, tc.want)
		}
	}
	nobody := itemAPI{t: t, url: api.url, id: objID + 1000}
	if status, _, raw := nobody.post(itemapi.AddPath, map[string]any{"item_id": apiPotionID, "count": 1}); status != http.StatusNotFound {
		t.Fatalf("add to no character: status %d %s, want 404", status, raw)
	}
	if got := api.list(); len(got.Items) != 1 || got.item(t, potion).Count != 5 {
		t.Fatalf("items after refused requests = %+v, want the potion alone at 5", got.Items)
	}
}
