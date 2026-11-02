package block

import (
	"encoding/binary"
	"math"
)

// A Region stores its blocks in four read-only sections, all little-endian:
//
//   - kinds: a 2-bit block kind per block, four blocks per byte.
//   - values: a u16 per block: a flat block's exact height, or a complex or
//     multilayer block's byte offset inside its superblock's data.
//   - bases: a u32 per superblock of 16x16 blocks: the data offset its
//     blocks are stored from, or wideSuperblock|n when its data outgrows u16
//     offsets and row n of wide holds its blocks' absolute data offsets.
//   - data: every complex and multilayer block, superblock by superblock,
//     followed by dataPad bytes so any field is read with one 8-byte load.
//
// Heights are stored in CellHeight units ("q", the 12-bit signed height of
// the on-disk cell code). A packed block starts with a mode byte: bits 0-3
// hold the residual width k (at most maxResidualBits), bits 4-6 the palette
// width p (at most maxPaletteBits), bit 7 a per-kind flag. It is followed by
// an i16 base height and a palette of 2^p NSWE nibbles. Each cell layer is
// then one k+p bit field: its height residual in the low k bits and its
// palette index above them.
//
// A complex block's flag adds two i8 slopes gx and gy after the base; cell
// (x, y) has height base + gx*x + gy*y + residual, and its fields follow the
// palette in cell order.
//
// A multilayer block's flag selects u16 instead of u8 cumulative layer
// starts. After the palette come CellCount+1 starts (cell c owns layers
// start[c] to start[c+1]), then one field per layer, lowest layer of each
// cell first; a layer's height is base + residual.
const (
	superblockShift  = 4
	superblockSide   = 1 << superblockShift
	superblocksY     = RegionBlocksY / superblockSide
	superblockCount  = RegionBlocksX / superblockSide * superblocksY
	superblockBlocks = superblockSide * superblockSide

	regionHeaderSize = 8
	kindsSize        = RegionBlockCount / 4
	valuesSize       = RegionBlockCount * 2
	basesSize        = superblockCount * 4
	wideRowSize      = superblockBlocks * 4
	dataPad          = 8

	wideSuperblock   = 1 << 31
	maxSuperblockOff = math.MaxUint16

	modeResidualMask = 0x0f
	modePaletteShift = 4
	modePaletteMask  = 0x07
	modeFlag         = 0x80
	maxResidualBits  = 12
	maxPaletteBits   = 4

	layerStarts = CellCount + 1
)

// Stored block kinds; each equals the Kind it decodes to.
const (
	regionNull uint8 = iota
	regionFlat
	regionComplex
	regionMultilayer
)

// Region stores one geodata region in the compact read-only layout above.
// Build one with RegionBuilder, or view a stored one with ParseRegion; it is
// never modified afterwards, so its storage may be a read-only mapping.
type Region struct {
	stored []byte // the whole stored form WriteTo emits
	kinds  []byte
	values []byte
	bases  []byte
	wide   []byte
	data   []byte
}

// NewRegion returns an empty region whose blocks answer as null geodata.
func NewRegion() *Region {
	return NewRegionBuilder().Build()
}

// NewRegionFromBlocks packs decoded block objects into a Region.
func NewRegionFromBlocks(blocks []Block) (*Region, error) {
	b, err := newRegionBuilderFromBlocks(blocks)
	if err != nil {
		return nil, err
	}
	return b.Build(), nil
}

// KindAt identifies the block layout at block coordinates.
func (r *Region) KindAt(blockX, blockY int) Kind {
	return Kind(r.kind(blockIndex(blockX, blockY)))
}

// HasGeodata reports whether a block carries real geodata.
func (r *Region) HasGeodata(blockX, blockY int) bool {
	return r.kind(blockIndex(blockX, blockY)) != regionNull
}

// Layers returns the cell's layer count.
func (r *Region) Layers(blockX, blockY, cellX, cellY int) int {
	if r.kind(blockIndex(blockX, blockY)) == regionMultilayer {
		b := r.multilayer(blockX, blockY)
		_, count := b.span(cellIndex(cellX, cellY))
		return count
	}
	return 1
}

