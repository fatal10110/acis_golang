package block

import (
	"encoding/binary"
	"math"
)

// A Region stores its blocks in four read-only sections, all little-endian:
//
//   - bases: a u32 per superblock of 16x16 blocks: the data offset its
//     blocks are stored from, or wideSuperblock|n when its data outgrows u16
//     offsets and row n of wide holds its blocks' absolute data offsets.
//   - index: a group per 16 blocks in block index order, so each group
//     covers one block X and 16 consecutive block Ys. A group is a u32 of
//     2-bit block kinds, then a u16 value per block: a flat block's exact
//     height, or a complex or multilayer block's byte offset inside its
//     superblock's data. A block's kind and value thus share one small
//     record.
//   - wide: per wide superblock, a row of u32 absolute data offsets, one
//     per block in superblock storage order.
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
	basesSize        = superblockCount * 4
	groupBlocks      = 16
	groupValues      = 4
	groupSize        = groupValues + groupBlocks*2
	indexSize        = RegionBlockCount / groupBlocks * groupSize
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
	bases  *[basesSize]byte
	index  *[indexSize]byte
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
	kind, _ := r.entry(blockX, blockY)
	return Kind(kind)
}

// HasGeodata reports whether a block carries real geodata.
func (r *Region) HasGeodata(blockX, blockY int) bool {
	kind, _ := r.entry(blockX, blockY)
	return kind != regionNull
}

// Layers returns the cell's layer count.
func (r *Region) Layers(blockX, blockY, cellX, cellY int) int {
	if kind, v := r.entry(blockX, blockY); kind == regionMultilayer {
		b := r.multilayer(blockX, blockY, v)
		_, count := b.span(r.data, cellIndex(cellX, cellY))
		return count
	}
	return 1
}

// HeightNearest returns the closest layer height at the cell.
func (r *Region) HeightNearest(blockX, blockY, cellX, cellY int, worldZ int32) int16 {
	kind, v := r.entry(blockX, blockY)
	switch kind {
	case regionFlat:
		return int16(v)
	case regionComplex:
		return r.complexHeight(blockX, blockY, v, cellIndex(cellX, cellY))
	case regionMultilayer:
		b := r.multilayer(blockX, blockY, v)
		start, count := b.span(r.data, cellIndex(cellX, cellY))
		_, c := b.nearest(r.data, start, count, worldZ)
		return c.Height
	default:
		return NullHeight(worldZ)
	}
}

// NSWENearest returns the closest layer passability mask at the cell.
func (r *Region) NSWENearest(blockX, blockY, cellX, cellY int, worldZ int32) NSWE {
	kind, v := r.entry(blockX, blockY)
	switch kind {
	case regionComplex:
		return r.complexCell(blockX, blockY, v, cellIndex(cellX, cellY)).NSWE
	case regionMultilayer:
		b := r.multilayer(blockX, blockY, v)
		start, count := b.span(r.data, cellIndex(cellX, cellY))
		_, c := b.nearest(r.data, start, count, worldZ)
		return c.NSWE
	default:
		return AllDirections
	}
}

