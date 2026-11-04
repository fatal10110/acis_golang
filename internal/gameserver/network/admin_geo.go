package network

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/pathfind"
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

const (
	geoUsage  = "Usage: //geo bug|pos|see|move|fly"
	pathUsage = "Usage: //path find|info"
	// geoBugPrefix is the command text a //geo bug comment follows.
	geoBugPrefix = "admin_geo bug "
	noGeodata    = "There is no geodata at this position."
)

// standing is a world object that stands somewhere.
type standing interface {
	Position() (x, y, z int)
}

// collisionHeighted is a creature with a body height.
type collisionHeighted interface {
	CollisionHeight() float64
}

// geoQueries returns the geodata engine and path finder the link moves
// creatures on. A link without geodata queries an engine with none loaded,
// and finds no path.
func (l *GameClientLink) geoQueries() (*engine.Engine, *pathfind.Finder) {
	if g, ok := l.geo.(move.EngineGeo); ok && g.Engine != nil {
		return g.Engine, g.Finder
	}
	return engine.New(), nil
}

// adminGeo answers //geo bug|pos|see|move|fly: the geodata under gm and
// between gm and its selection, drawn and told. Without a subcommand it
// answers the usage; an unknown one does nothing, as the command has always
// done, and an admin command leaves no client action pending.
func (l *GameClientLink) adminGeo(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	if len(args) == 0 {
		sendText(gm, geoUsage)
		return
	}
	eng, _ := l.geoQueries()
	switch args[0] {
	case "bug":
		l.geoBug(gm, eng, line)
	case "pos":
		geoPos(gm, eng)
	case "see":
		geoSee(gm, eng)
	case "move":
		geoMove(gm, eng)
	case "fly":
		geoFly(gm, eng)
	}
}

// geoBug saves gm's report of the geodata where it stands, signed with its
// name. Where there is no geodata nothing is saved. A report that cannot
// be written is logged and answers nothing, as the command has always done.
func (l *GameClientLink) geoBug(gm *livePlayer, eng *engine.Engine, line string) {
	x, y, z := gm.Position()
	if !eng.HasGeo(x, y) {
		sendText(gm, noGeodata)
		return
	}
	if len(line) < len(geoBugPrefix) {
		sendText(gm, "Usage: //geo bug comments")
		return
	}
	if l.geoBugs == nil {
		l.log.Error().Str("player", gm.CharacterName()).Msg("admin: no geodata bug report file to save to")
		return
	}
	if err := l.geoBugs.Add(x, y, z, gm.CharacterName()+": "+line[len(geoBugPrefix):]); err != nil {
		l.log.Error().Err(err).Str("player", gm.CharacterName()).Msg("admin: cannot save geodata bug report")
		return
	}
	sendText(gm, "GeoData bug saved.")
}

// geoPos tells gm the geodata region and block it stands in and, where
// there is geodata, the cell's open directions as a compass with its
// geodata coordinates, drawing the cell.
func geoPos(gm *livePlayer, eng *engine.Engine) {
	ox, oy, oz := gm.Position()
	gx, gy := engine.GeoX(ox), engine.GeoY(oy)
	rx, ry := engine.RegionOf(ox, oy)
	sendText(gm, "Region: "+strconv.Itoa(rx)+"_"+strconv.Itoa(ry)+"; Block: "+eng.BlockName(gx, gy))
	if !eng.HasGeo(ox, oy) {
		sendText(gm, noGeodata)
		return
	}
	gz := int(eng.HeightNearest(gx, gy, oz))
	nswe := eng.NSWENearest(gx, gy, gz)
	open := func(dirs ...block.NSWE) string {
		for _, d := range dirs {
			if nswe&d == 0 {
				return "   "
			}
		}
		return "x "
	}
	sendText(gm, "    "+open(block.North, block.West)+open(block.North)+open(block.North, block.East)+"         GeoX="+strconv.Itoa(gx))
	sendText(gm, "    "+open(block.West)+"o "+open(block.East)+"         GeoY="+strconv.Itoa(gy))
	sendText(gm, "    "+open(block.South, block.West)+open(block.South)+open(block.South, block.East)+"         GeoZ="+strconv.Itoa(gz))
	d := gm.drawing("POS")
	d.Reset()
	d.AddSquare(serverpackets.PrimitiveGreen, int32(ox&^15), int32(oy&^15), int32(oz), 15)
	d.AddPoint(serverpackets.PrimitivePoint{Name: "POS", Color: serverpackets.PrimitiveRed, NameColored: true, X: int32(ox), Y: int32(oy), Z: int32(oz)})
	sendDrawing(gm, d)
}