// HeightNearest returns the closest layer height at the cell.
func (r *Region) HeightNearest(blockX, blockY, cellX, cellY int, worldZ int32) int16 {
	i := blockIndex(blockX, blockY)
	switch r.kind(i) {
	case regionFlat:
		return r.flatHeight(i)
	case regionComplex:
		return r.complexCell(blockX, blockY, cellIndex(cellX, cellY)).Height
	case regionMultilayer:
		b := r.multilayer(blockX, blockY)
		start, count := b.span(cellIndex(cellX, cellY))
		return b.layer(start + b.nearest(start, count, worldZ)).Height
	default:
		return NullHeight(worldZ)
	}
}

// NSWENearest returns the closest layer passability mask at the cell.
func (r *Region) NSWENearest(blockX, blockY, cellX, cellY int, worldZ int32) NSWE {
	switch r.kind(blockIndex(blockX, blockY)) {
	case regionComplex:
		return r.complexCell(blockX, blockY, cellIndex(cellX, cellY)).NSWE
	case regionMultilayer:
		b := r.multilayer(blockX, blockY)
		start, count := b.span(cellIndex(cellX, cellY))
		return b.layer(start + b.nearest(start, count, worldZ)).NSWE
	default:
		return AllDirections
	}
}

// Nearest returns a handle to the closest layer at the cell.
func (r *Region) Nearest(blockX, blockY, cellX, cellY int, worldZ int32) int {
	switch r.kind(blockIndex(blockX, blockY)) {
	case regionComplex:
		return cellIndex(cellX, cellY)
	case regionMultilayer:
		ci := cellIndex(cellX, cellY)
		b := r.multilayer(blockX, blockY)
		start, count := b.span(ci)
		return ci*layerSlot + b.nearest(start, count, worldZ)
	default:
		return 0
	}
}

// Above returns a handle to the first layer above worldZ, or -1.
func (r *Region) Above(blockX, blockY, cellX, cellY int, worldZ int32) int {
	layer, _ := r.AboveCell(blockX, blockY, cellX, cellY, worldZ)
	return layer
}

// AboveCell is Above that also returns the found layer's cell, so a caller
// resolving the handle at once decodes the cell a single time.
func (r *Region) AboveCell(blockX, blockY, cellX, cellY int, worldZ int32) (int, Cell) {
	i := blockIndex(blockX, blockY)
	switch r.kind(i) {
	case regionFlat:
		if h := r.flatHeight(i); int32(h) > worldZ {
			return 0, Cell{Height: h, NSWE: AllDirections}
		}
	case regionComplex:
		ci := cellIndex(cellX, cellY)
		if c := r.complexCell(blockX, blockY, ci); int32(c.Height) > worldZ {
			return ci, c
		}
	case regionMultilayer:
		ci := cellIndex(cellX, cellY)
		b := r.multilayer(blockX, blockY)
		start, count := b.span(ci)
		for l := 0; l < count; l++ {
			if c := b.layer(start + l); int32(c.Height) > worldZ {
				return ci*layerSlot + l, c
			}
		}
	default:
		return 0, Cell{Height: 0, NSWE: AllDirections}
	}
	return -1, Cell{}
}

// Below returns a handle to the first layer below worldZ, or -1.
func (r *Region) Below(blockX, blockY, cellX, cellY int, worldZ int32) int {
	layer, _ := r.BelowCell(blockX, blockY, cellX, cellY, worldZ)
	return layer
}

// BelowCell is Below that also returns the found layer's cell, so a caller
// resolving the handle at once decodes the cell a single time.
func (r *Region) BelowCell(blockX, blockY, cellX, cellY int, worldZ int32) (int, Cell) {
	i := blockIndex(blockX, blockY)
	switch r.kind(i) {
	case regionFlat:
		if h := r.flatHeight(i); int32(h) < worldZ {
			return 0, Cell{Height: h, NSWE: AllDirections}
		}
	case regionComplex:
		ci := cellIndex(cellX, cellY)
		if c := r.complexCell(blockX, blockY, ci); int32(c.Height) < worldZ {
			return ci, c
		}
	case regionMultilayer:
		ci := cellIndex(cellX, cellY)
		b := r.multilayer(blockX, blockY)
		start, count := b.span(ci)
		for l := count - 1; l >= 0; l-- {
			if c := b.layer(start + l); int32(c.Height) < worldZ {
				return ci*layerSlot + l, c
			}
		}
	default:
		return 0, Cell{Height: 0, NSWE: AllDirections}
	}
	return -1, Cell{}
}

