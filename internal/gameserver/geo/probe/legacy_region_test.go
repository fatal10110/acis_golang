package probe

import (
	"encoding/binary"
	"fmt"
	"math"
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

// legacyRegion re-implements the previous packed region decoder with the
// same answers, as the equivalence oracle for the compact layout: a u32 index entry per block
// (kind<<30 | flat height or data offset) over raw 16-bit cell codes, a
// multilayer block storing 64 layer counts and 64 u16 cell offsets.
type legacyRegion struct {
	index [block.RegionBlockCount]uint32
	data  []byte
}

const (
	legacyKindShift      = 30
	legacyValueMask      = (1 << legacyKindShift) - 1
	legacyMultilayerHead = block.CellCount * 3
	legacyLayerSlot      = block.MaxLayers + 1
)

const (
	legacyNull uint32 = iota
	legacyFlat
	legacyComplex
	legacyMultilayer
)

func legacyCellIndex(x, y int) int { return x*block.CellsY + y }

// decodeLegacyL2OFF is the previous L2OFF reader feeding legacyRegion.
func decodeLegacyL2OFF(data []byte) (*legacyRegion, error) {
	pos := 18
	u16 := func() (uint16, error) {
		if len(data)-pos < 2 {
			return 0, fmt.Errorf("short L2OFF region at offset %d", pos)
		}
		v := binary.LittleEndian.Uint16(data[pos:])
		pos += 2
		return v, nil
	}
	r := &legacyRegion{}
	for i := range block.RegionBlockCount {
		typ, err := u16()
		if err != nil {
			return nil, err
		}
		switch typ {
		case 0x0000:
			h, err := u16()
			if err != nil {
				return nil, err
			}
			if _, err := u16(); err != nil {
				return nil, err
			}
			r.index[i] = legacyFlat<<legacyKindShift | uint32(h)
		case 0x0040:
			offset := len(r.data)
			for range block.CellCount {
				code, err := u16()
				if err != nil {
					return nil, err
				}
				r.data = binary.LittleEndian.AppendUint16(r.data, code)
			}
			r.index[i] = legacyComplex<<legacyKindShift | uint32(offset)
		default:
			var counts [block.CellCount]uint8
			var cells []uint16
			for c := range counts {
				count, err := u16()
				if err != nil {
					return nil, err
				}
				counts[c] = uint8(count)
				start := len(cells)
				for range count {
					code, err := u16()
					if err != nil {
						return nil, err
					}
					cells = append(cells, code)
				}
				slices.Reverse(cells[start:])
			}
			offset := len(r.data)
			r.data = append(r.data, make([]byte, legacyMultilayerHead+len(cells)*2)...)
			cellOffset, next := legacyMultilayerHead, 0
			for c, count := range counts {
				r.data[offset+c] = count
				binary.LittleEndian.PutUint16(r.data[offset+block.CellCount+c*2:], uint16(cellOffset))
				for j := range int(count) {
					binary.LittleEndian.PutUint16(r.data[offset+cellOffset+j*2:], cells[next+j])
				}
				next += int(count)
				cellOffset += int(count) * 2
			}
			r.index[i] = legacyMultilayer<<legacyKindShift | uint32(offset)
		}
	}
	return r, nil
}

func (r *legacyRegion) entry(bx, by int) uint32 { return r.index[bx*block.RegionBlocksY+by] }
func legacyKind(e uint32) uint32                { return e >> legacyKindShift }
func legacyValue(e uint32) int                  { return int(e & legacyValueMask) }

func (r *legacyRegion) complexCodeAt(e uint32, cell int) uint16 {
	return binary.LittleEndian.Uint16(r.data[legacyValue(e)+cell*2:])
}

func (r *legacyRegion) multilayerCodeAt(e uint32, cell, layer int) uint16 {
	off := legacyValue(e)
	cellOff := int(binary.LittleEndian.Uint16(r.data[off+block.CellCount+cell*2:]))
	return binary.LittleEndian.Uint16(r.data[off+cellOff+layer*2:])
}

func (r *legacyRegion) KindAt(bx, by int) block.Kind {
	return block.Kind(legacyKind(r.entry(bx, by)))
}

func (r *legacyRegion) Layers(bx, by, cx, cy int) int {
	e := r.entry(bx, by)
	if legacyKind(e) == legacyMultilayer {
		return int(r.data[legacyValue(e)+legacyCellIndex(cx, cy)])
	}
	return 1
}

func (r *legacyRegion) HeightNearest(bx, by, cx, cy int, z int32) int16 {
	e := r.entry(bx, by)
	switch legacyKind(e) {
	case legacyFlat:
		return int16(uint16(legacyValue(e)))
	case legacyComplex:
		return block.DecodeCell(r.complexCodeAt(e, legacyCellIndex(cx, cy))).Height
	case legacyMultilayer:
		return r.Height(bx, by, r.Nearest(bx, by, cx, cy, z))
	default:
		return block.NullHeight(z)
	}
}

func (r *legacyRegion) NSWENearest(bx, by, cx, cy int, z int32) block.NSWE {
	e := r.entry(bx, by)
	switch legacyKind(e) {
	case legacyComplex:
		return block.DecodeCell(r.complexCodeAt(e, legacyCellIndex(cx, cy))).NSWE
	case legacyMultilayer:
		return r.NSWE(bx, by, r.Nearest(bx, by, cx, cy, z))
	default:
		return block.AllDirections
	}
}

func (r *legacyRegion) Nearest(bx, by, cx, cy int, z int32) int {
	e := r.entry(bx, by)
	if legacyKind(e) != legacyMultilayer {
		if legacyKind(e) == legacyComplex {
			return legacyCellIndex(cx, cy)
		}
		return 0
	}
	count := r.Layers(bx, by, cx, cy)
	best := 0
	limit := int32(math.MaxInt32)
	for i := range count {
		d := int32(block.DecodeCell(r.multilayerCodeAt(e, legacyCellIndex(cx, cy), i)).Height) - z
		if d < 0 {
			d = -d
		}
		if d > limit {
			break
		}
		limit = d
		best = i
	}
	return legacyCellIndex(cx, cy)*legacyLayerSlot + best
}

func (r *legacyRegion) Above(bx, by, cx, cy int, z int32) int {
	e := r.entry(bx, by)
	switch legacyKind(e) {
	case legacyFlat:
		if int32(int16(uint16(legacyValue(e)))) > z {
			return 0
		}
	case legacyComplex:
		i := legacyCellIndex(cx, cy)
		if int32(block.DecodeCell(r.complexCodeAt(e, i)).Height) > z {
			return i
		}
	case legacyMultilayer:
		for i := range r.Layers(bx, by, cx, cy) {
			if int32(block.DecodeCell(r.multilayerCodeAt(e, legacyCellIndex(cx, cy), i)).Height) > z {
				return legacyCellIndex(cx, cy)*legacyLayerSlot + i
			}
		}
	default:
		return 0
	}
	return -1
}

func (r *legacyRegion) Below(bx, by, cx, cy int, z int32) int {
	e := r.entry(bx, by)
	switch legacyKind(e) {
	case legacyFlat:
		if int32(int16(uint16(legacyValue(e)))) < z {
			return 0
		}
	case legacyComplex:
		i := legacyCellIndex(cx, cy)
		if int32(block.DecodeCell(r.complexCodeAt(e, i)).Height) < z {
			return i
		}
	case legacyMultilayer:
		for i := r.Layers(bx, by, cx, cy) - 1; i >= 0; i-- {
			if int32(block.DecodeCell(r.multilayerCodeAt(e, legacyCellIndex(cx, cy), i)).Height) < z {
				return legacyCellIndex(cx, cy)*legacyLayerSlot + i
			}
		}
	default:
		return 0
	}
	return -1
}

func (r *legacyRegion) cell(bx, by, layer int) block.Cell {
	e := r.entry(bx, by)
	switch legacyKind(e) {
	case legacyFlat:
		return block.Cell{Height: int16(uint16(legacyValue(e))), NSWE: block.AllDirections}
	case legacyComplex:
		return block.DecodeCell(r.complexCodeAt(e, layer))
	case legacyMultilayer:
		return block.DecodeCell(r.multilayerCodeAt(e, layer/legacyLayerSlot, layer%legacyLayerSlot))
	default:
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	}
}

func (r *legacyRegion) Height(bx, by, layer int) int16    { return r.cell(bx, by, layer).Height }
func (r *legacyRegion) NSWE(bx, by, layer int) block.NSWE { return r.cell(bx, by, layer).NSWE }

func (r *legacyRegion) AppendCells(dst []block.Cell, bx, by, cx, cy int) []block.Cell {
	e := r.entry(bx, by)
	switch legacyKind(e) {
	case legacyMultilayer:
		for i := range r.Layers(bx, by, cx, cy) {
			dst = append(dst, block.DecodeCell(r.multilayerCodeAt(e, legacyCellIndex(cx, cy), i)))
		}
		return dst
	case legacyComplex:
		return append(dst, r.cell(bx, by, legacyCellIndex(cx, cy)))
	default:
		return append(dst, r.cell(bx, by, 0))
	}
}
