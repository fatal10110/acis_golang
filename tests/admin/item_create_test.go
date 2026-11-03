package admin

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/armorset"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

const (
	ancientAdenaID int32 = 5575
	// Mithril Heavy Set: chest, legs, head and shield.
	mithrilShieldID int32 = 628
)

// shippedSetOrder is the shipped armor sets' chest ids in the order the
// reference server lists them: its chest-keyed hash map filled in file
// order, as a JDK 21 HashMap iterates it.
var shippedSetOrder = []int32{
	394, 395, 397, 398, 400, 401, 23, 7851, 7854, 7857, 436, 437, 7861, 439, 7864, 441, 58, 442, 7867, 60,
	2376, 1101, 2381, 2382, 2383, 2384, 2385, 2390, 2391, 2392, 2393, 2394, 2395, 2397, 2398, 2399, 352, 2400,
	354, 356, 357, 6373, 2406, 358, 2407, 2408, 2409, 6379, 365, 6383, 374,
}

// bootItemAdmin boots the GM "Admin" with the shipped item templates, armor
// sets and item creation pages, and enters the world. It returns the
// server, the armor sets and the GM's id.
func bootItemAdmin(t *testing.T) (*gameservertest.Server, *armorset.Table, int32) {
	t.Helper()
	dir := datapack.Require(t)
	items, err := shippedItemTemplates()
	if err != nil {
		t.Fatalf("load item templates: %v", err)
	}
	sets, err := xmldata.LoadArmorSets(filepath.Join(dir, "data", "xml", "armorSets.xml"))
	if err != nil {
		t.Fatalf("load armor sets: %v", err)
	}
	srv, gmID := bootAdmin(t, adminLevel,
		gameservertest.WithItemTemplates(items),
		gameservertest.WithArmorSets(sets),
		gameservertest.WithHTMLPages(shippedAdminPages(t, "itemcreation.htm", "itemsets.htm", "gmshops.htm")),
	)
	enterWorld(t, srv.Client)
	return srv, sets, gmID
}

// itemNotice returns a SystemMessage frame's id, item-name parameter and
// item-number parameter (1 without one), false when it names no item.
func itemNotice(frame []byte) (id, itemID, count int32, ok bool) {
	if frame[0] != serverpackets.OpcodeSystemMessage {
		return 0, 0, 0, false
	}
	r := wire.NewReader(frame[1:])
	id, n := r.ReadInt32(), r.ReadInt32()
	if n < 1 || n > 2 || r.ReadInt32() != serverpackets.SystemMessageParamItemName {
		return 0, 0, 0, false
	}
	itemID, count = r.ReadInt32(), 1
	if n == 2 {
		if r.ReadInt32() != serverpackets.SystemMessageParamItemNumber {
			return 0, 0, 0, false
		}
		count = r.ReadInt32()
	}
	return id, itemID, count, r.Err() == nil
}

// assertPickedUp requires frames to hold, before the creation page last,
// the picked-up notice of count units of itemID, and no plain text.
func assertPickedUp(t *testing.T, frames [][]byte, itemID, count int32) {
	t.Helper()
	assertPage(t, frames[len(frames)-1:], "Item Creation Menu")
	want := int32(serverpackets.SystemMessageYouPickedUpS1)
	if count > 1 {
		want = serverpackets.SystemMessageYouPickedUpS2S1
	}
	found := false
	for _, f := range frames[:len(frames)-1] {
		if id, gotItem, gotCount, ok := itemNotice(f); ok && id == want && gotItem == itemID && gotCount == count {
			found = true
		}
		if f[0] == serverpackets.OpcodeSystemMessage && !found {
			t.Fatalf("frames = %x, want the picked-up notice of %d %d first", testsupport.FrameOpcodes(frames), count, itemID)
		}
	}
	if !found {
		t.Fatalf("frames = %x, want the picked-up notice of %d %d", testsupport.FrameOpcodes(frames), count, itemID)
	}
}

