package event

import "github.com/fatal10110/acis_golang/internal/gameserver/model/location"

// MoveDebugStart reports a move request accepted by a mover whose moves
// are debugged. Destination is where the first leg ends and Route holds
// the routed legs after it, none for an unrouted walk. Pawn marks a walk
// that tracks a pawn standing at Destination, stopping Offset short of it;
// Followed one that heads for the position of a followed pawn facing
// DestinationHeading. OriginHeading is the mover's heading before the
// request turned it.
type MoveDebugStart struct {
	Origin, Destination location.Location
	Route               []location.Location
	Pawn                bool
	Offset              int
	OriginHeading       int
	Followed            bool
	DestinationHeading  int
}

// MoveDebugStep reports one position step of a mover whose moves are
// debugged, from From to To, after which it faces Heading; Ground marks a
// step on the ground.
type MoveDebugStep struct {
	From, To location.Location
	Ground   bool
	Heading  int
}

// PathDebug reports the routed search of a mover whose paths are
// debugged: the shapes the search drew. Only a search that found a route
// reports one.
type PathDebug struct {
	Strokes []DebugStroke
}

// DebugStroke is one shape of a debug drawing: a labelled point at X, Y, Z
// or, with Line set, a labelled segment from there to X2, Y2, Z2. Color is
// 0xRRGGBB.
type DebugStroke struct {
	Label        string
	Color        uint32
	LabelColored bool
	Line         bool
	X, Y, Z      int
	X2, Y2, Z2   int
}

func (MoveDebugStart) event() {}
func (MoveDebugStep) event()  {}
func (PathDebug) event()      {}
