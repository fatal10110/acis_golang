package network

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"

	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Page sizes of the //find lists.
const (
	adminFindPlayerPageLimit = 12
	adminFindTablePageLimit  = 7
	// adminFindIPShown is the match count past which //find ip stops and
	// reports "more than" it.
	adminFindIPShown = 20
)

// dottedIPv4 matches a dotted-quad IPv4 address, each part 0-255 without
// leading zeros.
var dottedIPv4 = regexp.MustCompile(`^(?:(?:[0-9]|[1-9][0-9]|1[0-9][0-9]|2(?:[0-4][0-9]|5[0-5]))\.){3}(?:[0-9]|[1-9][0-9]|1[0-9][0-9]|2(?:[0-4][0-9]|5[0-5]))$`)

// adminFind answers //find [account|dualbox|ip|item|npc|player] with the
// matching search page; no parameter lists every player.
func (l *GameClientLink) adminFind(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	if len(args) == 0 {
		l.findPlayers(gm, 1, "")
		return
	}
	param, nameIPOrPage, search := args[0], "", ""
	if len(args) > 1 {
		nameIPOrPage = args[1]
	}
	if len(args) > 2 {
		search = strings.ToLower(args[2])
	}
	switch param {
	case "player":
		switch {
		case len(args) < 2:
			l.findPlayers(gm, 1, "")
		case !isDigits(nameIPOrPage):
			// The name is matched as typed against lower-cased names.
			l.findPlayers(gm, 1, nameIPOrPage)
		default:
			if page, ok := parseInt32(nameIPOrPage); !ok || !l.findPlayers(gm, int(page), search) {
				sendText(gm, "Usage: //find player name")
				l.findPlayers(gm, 1, "")
			}
		}
	case "ip":
		ip := "127.0.0.1"
		if len(args) > 1 {
			ip = nameIPOrPage
		}
		if ip != "disconnected" && !dottedIPv4.MatchString(ip) {
			sendText(gm, "Usage: //find ip 111.222.333.444")
			return
		}
		l.findPlayersByIP(gm, ip)
	case "account":
		name := gm.CharacterName()
		if len(args) > 1 {
			name = nameIPOrPage
		}
		l.findAccount(gm, name)
	case "dualbox":
		multibox := int32(2)
		if n, ok := parseInt32(nameIPOrPage); ok {
			if n < 1 {
				sendText(gm, "Usage: //find dualbox [number > 0]")
				return
			}
			multibox = n
		}
		l.findDualbox(gm, int(multibox))
	case "item", "npc":
		list := l.findItems
		if param == "npc" {
			list = l.findNPCs
		}
		if len(args) < 2 {
			list(gm, 1, "")
			return
		}
		// The search is the rest of the command: past the page when one
		// leads it, else past the list word, the page then searched too
		// when it shows no page.
		rest := textAfter(line, param)
		if isDigits(nameIPOrPage) {
			if page, ok := parseInt32(nameIPOrPage); ok && list(gm, int(page), textAfter(line, nameIPOrPage)) {
				return
			}
		}
		list(gm, 1, rest)
	default:
		sendText(gm, "Usage: //find [account|dualbox|ip|item|npc|player name/id]")
	}
}

// textAfter is line past the first occurrence of sep, trimmed of blanks
// and control characters.
func textAfter(line, sep string) string {
	_, after, _ := strings.Cut(line, sep)
	return strings.TrimFunc(after, func(r rune) bool { return r <= ' ' })
}

// worldPlayersByID returns the players in the world, by object id.
func (l *GameClientLink) worldPlayersByID() []*livePlayer {
	var out []*livePlayer
	for _, p := range l.world.Players() {
		if live, ok := p.(*livePlayer); ok {
			out = append(out, live)
		}
	}
	slices.SortFunc(out, func(a, b *livePlayer) int { return cmp.Compare(a.ObjectID(), b.ObjectID()) })
	return out
}