// Height resolves a layer handle to its height.
func (r *Region) Height(blockX, blockY, layer int) int16 {
	return r.layerCell(blockX, blockY, layer).Height
}

// NSWE resolves a layer handle to its passability mask.
func (r *Region) NSWE(blockX, blockY, layer int) NSWE {
	return r.layerCell(blockX, blockY, layer).NSWE
}

// Cells returns a copy of a cell's stored layers.
func (r *Region) Cells(blockX, blockY, cellX, cellY int) []Cell {
	return r.AppendCells(make([]Cell, 0, r.Layers(blockX, blockY, cellX, cellY)), blockX, blockY, cellX, cellY)
}

// AppendCells appends a cell's stored layers, in Cells order, to dst and
// returns the extended slice. With a dst whose capacity covers MaxLayers it
// never allocates, so hot paths can walk a cell's layers through a
// caller-owned buffer instead of Cells' fresh slice.
func (r *Region) AppendCells(dst []Cell, blockX, blockY, cellX, cellY int) []Cell {
	i := blockIndex(blockX, blockY)
	switch r.kind(i) {
	case regionFlat:
		return append(dst, Cell{Height: r.flatHeight(i), NSWE: AllDirections})
	case regionComplex:
		return append(dst, r.complexCell(blockX, blockY, cellIndex(cellX, cellY)))
	case regionMultilayer:
		b := r.multilayer(blockX, blockY)
		start, count := b.span(cellIndex(cellX, cellY))
		for l := 0; l < count; l++ {
			dst = append(dst, b.layer(start+l))
		}
		return dst
	default:
		return append(dst, Cell{Height: 0, NSWE: AllDirections})
	}
}

// layerCell resolves a layer handle to its cell layer. A null block is a
// constant ground height 0, the same contract as Null.Height.
func (r *Region) layerCell(blockX, blockY, layer int) Cell {
	i := blockIndex(blockX, blockY)
	switch r.kind(i) {
	case regionFlat:
		return Cell{Height: r.flatHeight(i), NSWE: AllDirections}
	case regionComplex:
		return r.complexCell(blockX, blockY, layer)
	case regionMultilayer:
		b := r.multilayer(blockX, blockY)
		start, _ := b.span(layer / layerSlot)
		return b.layer(start + layer%layerSlot)
	default:
		return Cell{Height: 0, NSWE: AllDirections}
	}
}

func blockIndex(blockX, blockY int) int {
	return blockX*RegionBlocksY + blockY
}

func (r *Region) kind(i int) uint8 {
	return r.kinds[i>>2] >> (uint(i&3) * 2) & 3
}

func (r *Region) flatHeight(i int) int16 {
	return int16(binary.LittleEndian.Uint16(r.values[i*2:]))
}

// offset returns the data offset of a complex or multilayer block.
func (r *Region) offset(blockX, blockY int) int {
	base := binary.LittleEndian.Uint32(r.bases[(blockX>>superblockShift*superblocksY+blockY>>superblockShift)*4:])
	if base&wideSuperblock != 0 {
		return r.wideOffset(base, blockX, blockY)
	}
	return int(base) + int(binary.LittleEndian.Uint16(r.values[blockIndex(blockX, blockY)*2:]))
}

func (r *Region) wideOffset(base uint32, blockX, blockY int) int {
	local := blockX&(superblockSide-1)*superblockSide + blockY&(superblockSide-1)
	return int(binary.LittleEndian.Uint32(r.wide[(int(base&^wideSuperblock)*superblockBlocks+local)*4:]))
}

// fields reads packed k+p bit fields out of a region's data.
type fields struct {
	data    []byte
	k, w    uint
	palette int // data offset of the NSWE palette
}

// at decodes the field at bit with its height residual added to q.
func (f *fields) at(bit int, q int32) Cell {
	return decodeField(f.data, f.palette, f.k, f.w, bit, q)
}