// geoSee draws the line of sight between gm and its selected creature (gm
// itself without one), both ways, each body as a blue line twice its
// collision height: green when both ways see, red otherwise, with the
// obstacle allowance above it in magenta.
func geoSee(gm *livePlayer, eng *engine.Engine) {
	target := adminTargetCreature(gm)
	d := gm.drawing("CAN_SEE")
	d.Reset()
	ox, oy, oz := gm.Position()
	oh := int(2 * gm.CollisionHeight())
	d.AddLine(drawLine("origin", serverpackets.PrimitiveBlue, ox, oy, oz, ox, oy, oz+oh))
	oh = oh * eng.PartOfCharacterHeight() / 100
	tx, ty, tz := positionOf(target)
	th := int(2 * collisionHeightOf(target))
	d.AddLine(drawLine("target", serverpackets.PrimitiveBlue, tx, ty, tz, tx, ty, tz+th))
	th = th * eng.PartOfCharacterHeight() / 100
	var ignore dynamic.Object
	if o, ok := target.(dynamic.Object); ok {
		ignore = o
	}
	tr := &engine.Trace{}
	back := eng.CanSeeOneWayTraced(tx, ty, tz, float64(th), ox, oy, oz, float64(oh), ignore, tr)
	forth := eng.CanSeeOneWayTraced(ox, oy, oz, float64(oh), tx, ty, tz, float64(th), ignore, tr)
	addTrace(d, tr)
	oh += oz
	th += tz
	color := serverpackets.PrimitiveRed
	if back && forth {
		color = serverpackets.PrimitiveGreen
	}
	d.AddLine(drawLine("Line-of-Sight", color, ox, oy, oh, tx, ty, th))
	limit := eng.MaxObstacleHeight()
	d.AddLine(drawLine("Geodata limit", serverpackets.PrimitiveMagenta, ox, oy, oh+limit, tx, ty, th+limit))
	sendDrawing(gm, d)
}

// geoMove tells gm whether it walks a straight line to its selection. The
// walk is drawn but never sent, as the command has always done; a later
// //show clear blanks it.
func geoMove(gm *livePlayer, eng *engine.Engine) {
	target, ok := gm.Target().(standing)
	if !ok {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
		return
	}
	ax, ay, az := gm.Position()
	tx, ty, tz := target.Position()
	d := gm.drawing("CAN_MOVE")
	d.Reset()
	tr := &engine.Trace{}
	loc := eng.ValidLocationTraced(ax, ay, az, tx, ty, tz, tr)
	addTrace(d, tr)
	d.AddLine(drawLine("Can move", serverpackets.PrimitiveGreen, ax, ay, az, loc.X, loc.Y, loc.Z))
	if loc == (location.Location{X: tx, Y: ty, Z: tz}) {
		sendText(gm, "Can move beeline.")
		return
	}
	d.AddLine(serverpackets.PrimitiveLine{PrimitivePoint: serverpackets.PrimitivePoint{Color: serverpackets.PrimitiveWhite, X: int32(ax), Y: int32(ay), Z: int32(az)}, X2: int32(tx), Y2: int32(ty), Z2: int32(tz)})
	d.AddLine(drawLine("Inaccessible", serverpackets.PrimitiveRed, loc.X, loc.Y, loc.Z, tx, ty, tz))
	d.AddPoint(serverpackets.PrimitivePoint{Name: "Limit", Color: serverpackets.PrimitiveRed, NameColored: true, X: int32(loc.X), Y: int32(loc.Y), Z: int32(loc.Z)})
	sendText(gm, "Can not move beeline!")
}