// TestAdminItemCreatesForTheGM clicks the main panel's Item button, then
// creates items for the GM itself through the creation page's links: an
// id with and without a count, and each coin by name. Unknown items,
// unknown coins and bad numbers are refused as specified.
func TestAdminItemCreatesForTheGM(t *testing.T) {
	t.Parallel()
	srv, _, gmID := bootItemAdmin(t)
	c := srv.Client

	main := htmlBody(t, exchange(t, c, encodeBypass("admin_admin"))[0])
	if !strings.Contains(main, `action="bypass -h admin_item"`) {
		t.Fatalf("main panel has no Item button: %q", main)
	}
	assertPage(t, exchange(t, c, encodeBypass("admin_item")), "Item Creation Menu")

	assertPickedUp(t, exchange(t, c, encodeBypass("admin_item 57 100 0")), item.AdenaID, 100)
	assertPickedUp(t, exchange(t, c, encodeBypass("admin_item 57")), item.AdenaID, 1)
	assertPickedUp(t, exchange(t, c, encodeBypass("admin_item coin Ancient 5 ")), ancientAdenaID, 5)
	assertPickedUp(t, exchange(t, c, encodeBuildCmd("item coin adena")), item.AdenaID, 1)
	if got := srv.PlayerItemCount(t, gmID, item.AdenaID); got != 102 {
		t.Fatalf("adena = %d, want 102", got)
	}
	if got := srv.PlayerItemCount(t, gmID, ancientAdenaID); got != 5 {
		t.Fatalf("ancient adena = %d, want 5", got)
	}

	frames := exchange(t, c, encodeBypass("admin_item 999999"))
	assertTexts(t, frames[:1], "This item doesn't exist.")
	assertPage(t, frames[1:], "Item Creation Menu")

	const coinUsage = "Usage: //item coin name [amount] [radius]"
	assertTexts(t, exchange(t, c, encodeBypass("admin_item coin gold 5")), coinUsage)
	for _, cmd := range []string{"admin_item coin", "admin_item coin adena x", "admin_item coin adena 1 99999999999"} {
		frames := exchange(t, c, encodeBypass(cmd))
		assertTexts(t, frames[:1], coinUsage)
		assertPage(t, frames[1:], "Item Creation Menu")
	}

	// A bad count or radius, or an id past the int range, answers nothing.
	for _, cmd := range []string{"admin_item 57 x", "admin_item 57 1 x", "admin_item 99999999999", "admin_item other"} {
		if frames := exchange(t, c, encodeBypass(cmd)); len(frames) != 0 {
			t.Fatalf("%s frames = %x, want none", cmd, testsupport.FrameOpcodes(frames))
		}
	}
	if got := srv.PlayerItemCount(t, gmID, item.AdenaID); got != 102 {
		t.Fatalf("adena after refusals = %d, want 102", got)
	}
}

// TestAdminItemSetListsAndGives clicks the GM shop's "Create armor set":
// every shipped set is listed, two to a row as the reference lays them
// out, in the reference order. A listed set's link gives one of each of
// its pieces and its shield with no notice, then lists the sets again; a
// chest no set has is refused without the list.
func TestAdminItemSetListsAndGives(t *testing.T) {
	t.Parallel()
	srv, sets, gmID := bootItemAdmin(t)
	c := srv.Client

	gmShop := htmlBody(t, exchange(t, c, encodeBypass("admin_buy"))[0])
	if !strings.Contains(gmShop, `action="bypass -h admin_item set"`) {
		t.Fatalf("GM shop page has no Create armor set link: %q", gmShop)
	}
	var cells strings.Builder
	for i, chest := range shippedSetOrder {
		set, ok := sets.FindByChest(chest)
		if !ok {
			t.Fatalf("shipped set with chest %d missing", chest)
		}
		cell := fmt.Sprintf(`<td><a action="bypass -h admin_item set %d">%s</a></td>`, chest, set.Name)
		if i%2 == 0 {
			cell = "<tr>" + cell + "</tr>"
		}
		cells.WriteString(cell)
	}
	if sets.Len() != len(shippedSetOrder) {
		t.Fatalf("shipped sets = %d, want %d", sets.Len(), len(shippedSetOrder))
	}
	page, ok := shippedAdminPages(t, "itemsets.htm")["admin/itemsets.htm"]
	if !ok {
		t.Fatal("itemsets.htm not shipped")
	}
	wantList := strings.ReplaceAll(strings.ReplaceAll(page, "\r\n", "\n"), "%sets%", cells.String())
	assertList := func(frames [][]byte) {
		t.Helper()
		if len(frames) != 1 {
			t.Fatalf("frames = %x, want the set list", testsupport.FrameOpcodes(frames))
		}
		if got := strings.TrimSuffix(htmlBody(t, frames[0]), "\n"); got != wantList {
			t.Fatalf("set list = %q\nwant %q", got, wantList)
		}
	}
	assertList(exchange(t, c, encodeBypass("admin_item set")))

	assertList(exchange(t, c, encodeBypass("admin_item set 58")))
	for _, id := range []int32{58, 59, 47, mithrilShieldID} {
		if got := srv.PlayerItemCount(t, gmID, id); got != 1 {
			t.Fatalf("Mithril Heavy Set piece %d = %d, want 1", id, got)
		}
	}
	srv.FlushItems(t)
	var stored int
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM items WHERE owner_id = ? AND item_id IN (58, 59, 47, 628) AND loc = 'INVENTORY'", gmID).Scan(&stored); err != nil || stored != 4 {
		t.Fatalf("stored set pieces = %d (%v), want 4", stored, err)
	}

	assertTexts(t, exchange(t, c, encodeBypass("admin_item set 1")), "This chest has no set.")
	frames := exchange(t, c, encodeBypass("admin_item set x"))
	assertTexts(t, frames[:1], "Usage: //item set [chestId]")
	assertList(frames[1:])
}