func decodeField(data []byte, palette int, k, w uint, bit int, q int32) Cell {
	v := uint32(binary.LittleEndian.Uint64(data[bit>>3:])>>(bit&7)) & (1<<w - 1)
	q += int32(v & (1<<k - 1))
	p := uint(v >> k)
	return Cell{
		Height: int16(q * CellHeight),
		NSWE:   NSWE(data[palette+int(p>>1)] >> (p & 1 * 4) & 0xf),
	}
}

// complexCell decodes cell ci of the complex block at blockX, blockY. It is
// the hottest geodata read, so it keeps the load chain short: the header
// (mode, base, slopes and usually the whole palette) is one 8-byte load,
// then the cell field is a second.
func (r *Region) complexCell(blockX, blockY, ci int) Cell {
	base := binary.LittleEndian.Uint32(r.bases[(blockX>>superblockShift*superblocksY+blockY>>superblockShift)*4:])
	off := uint(base) + uint(binary.LittleEndian.Uint16(r.values[blockIndex(blockX, blockY)*2:]))
	if base&wideSuperblock != 0 {
		off = uint(r.wideOffset(base, blockX, blockY))
	}
	head := binary.LittleEndian.Uint64(r.data[off:])
	mode := uint(head)
	k := mode & modeResidualMask
	w := k + mode>>modePaletteShift&modePaletteMask
	q := int32(int16(head >> 8))
	palette := uint(3)
	if mode&modeFlag != 0 {
		c := uint(ci)
		q += int32(int8(head>>24))*int32(c/CellsY) + int32(int8(head>>32))*int32(c%CellsY)
		palette = 5
	}
	size := (uint(1)<<(w-k) + 1) >> 1
	nibbles := head >> (palette * 8)
	if palette+size > 8 {
		nibbles = binary.LittleEndian.Uint64(r.data[off+palette:])
	}
	bit := (off+palette+size)*8 + uint(ci)*w
	v := uint(binary.LittleEndian.Uint64(r.data[bit>>3:])>>(bit&7)) & (1<<w - 1)
	q += int32(v & (1<<k - 1))
	return Cell{Height: int16(q * CellHeight), NSWE: NSWE(nibbles >> (v >> k * 4) & 0xf)}
}

type multilayerBlock struct {
	fields
	base       int32
	starts     int // data offset of the layer starts
	wideStarts bool
	layers     int // bit offset of layer 0
}

func (r *Region) multilayer(blockX, blockY int) multilayerBlock {
	off := r.offset(blockX, blockY)
	mode := r.data[off]
	k := uint(mode & modeResidualMask)
	b := multilayerBlock{
		fields: fields{data: r.data, k: k, w: k + uint(mode>>modePaletteShift&modePaletteMask), palette: off + 3},
		base:   int32(int16(binary.LittleEndian.Uint16(r.data[off+1:]))),
	}
	b.starts = b.palette + paletteBytes(b.w-b.k)
	b.wideStarts = r.data[off]&modeFlag != 0
	b.layers = b.starts + layerStarts
	if b.wideStarts {
		b.layers += layerStarts
	}
	b.layers *= 8
	return b
}

// span returns the index of the cell's lowest layer and its layer count.
func (b *multilayerBlock) span(ci int) (start, count int) {
	if b.wideStarts {
		start = int(binary.LittleEndian.Uint16(b.data[b.starts+ci*2:]))
		return start, int(binary.LittleEndian.Uint16(b.data[b.starts+ci*2+2:])) - start
	}
	start = int(b.data[b.starts+ci])
	return start, int(b.data[b.starts+ci+1]) - start
}

func (b *multilayerBlock) layer(l int) Cell {
	return b.at(b.layers+l*int(b.w), b.base)
}

// nearest returns the cell's layer closest to worldZ; of two equally close
// layers the higher wins.
func (b *multilayerBlock) nearest(start, count int, worldZ int32) int {
	best := 0
	limit := int32(math.MaxInt32)
	for l := 0; l < count; l++ {
		d := abs32(int32(b.layer(start+l).Height) - worldZ)
		if d > limit {
			break
		}
		limit = d
		best = l
	}
	return best
}

// paletteBytes is the size of a palette of 2^p nibbles.
func paletteBytes(p uint) int {
	return int((1<<p + 1) >> 1)
}
