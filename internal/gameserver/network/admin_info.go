package network

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
)

// adminInfo answers //info [args] on gm's selection, gm itself without one
// (AdminInfo.java): a player is shown its character page, a door, an NPC,
// a summon or a static object its own page. Any other selection gets an
// HTML window with no page, which still drops the links of the last one.
func (l *GameClientLink) adminInfo(gm *livePlayer, line string) {
	switch target := gm.Target().(type) {
	case nil:
		l.showCharInfo(gm, gm)
	case *livePlayer:
		l.showCharInfo(gm, target)
	case *door.Object:
		sendFilledHTML(gm, 0, l.doorInfoPage(target), 0)
	case *summon.Actor:
		sendFilledHTML(gm, 0, l.summonInfoPage(target), 0)
	case *staticobject.Object:
		sendFilledHTML(gm, 0, l.staticInfoPage(target), 0)
	default:
		if inst := npcInstanceOf(target); inst != nil {
			l.adminNpcInfo(gm, target, inst, javaTokens(line)[1:])
			return
		}
		sendFilledHTML(gm, 0, "", 0)
	}
}

// staticInfoPage is obj's static object page: where it stands, its object
// and static object ids, and its class.
func (l *GameClientLink) staticInfoPage(obj *staticobject.Object) string {
	x, y, z := obj.Position()
	return fillPage(l.adminHTML("staticinfo.htm"),
		"%x%", strconv.Itoa(x),
		"%y%", strconv.Itoa(y),
		"%z%", strconv.Itoa(z),
		"%objid%", strconv.Itoa(int(obj.ObjectID())),
		"%staticid%", strconv.Itoa(obj.StaticObjectID()),
		"%class%", "StaticObject",
	)
}

// javaTokens splits line on spaces the way a StringTokenizer with a space
// delimiter does: runs of spaces separate tokens, and any other whitespace
// stays inside one. The first token is the command word, so the result
// always holds at least one entry.
func javaTokens(line string) []string {
	tokens := strings.FieldsFunc(line, func(r rune) bool { return r == ' ' })
	if len(tokens) == 0 {
		return []string{""}
	}
	return tokens
}

// formatPercent formats v the way DecimalFormat("#.###") does: at most three
// fraction digits rounded half-even on v's exact value, no trailing zeros,
// no integer digit before the point of a value below one, and a lone "0"
// (signed when v is negative) for a value that rounds to zero.
func formatPercent(v float64) string {
	s := strconv.FormatFloat(v, 'f', 3, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign, s = "-", s[1:]
	}
	if s == "0" {
		return sign + "0"
	}
	return sign + strings.TrimPrefix(s, "0")
}
