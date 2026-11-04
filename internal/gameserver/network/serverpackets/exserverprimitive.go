package serverpackets

import (
	"strconv"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

// OpcodeExServerPrimitive is the extended sub-opcode of ExServerPrimitive.
const OpcodeExServerPrimitive uint16 = 0x0024

// Primitive colors, 0xRRGGBB.
const (
	PrimitiveWhite   uint32 = 0xffffff
	PrimitiveRed     uint32 = 0xff0000
	PrimitiveGreen   uint32 = 0x00ff00
	PrimitiveYellow  uint32 = 0xffff00
	PrimitiveBlue    uint32 = 0x0000ff
	PrimitiveCyan    uint32 = 0x00ffff
	PrimitiveMagenta uint32 = 0xff00ff
)

// PrimitivePoint is one labelled point of a debug drawing.
type PrimitivePoint struct {
	Name string
	// Color is 0xRRGGBB.
	Color uint32
	// NameColored draws the label in Color too.
	NameColored bool
	X, Y, Z     int32
}

// PrimitiveLine is one labelled segment of a debug drawing, from the point's
// coordinates to X2, Y2, Z2.
type PrimitiveLine struct {
	PrimitivePoint
	X2, Y2, Z2 int32
}

// FrameExServerPrimitive builds one debug drawing: the client replaces
// whatever it drew under name with points and lines, written points first.
// x, y and z anchor the drawing.
func FrameExServerPrimitive(name string, x, y, z int32, points []PrimitivePoint, lines []PrimitiveLine) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExServerPrimitive)
	w.WriteString(name)
	w.WriteInt32(x)
	w.WriteInt32(y)
	w.WriteInt32(z)
	// Display range and angle: unbounded.
	w.WriteInt32(0x7fffffff)
	w.WriteInt32(0x7fffffff)
	w.WriteInt32(int32(len(points) + len(lines)))
	for _, p := range points {
		w.WriteUint8(1)
		writePrimitive(w, p)
	}
	for _, l := range lines {
		w.WriteUint8(2)
		writePrimitive(w, l.PrimitivePoint)
		w.WriteInt32(l.X2)
		w.WriteInt32(l.Y2)
		w.WriteInt32(l.Z2)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func writePrimitive(w *wire.Writer, p PrimitivePoint) {
	w.WriteString(p.Name)
	w.WriteInt32(int32(p.Color >> 16 & 0xff))
	w.WriteInt32(int32(p.Color >> 8 & 0xff))
	w.WriteInt32(int32(p.Color & 0xff))
	colored := int32(0)
	if p.NameColored {
		colored = 1
	}
	w.WriteInt32(colored)
	w.WriteInt32(p.X)
	w.WriteInt32(p.Y)
	w.WriteInt32(p.Z)
}

// drawingPartCapacity is the payload size, in bytes of strokes, past which
// a drawing continues in its next frame. The stroke that crosses it stays.
const drawingPartCapacity = 16000

// dummyPointZ is the height of the white point an empty frame carries.
const dummyPointZ = 16384

// Drawing is a named debug drawing a client shows until it is replaced. Its
// strokes are split over as many frames as their size needs, the n-th
// frame after the first being named name+n. A Drawing is not safe for
// concurrent use: its owner holds it.
type Drawing struct {
	name    string
	x, y, z int32
	parts   []drawingPart
}

type drawingPart struct {
	size   int
	points []PrimitivePoint
	lines  []PrimitiveLine
}

// NewDrawing returns an empty drawing anchored at x, y, z.
func NewDrawing(name string, x, y, z int32) *Drawing {
	return &Drawing{name: name, x: x, y: y, z: z, parts: make([]drawingPart, 1)}
}

// open returns the first frame with room left, adding one when every frame
// is full.
func (d *Drawing) open() *drawingPart {
	for i := range d.parts {
		if d.parts[i].size < drawingPartCapacity {
			return &d.parts[i]
		}
	}
	d.parts = append(d.parts, drawingPart{})
	return &d.parts[len(d.parts)-1]
}

// AddPoint draws p.
func (d *Drawing) AddPoint(p PrimitivePoint) { d.open().addPoint(p) }

// AddLine draws l.
func (d *Drawing) AddLine(l PrimitiveLine) {
	part := d.open()
	part.size += 43 + 2*utf16Len(l.Name)
	part.lines = append(part.lines, l)
}

func (p *drawingPart) addPoint(pt PrimitivePoint) {
	p.size += 31 + 2*utf16Len(pt.Name)
	p.points = append(p.points, pt)
}

// AddSquare outlines the square of side size whose corner is x, y, at
// height z, with four unlabelled lines.
func (d *Drawing) AddSquare(color uint32, x, y, z, size int32) {
	x2, y2 := x+size, y+size
	for _, l := range [4][4]int32{{x, y, x, y2}, {x2, y2, x, y2}, {x2, y2, x2, y}, {x, y, x2, y}} {
		d.AddLine(PrimitiveLine{PrimitivePoint: PrimitivePoint{Color: color, X: l[0], Y: l[1], Z: z}, X2: l[2], Y2: l[3], Z2: z})
	}
}

// Reset erases every stroke. The frames the drawing spread over stay, so
// its next Frames still blanks them on the client.
func (d *Drawing) Reset() {
	for i := range d.parts {
		d.parts[i] = drawingPart{points: d.parts[i].points[:0], lines: d.parts[i].lines[:0]}
	}
}

// Frames returns the drawing's frames, in order. A frame without strokes
// gets a white point at the anchor, high above the world, so the client
// blanks it; every frame after the first that was empty is then forgotten.
func (d *Drawing) Frames() []wire.Frame {
	frames := make([]wire.Frame, 0, len(d.parts))
	keep := len(d.parts)
	for i := range d.parts {
		part := &d.parts[i]
		if part.size == 0 {
			part.addPoint(PrimitivePoint{Color: PrimitiveWhite, X: d.x, Y: d.y, Z: dummyPointZ})
			if i > 0 && keep == len(d.parts) {
				keep = i
			}
		}
		name := d.name
		if i > 0 {
			name += strconv.Itoa(i)
		}
		frames = append(frames, FrameExServerPrimitive(name, d.x, d.y, d.z, part.points, part.lines))
	}
	d.parts = d.parts[:keep]
	return frames
}

// AddZoneOutline draws a zone's outline marks: the floor green, the ceiling
// red, both labelled info, and the vertical edges yellow without a label.
func (d *Drawing) AddZoneOutline(info string, marks []zone.Mark) {
	for _, m := range marks {
		p := PrimitivePoint{Name: info, Color: PrimitiveGreen, NameColored: true, X: int32(m.X), Y: int32(m.Y), Z: int32(m.Z)}
		switch m.Part {
		case zone.MarkCeiling:
			p.Color = PrimitiveRed
		case zone.MarkEdge:
			p.Name, p.Color = "", PrimitiveYellow
		}
		if m.Line {
			d.AddLine(PrimitiveLine{PrimitivePoint: p, X2: int32(m.X2), Y2: int32(m.Y2), Z2: int32(m.Z2)})
		} else {
			d.AddPoint(p)
		}
	}
}

// utf16Len is the length of s in UTF-16 code units.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}
