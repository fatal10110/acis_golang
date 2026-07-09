package block

import (
	"encoding/binary"
	"fmt"
	"math"
)

const (
	regionKindShift      = 30
	regionValueMask      = (1 << regionKindShift) - 1
	multilayerHeaderSize = CellCount + CellCount*2
)

const (
	regionNull uint32 = iota
	regionFlat
	regionComplex
	regionMultilayer
)

// Region stores one geodata region as a fixed block index plus one packed
// little-endian cell-data buffer. Build it once, then treat it as read-only.
type Region struct {
	index [RegionBlockCount]uint32
	data  []byte
}

// NewRegion returns an empty region whose blocks answer as null geodata.
func NewRegion() *Region {
	return &Region{}
}

// NewRegionFromBlocks packs decoded block objects into a Region.
func NewRegionFromBlocks(blocks []Block) (*Region, error) {
	if len(blocks) > RegionBlockCount {
		return nil, fmt.Errorf("geo/block: region has %d blocks, want at most %d", len(blocks), RegionBlockCount)
	}

	r := NewRegion()
	for i, b := range blocks {
		if b == nil || b.Kind() == KindNull {
			continue
		}
		switch b.Kind() {
		case KindFlat:
			r.SetFlat(i, b.Cells(0, 0)[0].Height)
		case KindComplex:
			var cells [CellCount]Cell
			for x := 0; x < CellsX; x++ {
				for y := 0; y < CellsY; y++ {
					cells[cellIndex(x, y)] = b.Cells(x, y)[0]
				}
			}
			if err := r.SetComplex(i, cells); err != nil {
				return nil, err
			}
		case KindMultilayer:
			var cells [CellCount][]Cell
			for x := 0; x < CellsX; x++ {
				for y := 0; y < CellsY; y++ {
					cells[cellIndex(x, y)] = b.Cells(x, y)
				}
			}
			if err := r.SetMultilayer(i, cells); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("geo/block: block %d has unknown kind %v", i, b.Kind())
		}
	}
	return r, nil
}

// SetFlat stores a flat block at blockIndex.
func (r *Region) SetFlat(blockIndex int, height int16) {
	r.index[blockIndex] = regionEntry(regionFlat, uint32(uint16(height)))
}

// SetComplex stores a complex block at blockIndex. Heights are encoded in
// the on-disk cell format, so non-flat block heights are quantized to
// CellHeight units; SetFlat stores its height exactly.
func (r *Region) SetComplex(blockIndex int, cells [CellCount]Cell) error {
	var encoded [CellCount]uint16
	for i, c := range cells {
		encoded[i] = encodeCell(c)
	}
	return r.SetComplexEncoded(blockIndex, encoded)
}

// SetComplexEncoded stores a complex block from raw little-endian cell codes.
func (r *Region) SetComplexEncoded(blockIndex int, cells [CellCount]uint16) error {
	offset, err := r.appendData(CellCount * 2)
	if err != nil {
		return err
	}
	for i, code := range cells {
		binary.LittleEndian.PutUint16(r.data[offset+i*2:], code)
	}
	r.index[blockIndex] = regionEntry(regionComplex, uint32(offset))
	return nil
}

// SetMultilayer stores a multilayer block at blockIndex. Heights are encoded
// in the on-disk cell format, so non-flat block heights are quantized to
// CellHeight units; SetFlat stores its height exactly.
func (r *Region) SetMultilayer(blockIndex int, cells [CellCount][]Cell) error {
	var counts [CellCount]uint8
	codes := make([]uint16, 0, CellCount)
	for i, layers := range cells {
		if len(layers) == 0 || len(layers) > MaxLayers {
			return fmt.Errorf("geo/block: cell %d: invalid layer count %d", i, len(layers))
		}
		counts[i] = uint8(len(layers))
		for _, layer := range layers {
			codes = append(codes, encodeCell(layer))
		}
	}
	return r.SetMultilayerEncoded(blockIndex, counts, codes)
}

