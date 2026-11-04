package network

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"

	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

const showUsage = "Usage : //show <clear|door|html|move|path|restart|walker>"

// walkerPageLimit is how many walker routes a page of //show walker lists.
const walkerPageLimit = 15

// adminShow answers //show <boat|clear|door|html|move|path|restart|walker>:
// debug drawings of the world's data, and the move and path debug toggles.
// A missing or unknown subcommand, like any argument the subcommand cannot
// use, answers the usage.
func (l *GameClientLink) adminShow(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	if len(args) == 0 {
		sendText(gm, showUsage)
		return
	}
	switch strings.ToLower(args[0]) {
	case "boat":
		l.showBoats(gm)
	case "clear":
		// Only a player holds drawings. Another creature selected, or a
		// player without drawings, clears nothing and nothing answers, as
		// the command has always done; an admin command leaves no client
		// action pending.
		if p, ok := adminTargetCreature(gm).(*livePlayer); ok {
			onPlayer(gm, p, p.clearDrawings)
		}
	case "door":
		l.showDoors(gm)
	case "html":
		// Flipping the toggle answers nothing, as the command has always
		// done; an admin command leaves no client action pending.
		for {
			on := l.showPageFiles.Load()
			if l.showPageFiles.CompareAndSwap(on, !on) {
				break
			}
		}
	case "move":
		l.toggleDebugMove(gm, false)
	case "path":
		l.toggleDebugMove(gm, true)
	case "restart":
		// Without a known kind nothing is drawn and nothing answers, as
		// the command has always done; an admin command leaves no client
		// action pending.
		if len(args) < 2 {
			return
		}
		switch args[1] {
		case "area":
			l.showRestartAreas(gm)
		case "point":
			l.showRestartPoints(gm)
		}
	case "walker":
		l.showWalker(gm, args[1:])
	default:
		sendText(gm, showUsage)
	}
}

// adminTargetCreature is the creature gm has selected, gm itself when the
// selection is none or no creature.
func adminTargetCreature(gm *livePlayer) world.Tracked {
	switch t := gm.Target().(type) {
	case *livePlayer, *door.Object, *summon.Actor:
		return t
	}
	if npcInstanceOf(gm.Target()) != nil {
		return gm.Target()
	}
	return gm
}

// creatureName is the name of a creature adminTargetCreature resolves.
func creatureName(c world.Tracked) string {
	switch t := c.(type) {
	case *livePlayer:
		return t.CharacterName()
	case *door.Object:
		return t.Template.Name
	case *summon.Actor:
		return t.Name()
	}
	if inst := npcInstanceOf(c); inst != nil {
		return inst.Name()
	}
	return ""
}

// clearDrawings blanks every debug drawing live holds, by name, then
// forgets them all. It runs on live's queue.
func (live *livePlayer) clearDrawings() {
	names := make([]string, 0, len(live.drawings))
	for name := range live.drawings {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		d := live.drawings[name]
		d.Reset()
		sendDrawing(live, d)
	}
	clear(live.drawings)
}

// blankDrawing blanks live's drawing name. It runs on live's queue.
func (live *livePlayer) blankDrawing(name string) {
	d := live.drawing(name)
	d.Reset()
	sendDrawing(live, d)
}

// showBoats draws every boat route: one yellow segment between each two
// nodes, the last node closing none.
func (l *GameClientLink) showBoats(gm *livePlayer) {
	d := gm.drawing("BOAT")
	d.Reset()
	if l.boats != nil {
		for _, it := range l.boats.Itineraries() {
			for _, r := range it.Routes {
				for i := 0; i+1 < len(r.Nodes); i++ {
					d.AddLine(segment("Segment #"+strconv.Itoa(i), r.Nodes[i].Location, r.Nodes[i+1].Location))
				}
			}
		}
	}
	sendDrawing(gm, d)
}

// segment is a labelled yellow line from a to b.
func segment(label string, a, b location.Location) serverpackets.PrimitiveLine {
	return serverpackets.PrimitiveLine{
		PrimitivePoint: serverpackets.PrimitivePoint{Name: label, Color: serverpackets.PrimitiveYellow, NameColored: true, X: int32(a.X), Y: int32(a.Y), Z: int32(a.Z)},
		X2:             int32(b.X), Y2: int32(b.Y), Z2: int32(b.Z),
	}
}

