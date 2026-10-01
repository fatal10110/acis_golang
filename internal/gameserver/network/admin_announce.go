package network

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/handler/chat"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
)

// adminAnnounce answers //announce <list|all|all_auto|add|add_auto|del>.
// A missing or malformed argument opens the announcements panel instead.
//
//   - list shows the held announcements.
//   - all reads every player online the login announcements; all_auto
//     starts every automatic announcement's schedule over. Both then show
//     the list.
//   - add <critical> <message> adds a login announcement.
//   - add_auto <critical> <auto> <initial delay> <delay> <limit> <message>
//     adds an automatic announcement, or a login one when auto is false.
//   - del <index> deletes an announcement.
func (l *GameClientLink) adminAnnounce(gm *livePlayer, line string) {
	if !l.announceCommand(gm, line) {
		l.sendAdminFile(gm, "announce.htm")
	}
}

// announceCommand runs one //announce, reporting false when an argument is
// missing or malformed or the list cannot be shown.
func (l *GameClientLink) announceCommand(gm *livePlayer, line string) bool {
	tokens := strings.SplitN(line, " ", 3)
	if len(tokens) < 2 {
		return false
	}
	switch tokens[1] {
	case "list":
		return l.listAnnouncements(gm)
	case "all", "all_auto":
		if tokens[1] == "all_auto" {
			l.announcements.RestartAuto()
		} else if l.world != nil {
			for _, p := range l.world.Players() {
				if listener, ok := p.(*livePlayer); ok {
					l.sendLoginAnnouncements(listener)
				}
			}
		}
		return l.listAnnouncements(gm)
	case "add":
		if len(tokens) < 3 {
			return false
		}
		args := strings.SplitN(tokens[2], " ", 2)
		if len(args) < 2 {
			return false
		}
		l.addAnnouncement(gm, admin.Announcement{Message: args[1], Critical: javaBool(args[0])})
		return l.listAnnouncements(gm)
	case "add_auto":
		if len(tokens) < 3 {
			return false
		}
		args := strings.SplitN(tokens[2], " ", 6)
		if len(args) < 6 {
			return false
		}
		var numbers [3]int
		for i, arg := range args[2:5] {
			n, ok := parseJavaInt(arg)
			if !ok {
				return false
			}
			numbers[i] = int(n)
		}
		l.addAnnouncement(gm, admin.Announcement{
			Message:      args[5],
			Critical:     javaBool(args[0]),
			Auto:         javaBool(args[1]),
			InitialDelay: numbers[0],
			Delay:        numbers[1],
			Limit:        numbers[2],
		})
		return l.listAnnouncements(gm)
	case "del":
		if len(tokens) < 3 {
			return false
		}
		index, ok := parseJavaInt(tokens[2])
		if !ok || !l.announcements.Delete(int(index)) {
			return false
		}
		return l.listAnnouncements(gm)
	default:
		sendText(gm, "Possible //announce parameters : <list|all|add|add_auto|del>")
		return true
	}
}

// addAnnouncement adds a, telling gm when its message is empty.
func (l *GameClientLink) addAnnouncement(gm *livePlayer, a admin.Announcement) {
	if !l.announcements.Add(a) {
		sendText(gm, "Invalid //announce message content ; can't be null or empty.")
	}
}

// listAnnouncements shows gm the held announcements, each with its delete
// button. It reports false, showing nothing, when a message cannot stand
// in the page's placeholder.
func (l *GameClientLink) listAnnouncements(gm *livePlayer) bool {
	list := l.announcements.List()
	var b strings.Builder
	b.WriteString("<br>")
	if len(list) == 0 {
		b.WriteString("<tr><td>The XML file doesn't contain any content.</td></tr>")
	}
	for _, a := range list {
		index := strconv.Itoa(a.Index)
		b.WriteString("<table width=260><tr><td width=240>#" + index + " - " + a.Message +
			"</td><td></td></tr></table><table width=260><tr><td>Critical: " + strconv.FormatBool(a.Critical) +
			" | Auto: " + strconv.FormatBool(a.Auto) +
			"</td><td><button value=\"Delete\" action=\"bypass -h admin_announce del " + index +
			"\" width=65 height=19 back=\"L2UI_ch3.smallbutton2_over\" fore=\"L2UI_ch3.smallbutton2\"></td></tr></table>")
	}
	const placeholder = "%announces%"
	content, ok := placeholderValue(b.String(), placeholder)
	if !ok {
		return false
	}
	const file = "data/html/admin/announce_list.htm"
	page, found := l.html.Get(file)
	if !found {
		page = "<html><body>My html is missing:<br>" + file + "</body></html>"
	}
	sendValidatedHTML(gm, 0, strings.ReplaceAll(page, placeholder, content), 0)
	return true
}

// placeholderValue is value as it stands in for placeholder in a page.
// A dollar sign stands as it is. A backslash drops and keeps the character
// after it as it is, except a dollar sign: that keeps the backslash and
// then must be followed by a zero, together standing for the placeholder
// itself. A trailing backslash, or a backslash and dollar sign followed by
// anything else, cannot stand.
func placeholderValue(value, placeholder string) (string, bool) {
	if !strings.ContainsRune(value, '\\') {
		return value, true
	}
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		i++
		switch {
		case i == len(value):
			return "", false
		case value[i] != '$':
			b.WriteByte(value[i])
		case i+1 < len(value) && value[i+1] == '0':
			b.WriteByte('\\')
			b.WriteString(placeholder)
			i++
		default:
			return "", false
		}
	}
	return b.String(), true
}

// adminAnnounceText answers //ann <text> and //say <text>: text, said by
// no one, to every player online, on the critical announcement channel for
// //say. A command with no text does nothing.
func (l *GameClientLink) adminAnnounceText(_ *livePlayer, line string) {
	// "admin_ann " and "admin_say " are both ten characters long.
	const prefix = len("admin_ann ")
	if len(line) <= prefix {
		return
	}
	announceToOnline(l.world, line[prefix:], strings.HasPrefix(line, "admin_say"))
}

// adminGMChat answers //gmchat <text>: gm's line, on the alliance channel,
// to every game master online, listed or not.
func (l *GameClientLink) adminGMChat(gm *livePlayer, line string) {
	const prefix = len("admin_gmchat ")
	if len(line) < prefix {
		sendText(gm, "Invalid //gmchat message content ; can't be null or empty.")
		return
	}
	line = line[prefix:]
	broadcastFrame(frameCreatureSay(gm, chat.Line{Type: chat.Alliance, Text: line}), func(send func(frameReceiver)) {
		for _, entry := range l.gms.Entries(true) {
			send(entry.Player)
		}
	})
}

// javaBool reads s as a flag: "true" in any case is true, anything else
// false.
func javaBool(s string) bool {
	return strings.EqualFold(s, "true")
}
