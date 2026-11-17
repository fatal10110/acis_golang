package scenario

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// links are the bypass commands a client may send back: the links of the last
// dialog page the server sent it. The server drops a validated command (npc_,
// admin_ or Quest) that is not one of them, silently, so a bypass step that
// names another leaves nothing to compare and would pass as an empty answer.
type links struct {
	exact  []string
	prefix []string
}

// observe takes one frame the client was sent: a dialog page replaces the
// links, and entering the world forgets them (the server keeps them per live
// player). A page with no NPC (object 0) is one the player asked for or a
// help page, which the server keeps out of the links; it is skipped.
func (l *links) observe(payload []byte) {
	if len(payload) == 0 {
		return
	}
	switch payload[0] {
	case serverpackets.OpcodeCharSelected:
		l.exact, l.prefix = nil, nil
	case serverpackets.OpcodeNpcHtmlMessage:
		rd := wire.NewPacketReader(payload)
		obj := rd.ReadInt32()
		html := rd.ReadString()
		if rd.Err() == nil && obj != 0 {
			l.record(html)
		}
	}
}

// record replaces the links with those of html: "bypass [-h ]<command>"
// inside double quotes. A command with a '$' (an edit box's value follows) is
// a prefix: the text before it.
func (l *links) record(html string) {
	l.exact, l.prefix = nil, nil
	const marker = `"bypass `
	for i := 0; i < len(html); {
		start := strings.Index(html[i:], marker)
		if start < 0 {
			return
		}
		start += i
		end := strings.IndexByte(html[start+1:], '"')
		if end < 0 {
			return
		}
		end += start + 1
		start += len(marker)
		if strings.HasPrefix(html[start:], "-h") {
			start += 3
		}
		i = end + 1
		if start > end {
			continue
		}
		link := html[start:end]
		if dollar := strings.IndexByte(link, '$'); dollar >= 0 {
			if p := strings.TrimFunc(link[:dollar], isBlank); p != "" {
				l.prefix = append(l.prefix, p)
			}
			continue
		}
		l.exact = append(l.exact, strings.TrimFunc(link, isBlank))
	}
}

func isBlank(r rune) bool { return r <= ' ' }

// validated reports whether the server checks command against the links: the
// npc_, admin_ and Quest families. The others are not page links.
func validated(command string) bool {
	return strings.HasPrefix(command, "npc_") || strings.HasPrefix(command, "admin_") || strings.HasPrefix(command, "Quest ")
}

// offers reports whether command is one of the links.
func (l *links) offers(command string) bool {
	for _, e := range l.exact {
		if e == command {
			return true
		}
	}
	for _, p := range l.prefix {
		if strings.HasPrefix(command, p) {
			return true
		}
	}
	return false
}

// track has the run follow the links c is offered.
func (r *run) track(c *testsupport.ScriptedClient) {
	if r.links == nil {
		r.links = map[*testsupport.ScriptedClient]*links{}
	}
	l := &links{}
	r.links[c] = l
	c.SetAfterRead(l.observe)
}

// offeredDiff is what is wrong with sending command from c, "" when nothing:
// the server drops a validated command the last page c received did not
// offer, and answers nothing, so an unmarked step that sends one passes
// without having done anything, and a step marked rejected must send one.
func (r *run) offeredDiff(c *testsupport.ScriptedClient, command string, rejected bool) string {
	l := r.links[c]
	if l == nil {
		l = &links{}
	}
	offered := !validated(command) || l.offers(command)
	switch {
	case rejected && offered && !validated(command):
		return "marked rejected, but the server does not check " + command + " against a page's links"
	case rejected && offered:
		return "marked rejected, but the last page the client received offers " + command
	case !rejected && !offered:
		return "the last page the client received does not offer " + command + ": the server drops it; follow an offered link or mark the step rejected"
	}
	return ""
}