// showDoors draws the outline of every door gm knows, by object id: each
// edge three times, 32 below its height in green, at its height in yellow
// and 32 above in red.
func (l *GameClientLink) showDoors(gm *livePlayer) {
	d := gm.drawing("DOOR")
	d.Reset()
	var doors []*door.Object
	if l.world != nil {
		l.world.ForEachKnown(gm, func(o world.Tracked) {
			if dr, ok := o.(*door.Object); ok {
				doors = append(doors, dr)
			}
		})
	}
	slices.SortFunc(doors, func(a, b *door.Object) int { return cmp.Compare(a.ObjectID(), b.ObjectID()) })
	for _, dr := range doors {
		t := dr.Template
		z := int32(t.Position.Z)
		for i, cur := range t.Coordinates {
			next := t.Coordinates[(i+1)%len(t.Coordinates)]
			for _, edge := range []struct {
				label string
				color uint32
				z     int32
			}{{t.Name + " MinZ", serverpackets.PrimitiveGreen, z - 32}, {t.Name, serverpackets.PrimitiveYellow, z}, {t.Name + " MaxZ", serverpackets.PrimitiveRed, z + 32}} {
				d.AddLine(serverpackets.PrimitiveLine{
					PrimitivePoint: serverpackets.PrimitivePoint{Name: edge.label, Color: edge.color, NameColored: true, X: int32(cur.X), Y: int32(cur.Y), Z: edge.z},
					X2:             int32(next.X), Y2: int32(next.Y), Z2: edge.z,
				})
			}
		}
	}
	sendDrawing(gm, d)
}

// showRestartAreas outlines every restart area, unlabelled.
func (l *GameClientLink) showRestartAreas(gm *livePlayer) {
	d := gm.drawing("RESTART_AREA")
	d.Reset()
	if l.restarts != nil {
		for _, a := range l.restarts.Areas {
			d.AddZoneOutline("", zone.Outline(a.Form()))
		}
	}
	sendDrawing(gm, d)
}

// showRestartPoints marks every restart point's spots green and its
// chaotic spots red.
func (l *GameClientLink) showRestartPoints(gm *livePlayer) {
	d := gm.drawing("RESTART_POINT")
	d.Reset()
	if l.restarts != nil {
		for _, p := range l.restarts.Points {
			for _, at := range p.Points {
				d.AddPoint(serverpackets.PrimitivePoint{Name: "point", Color: serverpackets.PrimitiveGreen, NameColored: true, X: int32(at.X), Y: int32(at.Y), Z: int32(at.Z)})
			}
			for _, at := range p.ChaoPoints {
				d.AddPoint(serverpackets.PrimitivePoint{Name: "chaoPoint", Color: serverpackets.PrimitiveRed, NameColored: true, X: int32(at.X), Y: int32(at.Y), Z: int32(at.Z)})
			}
		}
	}
	sendDrawing(gm, d)
}

// debugMoves holds the creatures that cannot move (doors) whose moves or
// paths are debugged; a creature that moves holds its own toggles, which
// leave the world with it.
type debugMoves struct {
	mu   sync.Mutex
	move map[world.Tracked]bool
	path map[world.Tracked]bool
}

// toggle flips c's move (or path) debugging and reports whether it is now
// on.
func (m *debugMoves) toggle(c world.Tracked, path bool) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	set := &m.move
	if path {
		set = &m.path
	}
	if (*set)[c] {
		delete(*set, c)
		return false
	}
	if *set == nil {
		*set = map[world.Tracked]bool{}
	}
	(*set)[c] = true
	return true
}

// toggleDebugMove flips the move (or path) debugging of gm's selected
// creature, telling gm and the creature. Turned off, the creature's move
// (or path) drawing is blanked for every game master and for the creature.
func (l *GameClientLink) toggleDebugMove(gm *livePlayer, path bool) {
	target := adminTargetCreature(gm)
	word, drawing := "move", "MOVE"
	if path {
		word, drawing = "path", "PATH"
	}
	state := "disabled"
	var on bool
	if m, ok := target.(debugMover); ok && m.Move() != nil {
		on = m.Move().ToggleDebug(path)
	} else {
		on = l.debugMoves.toggle(target, path)
	}
	if on {
		state = "enabled"
	}
	sendText(gm, "Debug "+word+" "+state+" on "+creatureName(target))
	p, isPlayer := target.(*livePlayer)
	if isPlayer && p != gm {
		onPlayer(gm, p, func() { sendText(p, "Debug "+word+" was "+state+".") })
	}
	if on {
		return
	}
	name := drawing + strconv.Itoa(int(target.ObjectID()))
	if l.world != nil {
		for _, o := range l.world.Players() {
			if g, ok := o.(*livePlayer); ok && g.accessLevel().IsGM {
				onPlayer(gm, g, func() { g.blankDrawing(name) })
			}
		}
	}
	if isPlayer {
		onPlayer(gm, p, func() { p.blankDrawing(name) })
	}
}

// walkerRoutes returns the loaded walker routes and the walker walking
// them, nil without one.
func (l *GameClientLink) walkerRoutes() (route.WalkerRoutes, *task.Walker) {
	npcs := l.npcSpawns.Load()
	if npcs == nil {
		return nil, nil
	}
	w := npcs.Walker()
	if w == nil {
		return nil, nil
	}
	return w.Routes(), w
}

