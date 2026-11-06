package admin

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// bookmarkRow is one bookmark row of AdminBookmark.showBookmarks.
func bookmarkRow(row int, name string, x, y, z int) string {
	open := "<table width=280><tr>"
	if row%2 == 0 {
		open = "<table width=280 bgcolor=000000><tr>"
	}
	at := strconv.Itoa(x) + " " + strconv.Itoa(y) + " " + strconv.Itoa(z)
	return open + `<td width=230><a action="bypass -h admin_teleport ` + at + `">` + name + " (" + at + `)</a></td>` +
		`<td width=50><a action="bypass -h admin_delbk ` + name + `">Remove</a></td>` +
		`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`
}

// bookmarkPage is the shipped bk.htm listing rows, padded to fifteen.
func bookmarkPage(t *testing.T, rows ...string) string {
	t.Helper()
	content := strings.Join(rows, "") + strings.Repeat("<img height=20>", 15-len(rows)) + pageBar("bypass admin_bk %page%", 1, min(len(rows), 1))
	return shippedPage(t, "bk.htm", "%content%", content)
}

// bookmarkNames returns ownerID's bookmarks rows, each "name x y z", by
// name.
func bookmarkNames(t *testing.T, srv *gameservertest.Server, ownerID int32) []string {
	t.Helper()
	srv.FlushPersistence(t)
	rows, err := srv.DB.QueryContext(context.Background(), "SELECT name, x, y, z FROM bookmarks WHERE obj_Id = ? ORDER BY name", ownerID)
	if err != nil {
		t.Fatalf("query bookmarks: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		var x, y, z int
		if err := rows.Scan(&name, &x, &y, &z); err != nil {
			t.Fatalf("scan bookmark: %v", err)
		}
		out = append(out, name+" "+strconv.Itoa(x)+" "+strconv.Itoa(y)+" "+strconv.Itoa(z))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read bookmarks: %v", err)
	}
	return out
}

// assertOnePage requires frames to be exactly the page want.
func assertOnePage(t *testing.T, cmd string, frames [][]byte, want string) {
	t.Helper()
	if len(frames) != 1 {
		t.Fatalf("%s frames = %x, want one page", cmd, testsupport.FrameOpcodes(frames))
	}
	if got := htmlBody(t, frames[0]); got != want {
		t.Fatalf("%s page =\n%s\nwant\n%s", cmd, got, want)
	}
}

// TestAdminBookmark pins AdminBookmark.java and BookmarkTable.java: the
// GM's saved bookmarks read back in key order, a new one saved at the GM's
// position joins the end of the list and the bookmarks table, a name
// matching one held in any case or longer than fifteen characters is
// refused, //delbk removes one by a name matching in any case, and a page
// below the first sends nothing.
func TestAdminBookmark(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithHTMLPages(shippedAdminPages(t, "bk.htm")))
	gm := srv.Client
	for _, row := range []struct {
		name    string
		x, y, z int
	}{{"zeta", 1, 2, 3}, {"Alpha", -4, 5, -6}} {
		if _, err := srv.DB.ExecContext(context.Background(), "INSERT INTO bookmarks (name, obj_Id, x, y, z) VALUES (?, ?, ?, ?, ?)", row.name, gmID, row.x, row.y, row.z); err != nil {
			t.Fatalf("seed bookmark: %v", err)
		}
	}
	enterWorld(t, gm)

	saved := []string{bookmarkRow(0, "Alpha", -4, 5, -6), bookmarkRow(1, "zeta", 1, 2, 3)}
	assertOnePage(t, "//bk", exchange(t, gm, encodeBuildCmd("bk")), bookmarkPage(t, saved...))

	home := bookmarkRow(2, "Home", spawnX, spawnY, spawnZ)
	assertOnePage(t, "//bk Home", exchange(t, gm, encodeBuildCmd("bk Home")), bookmarkPage(t, append(saved, home)...))
	if got, want := strings.Join(bookmarkNames(t, srv, gmID), ","), "Alpha -4 5 -6,Home 10 20 30,zeta 1 2 3"; got != want {
		t.Fatalf("bookmarks rows = %s, want %s", got, want)
	}

	assertTexts(t, exchange(t, gm, encodeBuildCmd("bk HOME")), "The bookmark name already exists.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("bk abcdefghijklmnop")), "The bookmark name is too long.")
	assertOnePage(t, "//bk abcdefghijklmno", exchange(t, gm, encodeBuildCmd("bk abcdefghijklmno")),
		bookmarkPage(t, append(saved, home, bookmarkRow(3, "abcdefghijklmno", spawnX, spawnY, spawnZ))...))
	assertOnePage(t, "//delbk ABCDEFGHIJKLMNO", exchange(t, gm, encodeBypass("admin_delbk ABCDEFGHIJKLMNO")), bookmarkPage(t, append(saved, home)...))
	assertOnePage(t, "//bk 2", exchange(t, gm, encodeBypass("admin_bk 2")), bookmarkPage(t, append(saved, home)...))
	if frames := exchange(t, gm, encodeBuildCmd("bk 0")); len(frames) != 0 {
		t.Fatalf("//bk 0 frames = %x, want none", testsupport.FrameOpcodes(frames))
	}
	if frames := exchange(t, gm, encodeBuildCmd("bk 99999999999")); len(frames) != 0 {
		t.Fatalf("//bk 99999999999 frames = %x, want none", testsupport.FrameOpcodes(frames))
	}

	assertTexts(t, exchange(t, gm, encodeBuildCmd("delbk")), "The command delbk must be followed by a valid name.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("delbk Nowhere")), "That bookmark doesn't exist.")
	assertOnePage(t, "//delbk ZETA", exchange(t, gm, encodeBuildCmd("delbk ZETA")),
		bookmarkPage(t, bookmarkRow(0, "Alpha", -4, 5, -6), bookmarkRow(1, "Home", spawnX, spawnY, spawnZ)))
	if got, want := strings.Join(bookmarkNames(t, srv, gmID), ","), "Alpha -4 5 -6,Home 10 20 30"; got != want {
		t.Fatalf("bookmarks rows after //delbk = %s, want %s", got, want)
	}
	exchange(t, gm, encodeBuildCmd("delbk alpha"))
	assertOnePage(t, "//delbk home", exchange(t, gm, encodeBuildCmd("delbk home")), bookmarkPage(t))
	if rows := bookmarkNames(t, srv, gmID); len(rows) != 0 {
		t.Fatalf("bookmarks rows after deleting all = %s, want none", rows)
	}
}