// geoFly draws the flight corridor, twice gm's collision height tall, from
// gm to its selected creature (gm itself without one), 32 below them both,
// and tells gm whether it flies there in a straight line.
func geoFly(gm *livePlayer, eng *engine.Engine) {
	target := adminTargetCreature(gm)
	d := gm.drawing("CAN_FLY")
	d.Reset()
	ox, oy, oz := gm.Position()
	oh := int(2 * gm.CollisionHeight())
	d.AddLine(drawLine("origin", serverpackets.PrimitiveBlue, ox, oy, oz-32, ox, oy, oz+oh-32))
	tx, ty, tz := positionOf(target)
	tr := &engine.Trace{}
	loc := eng.ValidFlyLocationTraced(ox, oy, oz, float64(oh), tx, ty, tz, tr)
	addTrace(d, tr)
	canFly := loc == (location.Location{X: tx, Y: ty, Z: tz})
	d.AddLine(drawLine("Can fly", serverpackets.PrimitiveGreen, ox, oy, oz-32, loc.X, loc.Y, loc.Z-32))
	color := serverpackets.PrimitiveGreen
	if canFly {
		sendText(gm, "Can fly beeline.")
	} else {
		color = serverpackets.PrimitiveRed
		sendText(gm, "Can not fly beeline!")
		d.AddLine(serverpackets.PrimitiveLine{PrimitivePoint: serverpackets.PrimitivePoint{Color: serverpackets.PrimitiveWhite, X: int32(ox), Y: int32(oy), Z: int32(oz - 32)}, X2: int32(tx), Y2: int32(ty), Z2: int32(tz - 32)})
		d.AddLine(drawLine("Inaccessible", serverpackets.PrimitiveRed, loc.X, loc.Y, loc.Z-32, tx, ty, tz-32))
		d.AddLine(drawLine("Last position", serverpackets.PrimitiveRed, loc.X, loc.Y, loc.Z-32, loc.X, loc.Y, loc.Z+oh-32))
	}
	d.AddLine(drawLine("Line-of-Flight MIN", color, ox, oy, oz-32, tx, ty, tz-32))
	d.AddLine(drawLine("Line-of-Fligth MAX", color, ox, oy, oz+oh-32, tx, ty, tz+oh-32))
	sendDrawing(gm, d)
}

// adminPath answers //path find: the route from gm to its selection, told
// point by point and drawn. Without a subcommand it answers the usage; info
// and any unknown subcommand do nothing, as the command has always done,
// and an admin command leaves no client action pending.
func (l *GameClientLink) adminPath(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	if len(args) == 0 {
		sendText(gm, pathUsage)
		return
	}
	if args[0] != "find" {
		return
	}
	target, ok := gm.Target().(standing)
	if !ok {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
		return
	}
	d := gm.drawing("PATH")
	d.Reset()
	_, finder := l.geoQueries()
	ox, oy, oz := gm.Position()
	tx, ty, tz := target.Position()
	tr := &engine.Trace{}
	var path []location.Location
	if finder != nil {
		path, _ = finder.FindTraced(location.Location{X: ox, Y: oy, Z: oz}, location.Location{X: tx, Y: ty, Z: tz}, tr)
	}
	if len(path) == 0 {
		sendText(gm, "No route found or pathfinding is disabled.")
		return
	}
	for _, p := range path {
		sendText(gm, "x:"+strconv.Itoa(p.X)+" y:"+strconv.Itoa(p.Y)+" z:"+strconv.Itoa(p.Z))
	}
	addTrace(d, tr)
	sendDrawing(gm, d)
}

// positionOf is where o stands, the origin for an object that stands
// nowhere.
func positionOf(o world.Tracked) (x, y, z int) {
	if p, ok := o.(standing); ok {
		return p.Position()
	}
	return 0, 0, 0
}

// collisionHeightOf is c's collision height, 0 for an object without a
// body.
func collisionHeightOf(c world.Tracked) float64 {
	if h, ok := c.(collisionHeighted); ok {
		return h.CollisionHeight()
	}
	return 0
}

// drawLine is a labelled line, its label in its color, from x, y, z to x2,
// y2, z2.
func drawLine(label string, color uint32, x, y, z, x2, y2, z2 int) serverpackets.PrimitiveLine {
	return serverpackets.PrimitiveLine{
		PrimitivePoint: serverpackets.PrimitivePoint{Name: label, Color: color, NameColored: true, X: int32(x), Y: int32(y), Z: int32(z)},
		X2:             int32(x2), Y2: int32(y2), Z2: int32(z2),
	}
}

// addTrace draws every stroke tr recorded, in order.
func addTrace(d *serverpackets.Drawing, tr *engine.Trace) {
	for _, s := range tr.Strokes {
		p := serverpackets.PrimitivePoint{Name: s.Label, Color: s.Color, NameColored: s.LabelColored, X: int32(s.X), Y: int32(s.Y), Z: int32(s.Z)}
		if s.Line {
			d.AddLine(serverpackets.PrimitiveLine{PrimitivePoint: p, X2: int32(s.X2), Y2: int32(s.Y2), Z2: int32(s.Z2)})
		} else {
			d.AddPoint(p)
		}
	}
}
