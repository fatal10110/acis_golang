package admin

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// helpSearchBar is Pagination.generateSearch("bypass admin_help", 45) over
// found entries.
func helpSearchBar(found int) string {
	return `<table width=280 height=45><tr><td width=70 align=center>Search</td>` +
		`<td width=140><edit var="search" width=130 height=15></td>` +
		`<td width=70><button value="Find" action="bypass admin_help 1 $search" width=65 height=19 back="L2UI_ch3.smallbutton2_over" fore="L2UI_ch3.smallbutton2"></td>` +
		`</tr><tr><td></td><td align=center>Found ` + strconv.Itoa(found) + ` results</td><td></td></tr></table>`
}

// helpRow is one command row of AdminAdmin.sendHelp.
func helpRow(row int, word, params, desc string) string {
	open := "<table width=280 height=41><tr>"
	if row%2 == 0 {
		open = "<table width=280 height=41 bgcolor=000000><tr>"
	}
	s := open + `<td width=280 height=34><font color="LEVEL">//` + word + "</font>"
	if params != "" {
		s += ` <font color="33cccc">` + params + "</font>"
	}
	return s + "<br1>" + desc + `</td></tr></table><img src="L2UI.SquareGray" width=280 height=1>`
}

// TestAdminHelp pins AdminAdmin.java's //help over the shipped
// adminCommands.xml byte for byte: a search, lower-cased, keeps the
// commands whose word or parameters contain it, in table order, seven to a
// page under the search bar, with the parameters shown when not blank; a
// page that does not read, or page 0, falls back to the first page of the
// whole table, and a page past the last shows the last.
func TestAdminHelp(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)

	want := "<html><body>" + helpSearchBar(5) +
		helpRow(0, "gmlist", "", "Toggle you from /gmlist results.") +
		helpRow(1, "gmoff", "[duration]", "Toggle off your GM status, 1min by default.") +
		helpRow(2, "gmchat", "message", "Broadcast the message to all GMs.") +
		helpRow(3, "gmspeed", "[0-4]", "Affect you with the GM Haste skill.") +
		helpRow(4, "server", "[shutdown|restart|abort|gmonly|all|max]", "Run one of the server related commands.") +
		strings.Repeat("<img height=42>", 2) + onePageBar("bypass admin_help 1 gm") + "</body></html>"
	for _, cmd := range []string{"help 1 GM", "help 3 gm"} {
		frames := exchange(t, gm, encodeBuildCmd(cmd))
		if len(frames) != 1 {
			t.Fatalf("//%s frames = %x, want one page", cmd, testsupport.FrameOpcodes(frames))
		}
		if got := htmlBody(t, frames[0]); got != want {
			t.Fatalf("//%s page =\n%s\nwant\n%s", cmd, got, want)
		}
	}

	raw, err := os.ReadFile(datapack.Path(t, "data", "xml", "adminCommands.xml"))
	if err != nil {
		t.Fatalf("read adminCommands.xml: %v", err)
	}
	total := len(regexp.MustCompile(`<aCar `).FindAll(raw, -1))
	firstPage := "<html><body>" + helpSearchBar(total) +
		helpRow(0, "admin", "[1-4]", "Go through the different admin panels.") +
		helpRow(1, "buy", "[id]", "Open the GM Shop panel, or the associated BuyList.")
	for _, cmd := range []string{"help", "help 0 gm", "help x gm", "help 99999999999"} {
		frames := exchange(t, gm, encodeBuildCmd(cmd))
		if len(frames) != 1 {
			t.Fatalf("//%s frames = %x, want one page", cmd, testsupport.FrameOpcodes(frames))
		}
		got := htmlBody(t, frames[0])
		if !strings.HasPrefix(got, firstPage) || !strings.Contains(got, `<a action="bypass admin_help 2 ">02</a>`) {
			t.Fatalf("//%s page = %s\nwant the first page of all %d commands", cmd, got, total)
		}
	}
}
