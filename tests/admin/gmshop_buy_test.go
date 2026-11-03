package admin

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

var gmShopBuyLink = regexp.MustCompile(`bypass -h admin_buy (\d+)`)

// gmShopAdena is the adena the GM shop tests' GM carries.
const gmShopAdena = 50_000_000

// bootGMShop boots the GM "Admin" with the shipped item templates, buylists
// and GM shop pages, carrying gmShopAdena, and enters the world. It
// returns the server, the shipped buylists and the GM's id.
func bootGMShop(t *testing.T, opts ...gameservertest.Option) (*gameservertest.Server, *buylist.Table, int32) {
	t.Helper()
	dir := datapack.Require(t)
	items, err := shippedItemTemplates()
	if err != nil {
		t.Fatalf("load item templates: %v", err)
	}
	table, err := xmldata.LoadBuyLists(filepath.Join(dir, "data", "xml", "buyLists.xml"), items)
	if err != nil {
		t.Fatalf("load buylists: %v", err)
	}
	var lists []buylist.List
	table.Each(func(l buylist.List) { lists = append(lists, l) })
	opts = append([]gameservertest.Option{
		gameservertest.WithItemTemplates(items),
		gameservertest.WithBuyLists(lists...),
		gameservertest.WithHTMLPages(shippedAdminPages(t, "gmshops.htm", "gmshop/weapon.htm", "gmshop/armor.htm", "gmshop/items.htm")),
	}, opts...)
	srv, gmID := bootAdmin(t, adminLevel, opts...)
	srv.GiveItem(t, gmID, item.AdenaID, gmShopAdena)
	enterWorld(t, srv.Client)
	return srv, table, gmID
}

// TestAdminBuyOpensShippedGMLists clicks the main panel's GM Shop
// button, then the first and last list link of every GM shop page: each opens its
// shipped list's buy window untaxed, showing the GM's adena. An extra
// argument is ignored and a list some NPC sells opens too.
func TestAdminBuyOpensShippedGMLists(t *testing.T) {
	t.Parallel()
	srv, table, _ := bootGMShop(t)
	c := srv.Client

	main := htmlBody(t, exchange(t, c, encodeBypass("admin_admin"))[0])
	if !regexp.MustCompile(`action="bypass -h admin_buy"`).MatchString(main) {
		t.Fatalf("main panel has no GM Shop button: %q", main)
	}
	assertPage(t, exchange(t, c, encodeBypass("admin_buy")), "GM Shop")

	// Every link's list is pinned by the datapack's own tests; the window
	// is the same code for each, so the first and last of every page are
	// opened, which keeps the clicks under the flood limit.
	for _, page := range []string{"gmshop/weapon.htm", "gmshop/armor.htm", "gmshop/items.htm"} {
		body := htmlBody(t, exchange(t, c, encodeBypass("admin_link "+page))[0])
		links := gmShopBuyLink.FindAllStringSubmatch(body, -1)
		if len(links) == 0 {
			t.Fatalf("%s has no admin_buy link", page)
		}
		for _, m := range [][]string{links[0], links[len(links)-1]} {
			id, _ := strconv.Atoi(m[1])
			assertBuyWindow(t, exchange(t, c, encodeBypass("admin_buy "+m[1])), table, id)
		}
	}
	assertBuyWindow(t, exchange(t, c, encodeBypass("admin_buy 1006 1")), table, 1006)
	assertBuyWindow(t, exchange(t, c, encodeBuildCmd("buy 18")), table, 18)
}

// assertBuyWindow requires frames to be the one buy window of shipped list
// id: the GM's adena, the list id, and each product at its untaxed price.
func assertBuyWindow(t *testing.T, frames [][]byte, table *buylist.Table, id int) {
	t.Helper()
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeBuyList {
		t.Fatalf("//buy %d frames = %x, want one BuyList", id, testsupport.FrameOpcodes(frames))
	}
	list, ok := table.Find(id)
	if !ok {
		t.Fatalf("list %d not shipped", id)
	}
	r := wire.NewReader(frames[0][1:])
	if money, listID, n := r.ReadInt32(), r.ReadInt32(), r.ReadUint16(); money != gmShopAdena || listID != int32(id) || int(n) != len(list.Products) {
		t.Fatalf("//buy %d header = adena %d list %d rows %d, want %d, %d, %d", id, money, listID, n, gmShopAdena, id, len(list.Products))
	}
	for i, p := range list.Products {
		r.ReadUint16() // type1
		r.ReadInt32()  // object id
		itemID := r.ReadInt32()
		r.ReadInt32()  // count
		r.ReadUint16() // type2
		r.ReadUint16()
		r.ReadInt32() // body part
		r.ReadUint16()
		r.ReadUint16()
		r.ReadUint16()
		if price := r.ReadInt32(); itemID != p.ItemID || int(price) != p.Price {
			t.Fatalf("//buy %d row %d = item %d at %d, want %d at %d", id, i, itemID, price, p.ItemID, p.Price)
		}
	}
	if err := r.Err(); err != nil || r.Remaining() != 0 {
		t.Fatalf("//buy %d decode: %v, %d bytes left", id, err, r.Remaining())
	}
}

