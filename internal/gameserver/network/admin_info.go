package network

import (
	"strconv"
	"strings"

	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
)

// adminInfo answers //info [args] on gm's selection, gm itself without one: a
// player is shown its character page, a door, an NPC, a summon or a static
// object its own page. Any other selection gets an HTML window with no page,
// which still drops the links of the last one.
func (l *GameClientLink) adminInfo(gm *livePlayer, line string) {
	switch target := gm.Target().(type) {
	case nil:
		l.showCharInfo(gm, gm)
	case *livePlayer:
		l.showCharInfo(gm, target)
	case *door.Object:
		l.sendFilePage(gm, 0, adminFile("doorinfo.htm"), l.doorInfoPage(target), 0)
	case *summon.Actor:
		l.sendFilePage(gm, 0, adminFile("petinfo.htm"), l.summonInfoPage(target), 0)
	case *staticobject.Object:
		l.sendFilePage(gm, 0, adminFile("staticinfo.htm"), l.staticInfoPage(target), 0)
	default:
		if inst := npcInstanceOf(target); inst != nil {
			l.adminNpcInfo(gm, target, inst, handleradmin.SpaceArgs(line))
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

// formatPercent formats v with at most three fraction digits, rounded
// half-even on v's exact value, no trailing zeros, no integer digit before
// the point of a value below one, and a lone "0" (signed when v is
// negative) for a value that rounds to zero.
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