// SetMultilayerEncoded stores a multilayer block from raw little-endian cell codes.
func (r *Region) SetMultilayerEncoded(blockIndex int, counts [CellCount]uint8, cells []uint16) error {
	total := 0
	for i, count := range counts {
		if count == 0 || count > MaxLayers {
			return fmt.Errorf("geo/block: cell %d: invalid layer count %d", i, count)
		}
		total += int(count)
	}
	if total != len(cells) {
		return fmt.Errorf("geo/block: multilayer has %d cells, want %d", len(cells), total)
	}

	offset, err := r.appendData(multilayerHeaderSize + total*2)
	if err != nil {
		return err
	}
	cellOffset := multilayerHeaderSize
	next := 0
	for i, count := range counts {
		r.data[offset+i] = count
		binary.LittleEndian.PutUint16(r.data[offset+CellCount+i*2:], uint16(cellOffset))
		for j := 0; j < int(count); j++ {
			binary.LittleEndian.PutUint16(r.data[offset+cellOffset+j*2:], cells[next+j])
		}
		next += int(count)
		cellOffset += int(count) * 2
	}
	r.index[blockIndex] = regionEntry(regionMultilayer, uint32(offset))
	return nil
}

// KindAt identifies the block layout at block coordinates.
func (r *Region) KindAt(blockX, blockY int) Kind {
	switch regionKind(r.entry(blockX, blockY)) {
	case regionFlat:
		return KindFlat
	case regionComplex:
		return KindComplex
	case regionMultilayer:
		return KindMultilayer
	default:
		return KindNull
	}
}

// HasGeodata reports whether a block carries real geodata.
func (r *Region) HasGeodata(blockX, blockY int) bool {
	return regionKind(r.entry(blockX, blockY)) != regionNull
}

// Layers returns the cell's layer count.
func (r *Region) Layers(blockX, blockY, cellX, cellY int) int {
	entry := r.entry(blockX, blockY)
	if regionKind(entry) == regionMultilayer {
		return int(r.data[regionValue(entry)+cellIndex(cellX, cellY)])
	}
	return 1
}

// HeightNearest returns the closest layer height at the cell.
func (r *Region) HeightNearest(blockX, blockY, cellX, cellY int, worldZ int32) int16 {
	entry := r.entry(blockX, blockY)
	switch regionKind(entry) {
	case regionFlat:
		return int16(uint16(regionValue(entry)))
	case regionComplex:
		return DecodeCell(r.complexCode(entry, cellX, cellY)).Height
	case regionMultilayer:
		return r.Height(blockX, blockY, r.Nearest(blockX, blockY, cellX, cellY, worldZ))
	default:
		return NullHeight(worldZ)
	}
}

// NSWENearest returns the closest layer passability mask at the cell.
func (r *Region) NSWENearest(blockX, blockY, cellX, cellY int, worldZ int32) NSWE {
	entry := r.entry(blockX, blockY)
	switch regionKind(entry) {
	case regionFlat:
		return AllDirections
	case regionComplex:
		return DecodeCell(r.complexCode(entry, cellX, cellY)).NSWE
	case regionMultilayer:
		return r.NSWE(blockX, blockY, r.Nearest(blockX, blockY, cellX, cellY, worldZ))
	default:
		return AllDirections
	}
}

// Nearest returns a handle to the closest layer at the cell.
func (r *Region) Nearest(blockX, blockY, cellX, cellY int, worldZ int32) int {
	entry := r.entry(blockX, blockY)
	if regionKind(entry) != regionMultilayer {
		if regionKind(entry) == regionComplex {
			return cellIndex(cellX, cellY)
		}
		return 0
	}

	count := r.Layers(blockX, blockY, cellX, cellY)
	best := 0
	limit := int32(math.MaxInt32)
	for i := 0; i < count; i++ {
		d := abs32(int32(DecodeCell(r.multilayerCode(entry, cellX, cellY, i)).Height) - worldZ)
		if d > limit {
			break
		}
		limit = d
		best = i
	}
	return cellIndex(cellX, cellY)*layerSlot + best
}

// Above returns a handle to the first layer above worldZ, or -1.
func (r *Region) Above(blockX, blockY, cellX, cellY int, worldZ int32) int {
	entry := r.entry(blockX, blockY)
	switch regionKind(entry) {
	case regionFlat:
		if int32(int16(uint16(regionValue(entry)))) > worldZ {
			return 0
		}
	case regionComplex:
		i := cellIndex(cellX, cellY)
		if int32(DecodeCell(r.complexCode(entry, cellX, cellY)).Height) > worldZ {
			return i
		}
	case regionMultilayer:
		count := r.Layers(blockX, blockY, cellX, cellY)
		for i := count - 1; i >= 0; i-- {
			if int32(DecodeCell(r.multilayerCode(entry, cellX, cellY, i)).Height) > worldZ {
				return cellIndex(cellX, cellY)*layerSlot + i
			}
		}
	default:
		return 0
	}
	return -1
}

