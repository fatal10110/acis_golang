package admin

import (
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// findPages are the shipped pages //find fills.
var findPages = []string{"ipfind.htm", "accountinfo.htm", "dualbox.htm"}

// searchBar is Pagination.generateSearch(action, 45) over found entries.
func searchBar(action string, found int) string {
	return `<table width=280 height=45><tr><td width=70 align=center>Search</td>` +
		`<td width=140><edit var="search" width=130 height=15></td>` +
		`<td width=70><button value="Find" action="` + action + ` 1 $search" width=65 height=19 back="L2UI_ch3.smallbutton2_over" fore="L2UI_ch3.smallbutton2"></td>` +
		`</tr><tr><td></td><td align=center>Found ` + strconv.Itoa(found) + ` results</td><td></td></tr></table>`
}

// findPlayerRow is one row of AdminFind.listPlayers for a level 1 human
// fighter.
func findPlayerRow(row int, name string) string {
	open := "<table width=280><tr>"
	if row%2 == 0 {
		open = "<table width=280 height=22 bgcolor=000000><tr>"
	}
	return open + `<td width=140><a action="bypass -h admin_debug ` + name + `">` + name + `</a></td><td width=120>` + player.ClassName(0) +
		`</td><td width=20>1</td></tr></table><img src="L2UI.SquareGray" width=280 height=1>`
}

// findPlayerPage is AdminFind.listPlayers listing rows under search.
func findPlayerPage(search string, rows ...string) string {
	return "<html><body>" + searchBar("bypass admin_find player", len(rows)) +
		"<br1><table width=280 height=26><tr><td width=140>Name</td><td width=120>Class</td><td width=20>Lvl</td></tr></table>" +
		strings.Join(rows, "") + strings.Repeat("<img height=22>", 12-len(rows)) +
		pageBar("bypass admin_find player %page% "+search, 1, min(len(rows), 1)) + "</body></html>"
}

// ipRow is one row of AdminFind.listPlayersPerIp.
func ipRow(name string) string {
	return `<tr><td><a action="bypass -h admin_debug ` + name + `">` + name + `</a></td><td>` + player.ClassName(0) + "</td><td>1</td></tr>"
}

// findItemRow is one row of AdminFind.listItems.
func findItemRow(row, id int, name string) string {
	open := "<table width=280><tr>"
	if row%2 == 0 {
		open = "<table width=280 height=22 bgcolor=000000><tr>"
	}
	sid := strconv.Itoa(id)
	return open + "<td width=36 height=41 align=center><table><tr><td><img src=icon.noimage width=32 height=32></td></tr></table></td>" +
		"<td width=160>" + name + `<br1><font color="B09878">Item Id:</font> <font color=BDB76B>` + sid + "</font></td>" +
		`<td><edit var="amount_` + sid + `" width=52 type=number></td>` +
		`<td><button action="bypass admin_give ` + sid + " $amount_" + sid + `" width=32 height=32 back=L2UI_CH3.mapbutton_zoomin2 fore=L2UI_CH3.mapbutton_zoomin1></td>` +
		`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`
}

// findNPCRow is one row of AdminFind.listNpcs.
func findNPCRow(row, id int, title, name string) string {
	open := "<table width=280><tr>"
	if row%2 == 0 {
		open = "<table width=280 height=22 bgcolor=000000><tr>"
	}
	sid := strconv.Itoa(id)
	return open + `<td width=216 height=41><font color="B09878">` + title + "</font><br1>" + name + "</td>" +
		`<td><button action="bypass admin_spawn ` + sid + `" width=32 height=32 back=L2UI_CH3.mapbutton_zoomin2 fore=L2UI_CH3.mapbutton_zoomin1></td>` +
		`<td><button action="bypass admin_list_spawns ` + sid + `" width=32 height=32 back=L2UI_CH3.mapicon_mark_light fore=L2UI_CH3.mapicon_mark></td>` +
		`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`
}

// findTablePage is AdminFind.listItems or listNpcs (list) listing rows
// under search.
func findTablePage(list, search string, rows ...string) string {
	return "<html><body>" + searchBar("bypass admin_find "+list, len(rows)) + strings.Join(rows, "") +
		strings.Repeat("<img height=42>", 7-len(rows)) + pageBar("bypass admin_find "+list+" %page% "+search, 1, min(len(rows), 1)) + "</body></html>"
}

// findItems are five item templates, one a quest item and one named past
// the 28 characters a row shows.
func findItems() *item.Table {
	etc := func(id int32, name string, typ item.EtcItemType) *item.Template {
		return &item.Template{ID: id, Name: name, Kind: item.KindEtcItem, Duration: -1, EtcItem: &item.EtcItemDetail{Type: typ}}
	}
	return item.NewTable([]*item.Template{
		etc(item.AdenaID, "Adena", 0),
		etc(1, "Short Sword", 0),
		etc(2, "Long Sword", 0),
		etc(3, "Sword of the Very Long Name Indeed", 0),
		etc(1100, "Wolf Pelt", item.EtcItemQuest),
	})
}

// findNPCs are four NPC templates: two wolves, a nameless one and a
// gremlin.
func findNPCs() *npc.Table {
	tmpl := func(id int, name, title string) *npc.Template {
		return &npc.Template{ID: id, TemplateID: id, Type: "Monster", Name: name, Title: title, Level: 1, HPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60}
	}
	return npc.NewTable([]*npc.Template{
		tmpl(20120, "Wolf", "Elder"),
		tmpl(20003, "", "Nobody"),
		tmpl(20001, "Wolf", ""),
		tmpl(20004, "Gremlin", "Pet"),
	})
}

// TestAdminFind pins AdminFind.java: //find player lists the players whose
// lower-cased name holds the search, //find ip the players connected from
// an address, //find account the online player's account and other
// characters, //find dualbox the addresses shared by enough players,
// //find item and //find npc the templates whose name holds every word
// searched; a page below the first falls back as the reference does.
func TestAdminFind(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithHTMLPages(shippedAdminPages(t, findPages...)),
		gameservertest.WithItemTemplates(findItems()),
		gameservertest.WithNPCs(findNPCs()))
	gm := srv.Client
	srv.SeedCharacterFor(t, "player1", "Alt", 1, 0)
	enterWorld(t, gm)
	other, _ := addPlayer(t, srv, "other", "Other", userLevel)
	drain(t, gm)
	drain(t, other)

	both := findPlayerPage("", findPlayerRow(0, "Admin"), findPlayerRow(1, "Other"))
	for _, cmd := range []string{"find", "find player", "find player 0", "find player 1"} {
		frames := exchange(t, gm, encodeBuildCmd(cmd))
		if cmd == "find player 0" {
			assertTexts(t, frames[:1], "Usage: //find player name")
			frames = frames[1:]
		}
		assertOnePage(t, "//"+cmd, frames, both)
	}
	assertOnePage(t, "//find player oth", exchange(t, gm, encodeBuildCmd("find player oth")), findPlayerPage("oth", findPlayerRow(0, "Other")))
	assertOnePage(t, "//find player 1 OTH", exchange(t, gm, encodeBypass("admin_find player 1 OTH")), findPlayerPage("oth", findPlayerRow(0, "Other")))
	// A name is matched as typed against lower-cased names.
	assertOnePage(t, "//find player Oth", exchange(t, gm, encodeBuildCmd("find player Oth")), findPlayerPage("Oth"))

	byIP := shippedPage(t, "ipfind.htm", "%number%", "2", "%ip%", "127.0.0.1", "%results%", ipRow("Admin")+ipRow("Other"))
	assertOnePage(t, "//find ip", exchange(t, gm, encodeBuildCmd("find ip")), byIP)
	assertOnePage(t, "//find ip 127.0.0.1", exchange(t, gm, encodeBuildCmd("find ip 127.0.0.1")), byIP)
	assertOnePage(t, "//find ip 10.0.0.1", exchange(t, gm, encodeBuildCmd("find ip 10.0.0.1")),
		shippedPage(t, "ipfind.htm", "%number%", "0", "%ip%", "10.0.0.1", "%results%", ""))
	assertOnePage(t, "//find ip disconnected", exchange(t, gm, encodeBuildCmd("find ip disconnected")),
		shippedPage(t, "ipfind.htm", "%number%", "0", "%ip%", "disconnected", "%results%", ""))
	for _, ip := range []string{"256.0.0.1", "01.2.3.4", "localhost"} {
		assertTexts(t, exchange(t, gm, encodeBuildCmd("find ip "+ip)), "Usage: //find ip 111.222.333.444")
	}

	assertOnePage(t, "//find account", exchange(t, gm, encodeBuildCmd("find account")),
		shippedPage(t, "accountinfo.htm", "%name%", "Admin", "%characters%", "Alt", "%account%", "player1"))
	assertOnePage(t, "//find account other", exchange(t, gm, encodeBuildCmd("find account other")),
		shippedPage(t, "accountinfo.htm", "%name%", "other", "%characters%", "", "%account%", "other"))
	frames := exchange(t, gm, encodeBuildCmd("find account Alt"))
	if len(frames) != 1 {
		t.Fatalf("//find account Alt frames = %x, want TARGET_CANT_FOUND", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageTargetCantFound)

	dualbox := func(multibox, results string) string {
		return shippedPage(t, "dualbox.htm", "%multibox%", multibox, "%results%", results)
	}
	shared := `<a action="bypass -h admin_find ip 127.0.0.1">127.0.0.1 (2)</a><br1>`
	assertOnePage(t, "//find dualbox", exchange(t, gm, encodeBuildCmd("find dualbox")), dualbox("2", shared))
	assertOnePage(t, "//find dualbox x", exchange(t, gm, encodeBuildCmd("find dualbox x")), dualbox("2", shared))
	assertOnePage(t, "//find dualbox 1", exchange(t, gm, encodeBuildCmd("find dualbox 1")),
		dualbox("1", `<a action="bypass -h admin_find ip 127.0.0.1">127.0.0.1 (1)</a><br1>`))
	assertOnePage(t, "//find dualbox 3", exchange(t, gm, encodeBuildCmd("find dualbox 3")), dualbox("3", ""))
	assertTexts(t, exchange(t, gm, encodeBuildCmd("find dualbox 0")), "Usage: //find dualbox [number > 0]")

	swords := findTablePage("item", "sword", findItemRow(0, 1, "Short Sword"), findItemRow(1, 2, "Long Sword"), findItemRow(2, 3, "Sword of the Very Long Na..."))
	assertOnePage(t, "//find item sword", exchange(t, gm, encodeBuildCmd("find item sword")), swords)
	assertOnePage(t, "//find item 1 sword", exchange(t, gm, encodeBypass("admin_find item 1 sword")), swords)
	assertOnePage(t, "//find item 1 long  SWORD", exchange(t, gm, encodeBypass("admin_find item 1 long  SWORD")),
		findTablePage("item", "long  SWORD", findItemRow(0, 2, "Long Sword"), findItemRow(1, 3, "Sword of the Very Long Na...")))
	// Page 0 searches the page too.
	assertOnePage(t, "//find item 0 sword", exchange(t, gm, encodeBuildCmd("find item 0 sword")), findTablePage("item", "0 sword"))
	all := findTablePage("item", "", findItemRow(0, 1, "Short Sword"), findItemRow(1, 2, "Long Sword"), findItemRow(2, 3, "Sword of the Very Long Na..."),
		findItemRow(3, 57, "Adena"), strings.Replace(findItemRow(4, 1100, "Wolf Pelt"), "1100</font>", "1100 (Quest)</font>", 1))
	assertOnePage(t, "//find item", exchange(t, gm, encodeBuildCmd("find item")), all)

	wolves := findTablePage("npc", "wolf", findNPCRow(0, 20001, "", "Wolf"), findNPCRow(1, 20120, "Elder", "Wolf"))
	assertOnePage(t, "//find npc wolf", exchange(t, gm, encodeBuildCmd("find npc wolf")), wolves)
	assertOnePage(t, "//find npc 2 wolf", exchange(t, gm, encodeBuildCmd("find npc 2 wolf")), wolves)
	assertOnePage(t, "//find npc", exchange(t, gm, encodeBuildCmd("find npc")),
		findTablePage("npc", "", findNPCRow(0, 20004, "Pet", "Gremlin"), findNPCRow(1, 20001, "", "Wolf"), findNPCRow(2, 20120, "Elder", "Wolf")))

	assertTexts(t, exchange(t, gm, encodeBuildCmd("find clan")), "Usage: //find [account|dualbox|ip|item|npc|player name/id]")
}
