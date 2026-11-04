package admin

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// pageNotice is the general-chat line, from no object and under the name
// HTML, that names the page file text ahead of its window.
func pageNotice(text string) []byte {
	w := wire.NewPacketWriter(serverpackets.OpcodeCreatureSay)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteString("HTML")
	w.WriteString(text)
	return w.Bytes()
}

func encodeLinkHTML(link string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestLinkHtml)
	w.WriteString(link)
	return w.Bytes()
}

// requirePage fails unless frames are one HTML window, alone.
func requirePage(t *testing.T, what string, frames [][]byte) {
	t.Helper()
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeNpcHtmlMessage {
		t.Fatalf("%s = %x, want the page alone", what, opcodes(frames))
	}
}

// requireNamedPage fails unless frames are the notice naming file, then
// the HTML window.
func requireNamedPage(t *testing.T, what, file string, frames [][]byte) {
	t.Helper()
	if len(frames) != 2 || !bytes.Equal(frames[0], pageNotice(file)) || frames[1][0] != serverpackets.OpcodeNpcHtmlMessage {
		t.Fatalf("%s = %x, want the notice naming %q, then the page", what, opcodes(frames), file)
	}
}

// TestAdminShowHTMLNamesPageFiles pins //show html: it answers nothing and
// flips one switch for every game master. While it is on, a game master
// shown a page set from a datapack file is first told the file's name,
// what follows its first "html/"; a page built in code, and every page a
// player below game master is shown, comes alone.
func TestAdminShowHTMLNamesPageFiles(t *testing.T) {
	t.Parallel()
	srv, gm := bootShowAdmin(t, gameservertest.WithHTMLPages(map[string]string{"test/link.htm": "<html><body>linked</body></html>"}))
	secondID := srv.SeedCharacterFor(t, "player2", "Second", 1, 0).ID
	setAccessLevel(t, srv, secondID, adminLevel)
	second := srv.DialClient(t, "player2", 1)
	enterWorld(t, second)
	srv.SeedCharacterFor(t, "player3", "Player", 1, 0)
	player := srv.DialClient(t, "player3", 1)
	enterWorld(t, player)
	drain(t, gm)
	drain(t, second)

	link := encodeLinkHTML("data/html/test/link.htm")
	requirePage(t, "//admin with the switch off", exchangeZones(t, gm, encodeBuildCmd("admin")))
	requirePage(t, "a linked page with the switch off", exchangeZones(t, gm, link))

	if frames := exchangeZones(t, gm, encodeBuildCmd("show html")); len(frames) != 0 {
		t.Fatalf("//show html = %x, want nothing", opcodes(frames))
	}
	requireNamedPage(t, "//admin", "admin/main_menu.htm", exchangeZones(t, gm, encodeBuildCmd("admin")))
	requireNamedPage(t, "//show walker", "admin/walker.htm", exchangeZones(t, gm, encodeBuildCmd("show walker")))
	requireNamedPage(t, "a linked page", "test/link.htm", exchangeZones(t, gm, link))
	requireNamedPage(t, "a link not under html/", "test/link.htm", exchangeZones(t, gm, encodeLinkHTML("test/link.htm")))
	requireNamedPage(t, "another game master's linked page", "test/link.htm", exchangeZones(t, second, link))
	requirePage(t, "a player's linked page", exchangeZones(t, player, link))
	requirePage(t, "//listfence, a page built in code", exchangeZones(t, gm, encodeBuildCmd("listfence")))

	// Another game master flips it back off for everyone.
	if frames := exchangeZones(t, second, encodeBuildCmd("show html")); len(frames) != 0 {
		t.Fatalf("second //show html = %x, want nothing", opcodes(frames))
	}
	requirePage(t, "//admin with the switch back off", exchangeZones(t, gm, encodeBuildCmd("admin")))
	requirePage(t, "a linked page with the switch back off", exchangeZones(t, second, link))
}

// TestAdminShowWalkerShowsFirstSpawn pins the walker panel row of a route
// whose NPC is spawned but walks no route: the NPC of the first spawn of
// that name, heading for node 0 the regular way.
func TestAdminShowWalkerShowsFirstSpawn(t *testing.T) {
	t.Parallel()
	const porterID, alias = 31356, "porter_remy"
	tmpl := gameservertest.FolkTemplate("Folk", porterID)
	tmpl.Alias, tmpl.Name = alias, "Porter Remy"
	routes := route.WalkerRoutes{
		alias: {alias: {{Location: location.Location{X: 110, Y: 20, Z: 50}}, {Location: location.Location{X: 410, Y: 20, Z: 50}}}},
	}
	srv, gm := bootShowAdmin(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{tmpl})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithWalkerRoutes(routes),
	)
	for range 2 {
		if _, err := srv.NpcSpawns.AddSpawn(porterID, 160, 20, 30, 0, false, 0); err != nil {
			t.Fatalf("spawn porter: %v", err)
		}
	}
	drain(t, gm)
	const row = `<table width=280 bgcolor=000000><tr><td width=150><a action="bypass admin_teleport 110 20 50">Porter Remy</a></td><td width=40>0 / 2</td><td width=50 align=right>Regular</td><td width=40 align=right><a action="bypass admin_show walker 1 porter_remy">Show</a></td></tr></table>`
	frames := exchangeZones(t, gm, encodeBuildCmd("show walker"))
	if len(frames) != 1 || !bytes.Contains([]byte(htmlBody(t, frames[0])), []byte(row)) {
		t.Fatalf("//show walker with the porter standing = %x, want the row\n%s\ngot\n%s", opcodes(frames), row, htmlBody(t, frames[0]))
	}
}