// Below returns a handle to the first layer below worldZ, or -1.
func (r *Region) Below(blockX, blockY, cellX, cellY int, worldZ int32) int {
	entry := r.entry(blockX, blockY)
	switch regionKind(entry) {
	case regionFlat:
		if int32(int16(uint16(regionValue(entry)))) < worldZ {
			return 0
		}
	case regionComplex:
		i := cellIndex(cellX, cellY)
		if int32(DecodeCell(r.complexCode(entry, cellX, cellY)).Height) < worldZ {
			return i
		}
	case regionMultilayer:
		count := r.Layers(blockX, blockY, cellX, cellY)
		for i := 0; i < count; i++ {
			if int32(DecodeCell(r.multilayerCode(entry, cellX, cellY, i)).Height) < worldZ {
				return cellIndex(cellX, cellY)*layerSlot + i
			}
		}
	default:
		return 0
	}
	return -1
}

// Height resolves a layer handle to its height.
func (r *Region) Height(blockX, blockY, layer int) int16 {
	entry := r.entry(blockX, blockY)
	switch regionKind(entry) {
	case regionFlat:
		return int16(uint16(regionValue(entry)))
	case regionComplex:
		return DecodeCell(r.complexCodeAt(entry, layer)).Height
	case regionMultilayer:
		ci, li := layer/layerSlot, layer%layerSlot
		return DecodeCell(r.multilayerCodeAt(entry, ci, li)).Height
	default:
		return 0
	}
}

// NSWE resolves a layer handle to its passability mask.
func (r *Region) NSWE(blockX, blockY, layer int) NSWE {
	entry := r.entry(blockX, blockY)
	switch regionKind(entry) {
	case regionFlat:
		return AllDirections
	case regionComplex:
		return DecodeCell(r.complexCodeAt(entry, layer)).NSWE
	case regionMultilayer:
		ci, li := layer/layerSlot, layer%layerSlot
		return DecodeCell(r.multilayerCodeAt(entry, ci, li)).NSWE
	default:
		return AllDirections
	}
}

// Cells returns a copy of a cell's stored layers.
func (r *Region) Cells(blockX, blockY, cellX, cellY int) []Cell {
	entry := r.entry(blockX, blockY)
	switch regionKind(entry) {
	case regionFlat:
		return []Cell{{Height: int16(uint16(regionValue(entry))), NSWE: AllDirections}}
	case regionComplex:
		return []Cell{DecodeCell(r.complexCode(entry, cellX, cellY))}
	case regionMultilayer:
		count := r.Layers(blockX, blockY, cellX, cellY)
		out := make([]Cell, count)
		for i := range out {
			out[i] = DecodeCell(r.multilayerCode(entry, cellX, cellY, i))
		}
		return out
	default:
		return []Cell{{Height: 0, NSWE: AllDirections}}
	}
}

func (r *Region) appendData(n int) (int, error) {
	if n < 0 || len(r.data) > regionValueMask-n {
		return 0, fmt.Errorf("geo/block: packed region data exceeds %d bytes", regionValueMask)
	}
	offset := len(r.data)
	r.data = append(r.data, make([]byte, n)...)
	return offset, nil
}

func (r *Region) entry(blockX, blockY int) uint32 {
	return r.index[blockX*RegionBlocksY+blockY]
}

func regionEntry(kind uint32, value uint32) uint32 {
	return kind<<regionKindShift | value&regionValueMask
}

func regionKind(entry uint32) uint32 {
	return entry >> regionKindShift
}

func regionValue(entry uint32) int {
	return int(entry & regionValueMask)
}

func (r *Region) complexCode(entry uint32, cellX, cellY int) uint16 {
	return r.complexCodeAt(entry, cellIndex(cellX, cellY))
}

func (r *Region) complexCodeAt(entry uint32, cell int) uint16 {
	return binary.LittleEndian.Uint16(r.data[regionValue(entry)+cell*2:])
}

func (r *Region) multilayerCode(entry uint32, cellX, cellY, layer int) uint16 {
	return r.multilayerCodeAt(entry, cellIndex(cellX, cellY), layer)
}

func (r *Region) multilayerCodeAt(entry uint32, cell, layer int) uint16 {
	offset := regionValue(entry)
	cellOffset := int(binary.LittleEndian.Uint16(r.data[offset+CellCount+cell*2:]))
	return binary.LittleEndian.Uint16(r.data[offset+cellOffset+layer*2:])
}

func encodeCell(c Cell) uint16 {
	return uint16(c.Height)<<1&0xfff0 | uint16(c.NSWE)&0x000f
}
