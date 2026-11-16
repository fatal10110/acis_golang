package admin

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The listings below follow the iteration order of the reference's hash
// tables: a table of n keys has 16 buckets up to twelve keys, an id falls
// in bucket id&15 (ids below 65536), a text key in the folded low bits of
// its 31-polynomial hash; a bucket lists its keys in the order they were
// put in.

// TestAdminSpawnByNameTakesListedTemplate: //spawn and //list_spawns by a
// name two templates share take the one the template table lists first,
// the monster 20120 (bucket 8) before the pet 12077 (bucket 13), as the
// shipped Wolf templates; the pet cannot be spawned.
func TestAdminSpawnByNameTakesListedTemplate(t *testing.T) {
	t.Parallel()
	tmpl := func(id int, typ string) *npc.Template {
		return &npc.Template{ID: id, TemplateID: id, Type: typ, Name: "Wolf", Level: 1, HPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true}
	}
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{tmpl(12077, "Pet"), tmpl(20120, "Monster")})),
		gameservertest.WithNpcSpawns(nil))
	gm := srv.Client
	enterWorld(t, gm)

	frames := exchange(t, gm, encodeBuildCmd("spawn wolf"))
	if got := textsIn(t, frames); !slices.Equal(got, []string{"You spawned Wolf. - Cmd: admin_spawn"}) {
		t.Fatalf("//spawn wolf frames = %x, texts %q; want the spawn notice", testsupport.FrameOpcodes(frames), got)
	}
	if got := len(npcsOf(srv, 20120)); got != 1 {
		t.Fatalf("NPCs of 20120 = %d, want the spawned wolf", got)
	}

	frames = exchange(t, gm, encodeBuildCmd("list_spawns WOLF"))
	if len(frames) != 1 || !strings.Contains(htmlBody(t, frames[0]), "Spawn [id=20120]") {
		t.Fatalf("//list_spawns WOLF frames = %x, want the page of 20120", testsupport.FrameOpcodes(frames))
	}
}

var spawnButton = regexp.MustCompile(`bypass admin_spawn (\d+)"`)

// TestAdminFindNPCEqualNamesInListingOrder: //find npc sorts by name and
// keeps templates of one name in table order: of the nine wolves 20010 to
// 20018, 20016-20018 (buckets 0-2) come before 20010-20015 (buckets 10-15),
// the tie running over the page break.
func TestAdminFindNPCEqualNamesInListingOrder(t *testing.T) {
	t.Parallel()
	var tmpls []*npc.Template
	for id := 20010; id <= 20018; id++ {
		tmpls = append(tmpls, &npc.Template{ID: id, TemplateID: id, Type: "Monster", Name: "Wolf", Level: 1, HPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60})
	}
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithNPCs(npc.NewTable(tmpls)))
	gm := srv.Client
	enterWorld(t, gm)

	for _, tc := range []struct {
		cmd  string
		want []int
	}{
		{"find npc wolf", []int{20016, 20017, 20018, 20010, 20011, 20012, 20013}},
		{"find npc 2 wolf", []int{20014, 20015}},
	} {
		frames := exchange(t, gm, encodeBuildCmd(tc.cmd))
		if len(frames) != 1 {
			t.Fatalf("//%s frames = %x, want one page", tc.cmd, testsupport.FrameOpcodes(frames))
		}
		var got []int
		for _, m := range spawnButton.FindAllStringSubmatch(htmlBody(t, frames[0]), -1) {
			id, _ := strconv.Atoi(m[1])
			got = append(got, id)
		}
		if !slices.Equal(got, tc.want) {
			t.Fatalf("//%s lists %v, want %v", tc.cmd, got, tc.want)
		}
	}
}

// TestAdminFindDualboxInListingOrder: //find dualbox lists the shared
// addresses in the order the address table does, not by address:
// 127.0.0.4 (bucket 3), .8 (7), .2 (13), .1 (14).
func TestAdminFindDualboxInListingOrder(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithHTMLPages(shippedAdminPages(t, findPages...)))
	gm := srv.Client
	enterWorld(t, gm)
	for i, local := range []string{"127.0.0.2", "127.0.0.8", "127.0.0.2", "127.0.0.8", "127.0.0.4"} {
		account := "box" + strconv.Itoa(i)
		srv.SeedCharacterFor(t, account, "Box"+strconv.Itoa(i), 1, 0)
		c := srv.DialClientFrom(t, account, 1, local)
		enterWorld(t, c)
	}
	drain(t, gm)

	rows := func(multibox string, ips ...string) string {
		var b strings.Builder
		for _, ip := range ips {
			b.WriteString(`<a action="bypass -h admin_find ip ` + ip + `">` + ip + " (" + multibox + ")</a><br1>")
		}
		return shippedPage(t, "dualbox.htm", "%multibox%", multibox, "%results%", b.String())
	}
	assertOnePage(t, "//find dualbox 1", exchange(t, gm, encodeBuildCmd("find dualbox 1")),
		rows("1", "127.0.0.4", "127.0.0.8", "127.0.0.2", "127.0.0.1"))
	assertOnePage(t, "//find dualbox", exchange(t, gm, encodeBuildCmd("find dualbox")),
		rows("2", "127.0.0.8", "127.0.0.2"))
}

var walkerShow = regexp.MustCompile(`bypass admin_show walker 1 (\w+)">Show`)

// TestAdminShowWalkerListingOrder: the walker panel lists the routes in the
// order the route table does, zeta (bucket 0), porter_remy (5), gordon
// (13), and a route's NPCs likewise: bob (4), dora (5), anna (13), cid (15).
func TestAdminShowWalkerListingOrder(t *testing.T) {
	t.Parallel()
	node := []route.WalkerLocation{{Location: location.Location{X: 1, Y: 2, Z: 3}}}
	_, gm := bootShowAdmin(t,
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithWalkerRoutes(route.WalkerRoutes{
			"gordon":      {"anna": node, "bob": node, "cid": node, "dora": node},
			"porter_remy": {"porter_remy": node},
			"zeta":        {"ghost": node},
		}))

	frames := exchangeZones(t, gm, encodeBuildCmd("show walker"))
	if len(frames) != 1 {
		t.Fatalf("//show walker frames = %x, want the panel", testsupport.FrameOpcodes(frames))
	}
	var got []string
	for _, m := range walkerShow.FindAllStringSubmatch(htmlBody(t, frames[0]), -1) {
		got = append(got, m[1])
	}
	if want := []string{"ghost", "porter_remy", "bob", "dora", "anna", "cid"}; !slices.Equal(got, want) {
		t.Fatalf("walker panel rows = %q, want %q", got, want)
	}
}
