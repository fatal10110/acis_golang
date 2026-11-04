package network

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/boat"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// boatDockDepth is the height the dock lines of a debugged player's move
// are drawn at.
const boatDockDepth = -3624

// debugMover is a creature whose moves can be debugged.
type debugMover interface {
	Move() *move.CreatureMove
}

// moveDebugViewers runs fn on the queue of every game master that knows
// mover and, for a player mover, of the player itself.
func moveDebugViewers(w *world.State, mover world.Tracked, fn func(gm *livePlayer)) {
	if w != nil {
		w.ForEachKnown(mover, func(o world.Tracked) {
			if g, ok := o.(*livePlayer); ok && g != mover && g.accessLevel().IsGM {
				postLive(g, func() { fn(g) })
			}
		})
	}
	if p, ok := mover.(*livePlayer); ok {
		postLive(p, func() { fn(p) })
	}
}

// drawMoveDebug draws ev, a debug event of mover, on the MOVE<id> or
// PATH<id> drawing of every game master that knows mover, and reports
// whether ev was one.
func drawMoveDebug(w *world.State, mover world.Tracked, ev event.Event) bool {
	id := strconv.Itoa(int(mover.ObjectID()))
	switch e := ev.(type) {
	case event.MoveDebugStart:
		var entrance, exit [2]boat.Point
		docked := false
		if _, player := mover.(*livePlayer); player && !e.Pawn {
			if b := dockedBoat(w, mover); b != nil {
				entrance[0], entrance[1] = b.Dock().Entrance()
				exit[0], exit[1] = b.Dock().Exit()
				docked = true
			}
		}
		moveDebugViewers(w, mover, func(gm *livePlayer) {
			d := gm.drawing("MOVE" + id)
			d.Reset()
			if e.Pawn {
				drawMoveToPawn(gm, d, e)
				return
			}
			d.AddLine(debugLine("MoveToLocation: "+locationText(e.Destination), serverpackets.PrimitiveWhite, e.Origin, e.Destination))
			if docked {
				d.AddLine(dockLine("Boat Entrance", entrance))
				d.AddLine(dockLine("Boat Exit", exit))
			}
			d.AddPoint(serverpackets.PrimitivePoint{Color: serverpackets.PrimitiveRed, X: int32(e.Origin.X), Y: int32(e.Origin.Y), Z: int32(e.Origin.Z)})
			if len(e.Route) == 0 {
				d.AddLine(debugLine("No geopath", serverpackets.PrimitiveYellow, e.Origin, e.Destination))
			} else {
				d.AddLine(debugLine("Segment #1", serverpackets.PrimitiveYellow, e.Origin, e.Destination))
				from := e.Destination
				for i, to := range e.Route {
					d.AddLine(debugLine("Segment #"+strconv.Itoa(i+2), serverpackets.PrimitiveYellow, from, to))
					from = to
				}
			}
			sendText(gm, "Moving from "+locationText(e.Origin)+" to "+locationText(e.Destination))
		})
	case event.MoveDebugStep:
		heading := strconv.Itoa(moverHeading(mover))
		moveDebugViewers(w, mover, func(gm *livePlayer) {
			d := gm.drawing("MOVE" + id)
			d.AddPoint(serverpackets.PrimitivePoint{Name: heading, Color: serverpackets.PrimitiveRed, NameColored: true, X: int32(e.To.X), Y: int32(e.To.Y), Z: int32(e.To.Z)})
			sendDrawing(gm, d)
			if e.Ground && max(e.From.Z-e.To.Z, e.To.Z-e.From.Z) > 100 {
				sendText(gm, "Falling/Climb bug found when moving from "+locationText(e.From)+" to "+locationText(e.To))
			}
		})
	case event.PathDebug:
		moveDebugViewers(w, mover, func(gm *livePlayer) {
			d := gm.drawing("PATH" + id)
			d.Reset()
			addDebugStrokes(d, e.Strokes)
			sendDrawing(gm, d)
		})
	default:
		return false
	}
	return true
}

