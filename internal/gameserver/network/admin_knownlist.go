package network

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/boat"
	"github.com/fatal10110/acis_golang/internal/gameserver/fence"
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// adminKnownlistPageLimit is how many objects a page of //knownlist shows.
const adminKnownlistPageLimit = 15

// adminKnownlist answers //knownlist [page]: the objects around gm's target,
// or around gm without one, fifteen to a page, each by name and class. A
// page that does not read as an int is answered with the usage and shows
// the first page.
func (l *GameClientLink) adminKnownlist(gm *livePlayer, line string) {
	var target world.Tracked = gm
	if t := gm.Target(); t != nil {
		target = t
	}
	page := int32(1)
	if args := spaceTokens(line)[1:]; len(args) > 0 {
		if n, ok := parseInt32(args[0]); ok {
			page = n
		} else {
			sendText(gm, "Usage: //knownlist [page]")
		}
	}
	name, ok := l.objectName(target)
	if !ok {
		// The page names the target, and this one has no name to show:
		// the page is not sent, as no page is when it cannot be built.
		l.log.Warn().Int32("target", target.ObjectID()).Msg("admin: //knownlist target has no name")
		return
	}
	known := l.world.AppendKnown(nil, target)
	pg, ok := handleradmin.Paginate(len(known), int(page), adminKnownlistPageLimit)
	if !ok {
		// No page below the first exists; the command answers nothing.
		l.log.Warn().Int32("page", page).Msg("admin: //knownlist page out of range")
		return
	}
	var b strings.Builder
	for row, obj := range known[pg.Start:pg.End] {
		if row%2 == 0 {
			b.WriteString("<table width=280 bgcolor=000000><tr>")
		} else {
			b.WriteString("<table width=280><tr>")
		}
		name, ok := l.objectName(obj)
		if !ok {
			name = "null"
		}
		b.WriteString("<td width=160>" + name + "</td><td width=120>" + objectClassName(obj) + "</td>")
		b.WriteString(`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`)
	}
	pg.Space(&b, 20)
	pg.Links(&b, "bypass admin_knownlist %page%")
	html := fillPage(l.adminHTML("knownlist.htm"), "%target%", name, "%content%", b.String())
	l.sendFilePage(gm, 0, adminFile("knownlist.htm"), html, 0)
}

// objectName is the name admin pages show for obj: a character's or NPC's
// name, a door's, a ground item's template name. Boats, static objects,
// fences and pets never named have none.
func (l *GameClientLink) objectName(obj world.Tracked) (string, bool) {
	if inst := npcInstanceOf(obj); inst != nil {
		return inst.Name(), true
	}
	switch o := obj.(type) {
	case *livePlayer:
		return o.CharacterName(), true
	case *summon.Actor:
		// A pet goes by its template name until its owner names it, but
		// has no name of its own to show here before.
		if _, st, ok := o.PetState(); ok && st.Name == "" {
			return "", false
		}
		return o.Name(), true
	case *door.Object:
		return o.CharacterName(), true
	case *grounditem.Item:
		if tpl, ok := l.itemTemplates.Get(o.ItemID()); ok {
			return tpl.Name, true
		}
	}
	return "", false
}

// objectClassName is the instance class name admin pages print for obj.
func objectClassName(obj world.Tracked) string {
	if inst := npcInstanceOf(obj); inst != nil {
		return npcClassName(inst)
	}
	switch o := obj.(type) {
	case *livePlayer:
		return "Player"
	case *summon.Actor:
		return summonClassName(o)
	case *door.Object:
		return "Door"
	case *grounditem.Item:
		return "ItemInstance"
	case *staticobject.Object:
		return "StaticObject"
	case *boat.Boat:
		return "Boat"
	case *fence.Fence:
		return "Fence"
	case *fence.Layer:
		return "L2DummyFence"
	}
	return ""
}