// findPlayers opens page of the players whose lower-cased name contains
// search, twelve to a page under a search bar. It reports false, sending
// nothing, for a page below 1 when players are found.
func (l *GameClientLink) findPlayers(gm *livePlayer, page int, search string) bool {
	var found []*livePlayer
	for _, p := range l.worldPlayersByID() {
		if strings.Contains(strings.ToLower(p.CharacterName()), search) {
			found = append(found, p)
		}
	}
	pg, ok := handleradmin.Paginate(len(found), page, adminFindPlayerPageLimit)
	if !ok {
		return false
	}
	var b strings.Builder
	b.WriteString("<html><body>")
	handleradmin.Search(&b, "bypass admin_find player", 45, len(found))
	b.WriteString("<br1><table width=280 height=26><tr><td width=140>Name</td><td width=120>Class</td><td width=20>Lvl</td></tr></table>")
	for row, p := range found[pg.Start:pg.End] {
		if row%2 == 0 {
			b.WriteString("<table width=280 height=22 bgcolor=000000><tr>")
		} else {
			b.WriteString("<table width=280><tr>")
		}
		name := p.CharacterName()
		b.WriteString(`<td width=140><a action="bypass -h admin_debug ` + name + `">` + name + `</a></td><td width=120>` +
			player.ClassName(p.ClassID()) + "</td><td width=20>" + strconv.Itoa(p.Level()) + "</td>")
		b.WriteString(`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`)
	}
	pg.Space(&b, 22)
	pg.Links(&b, "bypass admin_find player %page% "+search)
	b.WriteString("</body></html>")
	sendValidatedHTML(gm, 0, b.String(), 0)
	return true
}

// findPlayersByIP opens the players connected from ip, or, for
// "disconnected", the players whose client is gone, stopping past
// adminFindIPShown + 1 of them.
func (l *GameClientLink) findPlayersByIP(gm *livePlayer, ip string) {
	disconnected := ip == "disconnected"
	var b strings.Builder
	found := 0
	for _, p := range l.worldPlayersByID() {
		if p.clientDetached() != disconnected || (!disconnected && p.remoteIP != ip) {
			continue
		}
		name := p.CharacterName()
		b.WriteString(`<tr><td><a action="bypass -h admin_debug ` + name + `">` + name + `</a></td><td>` +
			player.ClassName(p.ClassID()) + "</td><td>" + strconv.Itoa(p.Level()) + "</td></tr>")
		found++
		if found > adminFindIPShown+1 {
			break
		}
	}
	number := strconv.Itoa(found)
	if found > adminFindIPShown {
		number = "more than 20"
	}
	html := fillPage(l.adminHTML("ipfind.htm"), "%number%", number, "%ip%", ip, "%results%", b.String())
	l.sendFilePage(gm, 0, adminFile("ipfind.htm"), html, 0)
}

// findAccount opens the account of the online player called name with its
// other characters, or answers that no such player is online.
func (l *GameClientLink) findAccount(gm *livePlayer, name string) {
	p, ok := l.livePlayerByName(name)
	if !ok {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetCantFound))
		return
	}
	// The account's other characters are read once when p enters the
	// world and never change after.
	others := make([]string, len(p.storage.accountChars))
	for i, c := range p.storage.accountChars {
		others[i] = c.Name
	}
	html := fillPage(l.adminHTML("accountinfo.htm"), "%name%", name, "%characters%", strings.Join(others, "<br1>"), "%account%", p.AccountName())
	l.sendFilePage(gm, 0, adminFile("accountinfo.htm"), html, 0)
}

// findDualbox opens the addresses at least multibox connected players
// share, in address order.
func (l *GameClientLink) findDualbox(gm *livePlayer, multibox int) {
	perIP := map[string]int{}
	var shared []string
	for _, p := range l.worldPlayersByID() {
		if p.clientDetached() {
			continue
		}
		perIP[p.remoteIP]++
		if perIP[p.remoteIP] == multibox {
			shared = append(shared, p.remoteIP)
		}
	}
	slices.Sort(shared)
	var b strings.Builder
	for _, ip := range shared {
		// Each address shows the threshold it reached, not its count.
		b.WriteString(`<a action="bypass -h admin_find ip ` + ip + `">` + ip + " (" + strconv.Itoa(multibox) + ")</a><br1>")
	}
	html := fillPage(l.adminHTML("dualbox.htm"), "%multibox%", strconv.Itoa(multibox), "%results%", b.String())
	l.sendFilePage(gm, 0, adminFile("dualbox.htm"), html, 0)
}