// readItemNotice reads c until the picked-up notice of count units of
// itemID.
func readItemNotice(t *testing.T, c *testsupport.ScriptedClient, itemID, count int32) {
	t.Helper()
	for range 100 {
		f := c.ReadWithTimeout(5 * time.Second)
		if f == nil {
			break
		}
		if _, gotItem, gotCount, ok := itemNotice(f); ok && gotItem == itemID && gotCount == count {
			return
		}
	}
	t.Fatalf("no notice of %d %d", count, itemID)
}

// TestAdminItemGivesToOthers creates items for a selected player, through
// //item, //item set and //give, the GM told what went where, and around
// the GM, everyone near rewarded but the GM. A target without room is
// refused.
func TestAdminItemGivesToOthers(t *testing.T) {
	t.Parallel()
	srv, _, gmID := bootItemAdmin(t)
	gm := srv.Client
	player, playerID := addPlayer(t, srv, "target", "Target", 0)
	drain(t, gm)
	exchange(t, gm, encodeAction(playerID))
	drain(t, player)

	frames := readUntilPage(t, gm, exchange(t, gm, encodeBypass("admin_item 57 10")))
	assertTexts(t, frames[:len(frames)-1], "You have spawned 10 Adena (57) in Target's inventory.")
	readItemNotice(t, player, item.AdenaID, 10)

	frames = readUntilPage(t, gm, exchange(t, gm, encodeBypass("admin_item set 58")))
	assertTexts(t, frames[:len(frames)-1], "You have spawned Mithril Heavy Set in Target's inventory.")
	for _, id := range []int32{58, 59, 47, mithrilShieldID} {
		if got := srv.PlayerItemCount(t, playerID, id); got != 1 {
			t.Fatalf("target's set piece %d = %d, want 1", id, got)
		}
	}

	gm.Send(encodeBuildCmd("give 57 3"))
	if got := readText(t, gm); got != "You have spawned 3 Adena (57) in Target's inventory." {
		t.Fatalf("//give report = %q", got)
	}
	readItemNotice(t, player, item.AdenaID, 3)
	assertTexts(t, exchange(t, gm, encodeBuildCmd("give")), "Usage: //give itemId count")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("give x")), "Usage: //give itemId count")

	// A Short Sword per unit cannot fit two hundred times.
	gm.Send(encodeBuildCmd("give 1 200"))
	if got := readText(t, gm); got != "Your target's inventory is full." {
		t.Fatalf("//give over capacity = %q", got)
	}

	frames = readUntilPage(t, gm, exchange(t, gm, encodeBypass("admin_item 57 7 1000")))
	assertTexts(t, frames[:len(frames)-1], "Surrounding players were rewarded with 7 Adena in a 1000 radius.")
	readItemNotice(t, player, item.AdenaID, 7)
	if got := srv.PlayerItemCount(t, playerID, item.AdenaID); got != 20 {
		t.Fatalf("target's adena = %d, want 20", got)
	}
	if got := srv.PlayerItemCount(t, gmID, item.AdenaID); got != 0 {
		t.Fatalf("GM's adena = %d, want 0", got)
	}
}
