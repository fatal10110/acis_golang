package move

import (
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// tracedGeo is a recordingGeo whose routed search draws one red point.
type tracedGeo struct{ *recordingGeo }

func (g tracedGeo) FindPathTraced(origin, target location.Location, tr *engine.Trace) ([]location.Location, bool) {
	tr.Point("searched", engine.TraceRed, true, origin.X, origin.Y, origin.Z)
	return g.FindPath(origin, target)
}

// debugEvents returns the debug events among events, in order.
func debugEvents(events []event.Event) []event.Event {
	var out []event.Event
	for _, e := range events {
		switch e.(type) {
		case event.MoveDebugStart, event.MoveDebugStep, event.PathDebug:
			out = append(out, e)
		}
	}
	return out
}

// A debugged mover reports, to its sink: the routed search's drawing when
// its paths are debugged, then the accepted request (first leg and the
// routed legs after it, or the pawn it tracks) and each step when its moves
// are; toggled off it reports nothing.
func TestControllerReportsDebuggedMoves(t *testing.T) {
	geo := tracedGeo{&recordingGeo{
		canMove:    false,
		findPath:   []location.Location{{X: 100, Y: 100}, {Y: 300}},
		findPathOK: true,
	}}
	controller, mover, log, _ := newChaseController(t, &headingSelf{}, geo)
	if ok, _ := controller.MoveToLocation(location.Location{Y: 300}); !ok {
		t.Fatal("walk not accepted")
	}
	if got := debugEvents(log.events); len(got) != 0 {
		t.Fatalf("undebugged walk reported %+v", got)
	}

	if !mover.ToggleDebug(false) || !mover.ToggleDebug(true) || !mover.DebugMove() {
		t.Fatal("toggles did not turn on")
	}
	log.events = nil
	controller.Stop()
	if ok, _ := controller.MoveToLocation(location.Location{Y: 300}); !ok {
		t.Fatal("walk not accepted")
	}
	geo.canMove = true
	controller.PositionUpdate()
	want := []event.Event{
		event.PathDebug{Strokes: []event.DebugStroke{{Label: "searched", Color: engine.TraceRed, LabelColored: true}}},
		event.MoveDebugStart{Destination: location.Location{X: 100, Y: 100}, Route: []location.Location{{Y: 300}}},
	}
	got := debugEvents(log.events)
	if len(got) != 3 || !reflect.DeepEqual(got[:2], want) {
		t.Fatalf("debugged walk reported %+v, want %+v then a step", got, want)
	}
	step, ok := got[2].(event.MoveDebugStep)
	if !ok || step.From != (location.Location{}) || step.To == step.From || !step.Ground {
		t.Fatalf("step = %+v, want a ground step from the start", got[2])
	}

	log.events = nil
	if !controller.MoveToPawn(&followTarget{x: 300}, 40) {
		t.Fatal("pawn walk not accepted")
	}
	start, ok := debugEvents(log.events)[0].(event.MoveDebugStart)
	if !ok || !start.Pawn || start.Offset != 40 || start.Destination != (location.Location{X: 300}) {
		t.Fatalf("pawn walk reported %+v, want the pawn at 300 tracked 40 short", log.events)
	}

	if mover.ToggleDebug(false) || mover.ToggleDebug(true) || mover.DebugMove() {
		t.Fatal("toggles did not turn off")
	}
	log.events = nil
	controller.Stop()
	if ok, _ := controller.MoveToLocation(location.Location{X: -300}); !ok {
		t.Fatal("walk not accepted")
	}
	controller.PositionUpdate()
	if got := debugEvents(log.events); len(got) != 0 {
		t.Fatalf("walk after the toggles went off reported %+v", got)
	}
}