// searchMatches reports whether name, lower-cased, contains every
// space-separated word of search, lower-cased.
func searchMatches(name, search string) bool {
	name = strings.ToLower(name)
	for _, word := range strings.Split(strings.ToLower(search), " ") {
		if !strings.Contains(name, word) {
			return false
		}
	}
	return true
}

// findItems opens page of the item templates search matches, in id order,
// seven to a page, each with an amount box giving it. It reports false,
// sending nothing, for a page below 1 when items are found.
func (l *GameClientLink) findItems(gm *livePlayer, page int, search string) bool {
	var found []*item.Template
	for _, tpl := range l.itemTemplates.All() {
		if searchMatches(tpl.Name, search) {
			found = append(found, tpl)
		}
	}
	pg, ok := handleradmin.Paginate(len(found), page, adminFindTablePageLimit)
	if !ok {
		return false
	}
	var b strings.Builder
	b.WriteString("<html><body>")
	handleradmin.Search(&b, "bypass admin_find item", 45, len(found))
	for row, tpl := range found[pg.Start:pg.End] {
		if row%2 == 0 {
			b.WriteString("<table width=280 height=22 bgcolor=000000><tr>")
		} else {
			b.WriteString("<table width=280><tr>")
		}
		id := strconv.Itoa(int(tpl.ID))
		quest := ""
		if tpl.EtcItem != nil && tpl.EtcItem.IsQuestItem() {
			quest = " (Quest)"
		}
		b.WriteString("<td width=36 height=41 align=center><table><tr><td><img src=icon.noimage width=32 height=32></td></tr></table></td>")
		b.WriteString("<td width=160>" + trimAndDress(tpl.Name, 28) + `<br1><font color="B09878">Item Id:</font> <font color=BDB76B>` + id + quest + "</font></td>")
		b.WriteString(`<td><edit var="amount_` + id + `" width=52 type=number></td>`)
		b.WriteString(`<td><button action="bypass admin_give ` + id + " $amount_" + id + `" width=32 height=32 back=L2UI_CH3.mapbutton_zoomin2 fore=L2UI_CH3.mapbutton_zoomin1></td>`)
		b.WriteString(`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`)
	}
	pg.Space(&b, 42)
	pg.Links(&b, "bypass admin_find item %page% "+search)
	b.WriteString("</body></html>")
	sendValidatedHTML(gm, 0, b.String(), 0)
	return true
}

// findNPCs opens page of the named NPC templates search matches, by name
// then id, seven to a page, each with spawn and spawn-list buttons. It
// reports false, sending nothing, for a page below 1 when NPCs are found.
func (l *GameClientLink) findNPCs(gm *livePlayer, page int, search string) bool {
	var found []*npc.Template
	for _, tpl := range l.npcs.All() {
		if tpl.Name != "" && searchMatches(tpl.Name, search) {
			found = append(found, tpl)
		}
	}
	slices.SortStableFunc(found, func(a, b *npc.Template) int { return strings.Compare(a.Name, b.Name) })
	pg, ok := handleradmin.Paginate(len(found), page, adminFindTablePageLimit)
	if !ok {
		return false
	}
	var b strings.Builder
	b.WriteString("<html><body>")
	handleradmin.Search(&b, "bypass admin_find npc", 45, len(found))
	for row, tpl := range found[pg.Start:pg.End] {
		if row%2 == 0 {
			b.WriteString("<table width=280 height=22 bgcolor=000000><tr>")
		} else {
			b.WriteString("<table width=280><tr>")
		}
		id := strconv.Itoa(tpl.ID)
		b.WriteString(`<td width=216 height=41><font color="B09878">` + tpl.Title + "</font><br1>" + tpl.Name + "</td>")
		b.WriteString(`<td><button action="bypass admin_spawn ` + id + `" width=32 height=32 back=L2UI_CH3.mapbutton_zoomin2 fore=L2UI_CH3.mapbutton_zoomin1></td>`)
		b.WriteString(`<td><button action="bypass admin_list_spawns ` + id + `" width=32 height=32 back=L2UI_CH3.mapicon_mark_light fore=L2UI_CH3.mapicon_mark></td>`)
		b.WriteString(`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`)
	}
	pg.Space(&b, 42)
	pg.Links(&b, "bypass admin_find npc %page% "+search)
	b.WriteString("</body></html>")
	sendValidatedHTML(gm, 0, b.String(), 0)
	return true
}
