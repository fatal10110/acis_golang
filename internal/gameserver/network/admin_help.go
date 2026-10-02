package network

import (
	"strings"

	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
)

// adminHelpPageLimit is how many commands a page of //help shows.
const adminHelpPageLimit = 7

// adminHelp answers //help [page] [search]: a page of the command table,
// in the order the table lists it, seven commands to a page, under a search
// bar. A search keeps the commands whose word or parameters contain it,
// the search lower-cased first. A page that does not read as an int, or a
// page below 1 when commands are found, opens the first page of the whole
// table instead.
func (l *GameClientLink) adminHelp(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	page, search := 1, ""
	if len(args) > 0 {
		n, ok := parseJavaInt(args[0])
		if !ok {
			l.sendAdminHelp(gm, 1, "")
			return
		}
		page = int(n)
		if len(args) > 1 {
			search = strings.ToLower(args[1])
		}
	}
	if !l.sendAdminHelp(gm, page, search) {
		l.sendAdminHelp(gm, 1, "")
	}
}

// sendAdminHelp opens page of the commands search finds on gm. It reports
// false, sending nothing, for a page below 1 when commands are found.
func (l *GameClientLink) sendAdminHelp(gm *livePlayer, page int, search string) bool {
	var found []admin.Command
	for _, cmd := range l.admin.Commands() {
		if strings.Contains(commandWord(cmd.Name), search) || strings.Contains(cmd.Params, search) {
			found = append(found, cmd)
		}
	}
	pg, ok := handleradmin.Paginate(len(found), page, adminHelpPageLimit)
	if !ok {
		return false
	}
	var b strings.Builder
	b.WriteString("<html><body>")
	handleradmin.Search(&b, "bypass admin_help", 45, len(found))
	for row, cmd := range found[pg.Start:pg.End] {
		if row%2 == 0 {
			b.WriteString("<table width=280 height=41 bgcolor=000000><tr>")
		} else {
			b.WriteString("<table width=280 height=41><tr>")
		}
		b.WriteString(`<td width=280 height=34><font color="LEVEL">//` + commandWord(cmd.Name) + "</font>")
		if strings.TrimSpace(cmd.Params) != "" {
			b.WriteString(` <font color="33cccc">` + cmd.Params + "</font>")
		}
		b.WriteString("<br1>" + cmd.Description + "</td>")
		b.WriteString(`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`)
	}
	pg.Space(&b, 42)
	pg.Links(&b, "bypass admin_help %page% "+search)
	b.WriteString("</body></html>")
	sendValidatedHTML(gm, 0, b.String(), 0)
	return true
}

// commandWord is a command table name without its "admin_" prefix: what a
// game master types after "//".
func commandWord(name string) string {
	return name[min(len("admin_"), len(name)):]
}
