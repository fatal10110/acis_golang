package block

import "strings"

// NSWE is a passability mask for one geodata cell layer: one bit per
// cardinal direction that can be moved into from the layer.
type NSWE uint8

// Direction bits, one per cardinal direction a cell layer may allow
// movement into. Values match the on-disk nibble layout exactly.
const (
	East NSWE = 1 << iota
	West
	South
	North
)

// NoDirections is the mask with no direction passable (walled on every side).
const NoDirections NSWE = 0

// AllDirections is the mask with every direction passable (open ground).
const AllDirections NSWE = East | West | South | North

// Allows reports whether every direction in dirs is passable in n.
func (n NSWE) Allows(dirs NSWE) bool { return n&dirs == dirs }

// String renders the set directions as a compact letter combination, e.g.
// "NW", or "none"/"all" for the two extremes.
func (n NSWE) String() string {
	switch n {
	case NoDirections:
		return "none"
	case AllDirections:
		return "all"
	}
	var b strings.Builder
	if n&North != 0 {
		b.WriteByte('N')
	}
	if n&South != 0 {
		b.WriteByte('S')
	}
	if n&West != 0 {
		b.WriteByte('W')
	}
	if n&East != 0 {
		b.WriteByte('E')
	}
	return b.String()
}

// Cell is one geodata layer's decoded height and passability.
type Cell struct {
	Height int16
	NSWE   NSWE
}

// DecodeCell decodes one on-disk geodata cell code, as read from a
// little-endian 16-bit value: the low nibble holds the NSWE mask, and the
// upper 12 bits, arithmetic-shifted right by one bit to preserve sign,
// hold the height (quantized to CellHeight units).
func DecodeCell(code uint16) Cell {
	return Cell{
		Height: int16(code&0xFFF0) >> 1,
		NSWE:   NSWE(code & 0x000F),
	}
}