// TestAdminBuyRefusesBadListIDs answers an id that is not a number, is out
// of the int range or names no list.
func TestAdminBuyRefusesBadListIDs(t *testing.T) {
	t.Parallel()
	srv, _, _ := bootGMShop(t)
	for _, arg := range []string{"x", "99999999999", "999999999", "-1"} {
		assertTexts(t, exchange(t, srv.Client, encodeBypass("admin_buy "+arg)), "Invalid buylist id.")
	}
}

func encodeGMShopBuy(listID, itemID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBuyItem)
	w.WriteInt32(listID)
	w.WriteInt32(1)
	w.WriteInt32(itemID)
	w.WriteInt32(count)
	return w.Bytes()
}

// TestAdminBuyGMListPurchase buys from a GM list with no merchant in
// sight: the item arrives and is stored, the adena goes, and the full item
// list follows. Two of a weapon at once are refused; a player who is no GM sending
// the same list gets nothing and keeps the adena.
func TestAdminBuyGMListPurchase(t *testing.T) {
	t.Parallel()
	srv, table, gmID := bootGMShop(t)
	c := srv.Client
	list, ok := table.Find(1006)
	if !ok || list.NPCID != -1 || len(list.Products) == 0 {
		t.Fatalf("list 1006 = %+v, want a GM list", list)
	}
	p := list.Products[0]
	exchange(t, c, encodeBypass("admin_buy 1006"))

	assertStatic(t, exchange(t, c, encodeGMShopBuy(1006, p.ItemID, 2))[0], serverpackets.SystemMessageYouHaveExceededQuantityThatCanBeInputted)
	frames := exchange(t, c, encodeGMShopBuy(1006, p.ItemID, 1))
	if last := frames[len(frames)-1]; last[0] != serverpackets.OpcodeItemList {
		t.Fatalf("buy frames = %x, want the item list last", testsupport.FrameOpcodes(frames))
	}
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeNpcHtmlMessage {
			t.Fatalf("buy frames = %x, want no page", testsupport.FrameOpcodes(frames))
		}
	}
	if got := srv.PlayerItemCount(t, gmID, p.ItemID); got != 1 {
		t.Fatalf("bought %d = %d, want 1", p.ItemID, got)
	}
	if got, want := srv.PlayerItemCount(t, gmID, item.AdenaID), gmShopAdena-p.Price; got != want {
		t.Fatalf("adena after buying = %d, want %d", got, want)
	}
	srv.FlushItems(t)
	var stored int
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM items WHERE owner_id = ? AND item_id = ? AND loc = 'INVENTORY'", gmID, p.ItemID).Scan(&stored); err != nil || stored == 0 {
		t.Fatalf("stored rows of %d = %d (%v), want some", p.ItemID, stored, err)
	}

	ch := srv.SeedCharacterFor(t, "plain", "Plain", 1, 0)
	playerID := ch.ID
	srv.GiveItem(t, playerID, item.AdenaID, gmShopAdena)
	player := srv.DialClient(t, "plain", 1)
	enterWorld(t, player)
	if frames := exchange(t, player, encodeGMShopBuy(1006, p.ItemID, 1)); len(frames) != 0 {
		t.Fatalf("player buy frames = %x, want none", testsupport.FrameOpcodes(frames))
	}
	if got := srv.PlayerItemCount(t, playerID, p.ItemID); got != 0 {
		t.Fatalf("player bought %d = %d, want 0", p.ItemID, got)
	}
	if got := srv.PlayerItemCount(t, playerID, item.AdenaID); got != gmShopAdena {
		t.Fatalf("player adena = %d, want %d", got, gmShopAdena)
	}
}