// Nearest returns a handle to the closest layer at the cell.
func (r *Region) Nearest(blockX, blockY, cellX, cellY int, worldZ int32) int {
	kind, v := r.entry(blockX, blockY)
	switch kind {
	case regionComplex:
		return cellIndex(cellX, cellY)
	case regionMultilayer:
		ci := cellIndex(cellX, cellY)
		b := r.multilayer(blockX, blockY, v)
		start, count := b.span(r.data, ci)
		l, _ := b.nearest(r.data, start, count, worldZ)
		return ci*layerSlot + l
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
	kind, v := r.entry(blockX, blockY)
	switch kind {
	case regionFlat:
		if h := int16(v); int32(h) > worldZ {
			return 0, Cell{Height: h, NSWE: AllDirections}
		}
	case regionComplex:
		ci := cellIndex(cellX, cellY)
		if c := r.complexCell(blockX, blockY, v, ci); int32(c.Height) > worldZ {
			return ci, c
		}
	case regionMultilayer:
		ci := cellIndex(cellX, cellY)
		b := r.multilayer(blockX, blockY, v)
		start, count := b.span(r.data, ci)
		for l := 0; l < count; l++ {
			if c := b.layer(r.data, start+l); int32(c.Height) > worldZ {
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
	kind, v := r.entry(blockX, blockY)
	switch kind {
	case regionFlat:
		if h := int16(v); int32(h) < worldZ {
			return 0, Cell{Height: h, NSWE: AllDirections}
		}
	case regionComplex:
		ci := cellIndex(cellX, cellY)
		if c := r.complexCell(blockX, blockY, v, ci); int32(c.Height) < worldZ {
			return ci, c
		}
	case regionMultilayer:
		ci := cellIndex(cellX, cellY)
		b := r.multilayer(blockX, blockY, v)
		start, count := b.span(r.data, ci)
		for l := count - 1; l >= 0; l-- {
			if c := b.layer(r.data, start+l); int32(c.Height) < worldZ {
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
	kind, v := r.entry(blockX, blockY)
	switch kind {
	case regionFlat:
		return append(dst, Cell{Height: int16(v), NSWE: AllDirections})
	case regionComplex:
		return append(dst, r.complexCell(blockX, blockY, v, cellIndex(cellX, cellY)))
	case regionMultilayer:
		b := r.multilayer(blockX, blockY, v)
		start, count := b.span(r.data, cellIndex(cellX, cellY))
		for l := 0; l < count; l++ {
			dst = append(dst, b.layer(r.data, start+l))
		}
		return dst
	default:
		return append(dst, Cell{Height: 0, NSWE: AllDirections})
	}
}

// layerCell resolves a layer handle to its cell layer. A null block is a
// constant ground height 0, the same contract as Null.Height.
func (r *Region) layerCell(blockX, blockY, layer int) Cell {
	kind, v := r.entry(blockX, blockY)
	switch kind {
	case regionFlat:
		return Cell{Height: int16(v), NSWE: AllDirections}
	case regionComplex:
		return r.complexCell(blockX, blockY, v, layer)
	case regionMultilayer:
		b := r.multilayer(blockX, blockY, v)
		start, _ := b.span(r.data, layer/layerSlot)
		return b.layer(r.data, start+layer%layerSlot)
	default:
		return Cell{Height: 0, NSWE: AllDirections}
	}
}

func blockIndex(blockX, blockY int) int {
	return blockX*RegionBlocksY + blockY
}

// entry returns the kind and value of the block at blockX, blockY. Block
// coordinates are always in range; reducing the index modulo the block count
// only lets the compiler drop the bounds checks.
func (r *Region) entry(blockX, blockY int) (uint8, uint16) {
	i := uint(blockIndex(blockX, blockY)) % RegionBlockCount
	g, j := i/groupBlocks*groupSize, i%groupBlocks
	return uint8(binary.LittleEndian.Uint32(r.index[g:]) >> (j * 2) & 3), binary.LittleEndian.Uint16(r.index[g+groupValues+j*2:])
}

// offset returns the data offset of the complex or multilayer block at
// blockX, blockY whose index value is value.
func (r *Region) offset(blockX, blockY int, value uint16) uint {
	sb := uint(blockX>>superblockShift*superblocksY+blockY>>superblockShift) % superblockCount
	base := binary.LittleEndian.Uint32(r.bases[sb*4:])
	if base&wideSuperblock != 0 {
		local := blockX&(superblockSide-1)*superblockSide + blockY&(superblockSide-1)
		return uint(binary.LittleEndian.Uint32(r.wide[(int(base&^wideSuperblock)*superblockBlocks+local)*4:]))
	}
	return uint(base) + uint(value)
}

// complexCell decodes cell ci of the complex block at blockX, blockY whose
// index value is value. It is the hottest geodata read, so it keeps the
// load chain short: the header (mode, base, slopes and usually the whole
// palette) is one 8-byte load, then the cell field is a second.
func (r *Region) complexCell(blockX, blockY int, value uint16, ci int) Cell {
	off := r.offset(blockX, blockY, value)
	head := load64(r.data, off)
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
	nibbles := head >> (palette * 8)
	if w == 0 {
		// One height plane and one NSWE value: the block stores no fields.
		return Cell{Height: int16(q * CellHeight), NSWE: NSWE(nibbles & 0xf)}
	}
	size := (uint(1)<<(w-k) + 1) >> 1
	if palette+size > 8 {
		nibbles = load64(r.data, off+palette)
	}
	bit := (off+palette+size)*8 + uint(ci)*w
	v := uint(load64(r.data, bit>>3)>>(bit&7)) & (1<<w - 1)
	q += int32(v & (1<<k - 1))
	return Cell{Height: int16(q * CellHeight), NSWE: NSWE(nibbles >> (v >> k * 4) & 0xf)}
}

// complexHeight is complexCell's height alone. It skips the palette, and a
// block whose heights lie exactly on its plane (k == 0) needs no field read.
func (r *Region) complexHeight(blockX, blockY int, value uint16, ci int) int16 {
	off := r.offset(blockX, blockY, value)
	head := load64(r.data, off)
	mode := uint(head)
	k := mode & modeResidualMask
	q := int32(int16(head >> 8))
	fields := off + 3
	if mode&modeFlag != 0 {
		c := uint(ci)
		q += int32(int8(head>>24))*int32(c/CellsY) + int32(int8(head>>32))*int32(c%CellsY)
		fields += 2
	}
	if k != 0 {
		p := mode >> modePaletteShift & modePaletteMask
		bit := (fields+(uint(1)<<p+1)>>1)*8 + uint(ci)*(k+p)
		q += int32(uint(load64(r.data, bit>>3)>>(bit&7)) & (1<<k - 1))
	}
	return int16(q * CellHeight)
}

// multilayerBlock is a multilayer block's decoded header. It holds no data
// slice and at most four fields, so it stays in registers.
type multilayerBlock struct {
	head    uint64 // the mode byte, then the base height
	nibbles uint64 // the NSWE palette, at most 16 nibbles
	starts  int    // data offset of the layer starts
	layers  uint   // bit offset of layer 0
}

func (r *Region) multilayer(blockX, blockY int, value uint16) multilayerBlock {
	off := r.offset(blockX, blockY, value)
	head := load64(r.data, off)
	b := multilayerBlock{head: head, nibbles: head >> 24}
	mode := uint(head)
	size := uint(paletteBytes(mode >> modePaletteShift & modePaletteMask))
	if 3+size > 8 {
		b.nibbles = load64(r.data, off+3)
	}
	b.starts = int(off + 3 + size)
	b.layers = off + 3 + size + layerStarts
	if mode&modeFlag != 0 {
		b.layers += layerStarts
	}
	b.layers *= 8
	return b
}

// span returns the index of the cell's lowest layer and its layer count.
func (b multilayerBlock) span(data []byte, ci int) (start, count int) {
	if b.head&modeFlag != 0 {
		start = int(binary.LittleEndian.Uint16(data[b.starts+ci*2:]))
		return start, int(binary.LittleEndian.Uint16(data[b.starts+ci*2+2:])) - start
	}
	start = int(data[b.starts+ci])
	return start, int(data[b.starts+ci+1]) - start
}

func (b multilayerBlock) layer(data []byte, l int) Cell {
	mode := uint(b.head)
	k := mode & modeResidualMask
	w := k + mode>>modePaletteShift&modePaletteMask
	bit := b.layers + uint(l)*w
	v := uint(load64(data, bit>>3)>>(bit&7)) & (1<<w - 1)
	q := int32(int16(b.head>>8)) + int32(v&(1<<k-1))
	return Cell{Height: int16(q * CellHeight), NSWE: NSWE(b.nibbles >> (v >> k * 4) & 0xf)}
}

// nearest returns the cell's layer closest to worldZ and that layer; of two
// equally close layers the higher wins.
func (b multilayerBlock) nearest(data []byte, start, count int, worldZ int32) (int, Cell) {
	best, cell := 0, Cell{}
	limit := int32(math.MaxInt32)
	for l := 0; l < count; l++ {
		c := b.layer(data, start+l)
		d := abs32(int32(c.Height) - worldZ)
		if d > limit {
			break
		}
		limit = d
		best, cell = l, c
	}
	return best, cell
}

// load64 reads the 8 bytes at data[off:]. Its three-index slice keeps the
// read to one bounds check.
func load64(data []byte, off uint) uint64 {
	return binary.LittleEndian.Uint64(data[off : off+8 : off+8])
}

// paletteBytes is the size of a palette of 2^p nibbles.
func paletteBytes(p uint) int {
	return int((1<<p + 1) >> 1)
}