// drawMoveToPawn adds to d a debugged player's walk toward a pawn: its
// start and the line to the pawn, then tells gm.
func drawMoveToPawn(gm *livePlayer, d *serverpackets.Drawing, e event.MoveDebugStart) {
	o, t := e.Origin, e.Destination
	d.AddPoint(serverpackets.PrimitivePoint{Color: serverpackets.PrimitiveRed, X: int32(o.X), Y: int32(o.Y), Z: int32(o.Z)})
	d.AddLine(debugLine("MoveToPawn ("+strconv.Itoa(e.Offset)+"): "+spacedText(t), serverpackets.PrimitiveWhite, o, t))
	sendText(gm, "Moving from "+spacedText(o)+" to "+spacedText(t))
}

// drawValidatedPosition adds the position live reported, labelled with its
// speed and desync, to the MOVE<id> drawing of every game master that
// knows live, when live's moves are debugged.
func (l *GameClientLink) drawValidatedPosition(live *livePlayer, at location.Location, speed float32, desync float64) {
	if !live.Move().DebugMove() {
		return
	}
	name := "MOVE" + strconv.Itoa(int(live.ObjectID()))
	desc := "speed=" + commons.FormatFloat(speed) + " desync=" + commons.FormatDouble(desync)
	moveDebugViewers(l.world, live, func(gm *livePlayer) {
		d := gm.drawing(name)
		d.AddPoint(serverpackets.PrimitivePoint{Name: desc, Color: serverpackets.PrimitiveGreen, NameColored: true, X: int32(at.X), Y: int32(at.Y), Z: int32(at.Z)})
		sendDrawing(gm, d)
	})
}

// dockedBoat returns a boat docked where mover knows it, nil without one.
func dockedBoat(w *world.State, mover world.Tracked) *boat.Boat {
	if w == nil {
		return nil
	}
	var found *boat.Boat
	w.ForEachKnown(mover, func(o world.Tracked) {
		if b, ok := o.(*boat.Boat); ok && found == nil && b.Dock() != nil {
			found = b
		}
	})
	return found
}

// moverHeading is mover's heading, 0 for a mover with none.
func moverHeading(mover world.Tracked) int {
	switch m := mover.(type) {
	case interface{ Heading() int }:
		return m.Heading()
	case interface{ CurrentHeading() int }:
		return m.CurrentHeading()
	}
	return 0
}

// debugLine is a labelled line of color from a to b.
func debugLine(label string, color uint32, a, b location.Location) serverpackets.PrimitiveLine {
	l := segment(label, a, b)
	l.Color = color
	return l
}

// dockLine is a labelled white dock line at boatDockDepth.
func dockLine(label string, ends [2]boat.Point) serverpackets.PrimitiveLine {
	a := location.Location{X: ends[0].X, Y: ends[0].Y, Z: boatDockDepth}
	b := location.Location{X: ends[1].X, Y: ends[1].Y, Z: boatDockDepth}
	return debugLine(label, serverpackets.PrimitiveWhite, a, b)
}

// addDebugStrokes adds every stroke to d, in order.
func addDebugStrokes(d *serverpackets.Drawing, strokes []event.DebugStroke) {
	for _, s := range strokes {
		p := serverpackets.PrimitivePoint{Name: s.Label, Color: s.Color, NameColored: s.LabelColored, X: int32(s.X), Y: int32(s.Y), Z: int32(s.Z)}
		if s.Line {
			d.AddLine(serverpackets.PrimitiveLine{PrimitivePoint: p, X2: int32(s.X2), Y2: int32(s.Y2), Z2: int32(s.Z2)})
			continue
		}
		d.AddPoint(p)
	}
}

// locationText writes at as "x, y, z".
func locationText(at location.Location) string {
	return strconv.Itoa(at.X) + ", " + strconv.Itoa(at.Y) + ", " + strconv.Itoa(at.Z)
}

// spacedText writes at as "x y z".
func spacedText(at location.Location) string {
	return strconv.Itoa(at.X) + " " + strconv.Itoa(at.Y) + " " + strconv.Itoa(at.Z)
}
