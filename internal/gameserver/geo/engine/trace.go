package engine

// Trace colors, 0xRRGGBB.
const (
	TraceWhite   uint32 = 0xffffff
	TraceRed     uint32 = 0xff0000
	TraceGreen   uint32 = 0x00ff00
	TraceBlue    uint32 = 0x0000ff
	TraceYellow  uint32 = 0xffff00
	TraceCyan    uint32 = 0x00ffff
	TraceMagenta uint32 = 0xff00ff
)

// Stroke is one shape a traced query drew: a labelled point at X, Y, Z or,
// with Line set, a labelled segment from there to X2, Y2, Z2.
type Stroke struct {
	Label        string
	Color        uint32
	LabelColored bool
	Line         bool
	X, Y, Z      int
	X2, Y2, Z2   int
}

// Trace records, in order, the cells and borders a traced query walks. A
// nil *Trace records nothing, which is what every untraced query passes;
// its methods are safe to call on nil.
type Trace struct {
	Strokes []Stroke
}

// Point records a labelled point.
func (t *Trace) Point(label string, color uint32, labelColored bool, x, y, z int) {
	if t == nil {
		return
	}
	t.Strokes = append(t.Strokes, Stroke{Label: label, Color: color, LabelColored: labelColored, X: x, Y: y, Z: z})
}

// Line records a labelled segment.
func (t *Trace) Line(label string, color uint32, labelColored bool, x, y, z, x2, y2, z2 int) {
	if t == nil {
		return
	}
	t.Strokes = append(t.Strokes, Stroke{Label: label, Color: color, LabelColored: labelColored, Line: true, X: x, Y: y, Z: z, X2: x2, Y2: y2, Z2: z2})
}

// cellSquare outlines the geodata cell whose corner is x, y at height z:
// four unlabelled lines.
func (t *Trace) cellSquare(color uint32, x, y, z int) {
	if t == nil {
		return
	}
	const size = 15
	x2, y2 := x+size, y+size
	t.Line("", color, false, x, y, z, x, y2, z)
	t.Line("", color, false, x2, y2, z, x, y2, z)
	t.Line("", color, false, x2, y2, z, x2, y, z)
	t.Line("", color, false, x, y, z, x2, y, z)
}

// border marks the cell border a walk crosses: the direction it steps in,
// cyan crossing in X, yellow in Y.
func (t *Trace) border(dir moveDirection, inX bool, x, y, z int) {
	if t == nil {
		return
	}
	if inX {
		t.Point(symbol(dir.signumX, "W", "E"), TraceCyan, true, x, y, z)
	} else {
		t.Point(symbol(dir.signumY, "N", "S"), TraceYellow, true, x, y, z)
	}
}

func symbol(signum int, negative, positive string) string {
	switch {
	case signum < 0:
		return negative
	case signum > 0:
		return positive
	default:
		return "-"
	}
}