// showWalker answers //show walker [page] [route]: the route named both as
// route and NPC drawn as a closed yellow loop, then the walker panel at
// page, 1 by default. A route with no node under that NPC name answers
// the usage, one unknown or empty that no walker route links the name.
func (l *GameClientLink) showWalker(gm *livePlayer, args []string) {
	page := 1
	if len(args) > 0 && isDigits(args[0]) {
		n, ok := parseInt32(args[0])
		if !ok {
			sendText(gm, showUsage)
			return
		}
		page = int(n)
		args = args[1:]
	}
	if len(args) == 0 {
		l.sendWalkerPanel(gm, page)
		return
	}
	param := args[0]
	routes, _ := l.walkerRoutes()
	byNPC, known := routes[param]
	nodes, linked := byNPC[param]
	if known && !linked {
		sendText(gm, showUsage)
		return
	}
	if len(nodes) == 0 {
		sendText(gm, "The npcId "+param+" isn't linked to any WalkerRoute.")
		return
	}
	d := gm.drawing("WALKER")
	d.Reset()
	for i := range nodes {
		next := (i + 1) % len(nodes)
		d.AddLine(segment("Segment #"+strconv.Itoa(i+1), nodes[i].Location, nodes[next].Location))
	}
	sendDrawing(gm, d)
	l.sendWalkerPanel(gm, page)
}

// walkerRow is one row of the walker panel: the NPC name a route is
// keyed under and its nodes.
type walkerRow struct {
	npc   string
	nodes []route.WalkerLocation
}

// walkerRowNPC is the NPC the walker panel row of the NPC alias shows:
// the one of the first spawn of that name, its name, the route node it
// heads for and its direction; false when that spawn keeps none.
func (l *GameClientLink) walkerRowNPC(alias string, walker *task.Walker) (name string, index int, reverse, ok bool) {
	npcs := l.npcSpawns.Load()
	if npcs == nil {
		return "", 0, false, false
	}
	kept, ok := npcs.FirstNPCOfAlias(alias)
	if !ok {
		return "", 0, false, false
	}
	if kept.Decayed {
		return kept.Name, kept.Index, kept.Reverse, true
	}
	obj, ok := l.world.Object(kept.ID)
	if !ok {
		return "", 0, false, false
	}
	if walker != nil {
		index, reverse, _ = walker.Walking(kept.ID)
	}
	return creatureName(obj), index, reverse, true
}

// sendWalkerPanel opens page of the walker panel: every walker route, by
// route name then NPC name, the NPC of the first spawn of that name with
// the node it heads for and its direction, node 0 the regular way when it
// never walked its route, or unspawned without one. A dead walker keeps
// its node and direction, through its decay while its spawn waits to
// respawn it. A page below 1 with routes listed, or a
// walker heading for a node its listed route lacks, answers the usage.
func (l *GameClientLink) sendWalkerPanel(gm *livePlayer, page int) {
	routes, walker := l.walkerRoutes()
	var rows []walkerRow
	for _, name := range slices.Sorted(maps.Keys(routes)) {
		byNPC := routes[name]
		for _, npcName := range slices.Sorted(maps.Keys(byNPC)) {
			rows = append(rows, walkerRow{npc: npcName, nodes: byNPC[npcName]})
		}
	}
	pg, ok := handleradmin.Paginate(len(rows), page, walkerPageLimit)
	if !ok {
		sendText(gm, showUsage)
		return
	}
	var b strings.Builder
	for row, e := range rows[pg.Start:pg.End] {
		if row%2 == 0 {
			b.WriteString("<table width=280 bgcolor=000000><tr>")
		} else {
			b.WriteString("<table width=280><tr>")
		}
		show := `<td width=40 align=right><a action="bypass admin_show walker ` + strconv.Itoa(page) + " " + e.npc + `">Show</a></td>`
		name, index, reverse, spawned := l.walkerRowNPC(e.npc, walker)
		if !spawned {
			b.WriteString(`<td width=150><a action="bypass admin_teleport ` + teleportLoc(e.nodes[0].Location) + `">Unspawned</a></td><td width=40>-</td><td width=50 align=right>-</td>` + show)
		} else {
			if index >= len(e.nodes) {
				sendText(gm, showUsage)
				return
			}
			direction := "Regular"
			if reverse {
				direction = "Reverse"
			}
			b.WriteString(`<td width=150><a action="bypass admin_teleport ` + teleportLoc(e.nodes[index].Location) + `">` + trimAndDress(name, 25) + "</a></td><td width=40>" + strconv.Itoa(index) + " / " + strconv.Itoa(len(e.nodes)) + "</td><td width=50 align=right>" + direction + "</td>" + show)
		}
		b.WriteString(`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`)
	}
	pg.Space(&b, 20)
	pg.Links(&b, "bypass admin_show walker %page%")
	const file = "data/html/admin/walker.htm"
	html, found := l.html.Get(file)
	if !found {
		html = "<html><body>My html is missing:<br>" + file + "</body></html>"
	}
	l.sendValidatedFile(gm, 0, file, strings.ReplaceAll(html, "%content%", b.String()), 0)
}

// teleportLoc is at as the //teleport arguments: x y z.
func teleportLoc(at location.Location) string {
	return strconv.Itoa(at.X) + " " + strconv.Itoa(at.Y) + " " + strconv.Itoa(at.Z)
}
