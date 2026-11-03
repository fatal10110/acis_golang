package block

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"
)

// RegionBuilder collects one region's blocks, encoding each as it is set,
// and lays them out into a Region with Build. Setting a block again replaces
// it. The zero value is an empty region; its blocks answer as null geodata.
type RegionBuilder struct {
	kinds   [RegionBlockCount]uint8
	heights [RegionBlockCount]int16
	spans   [RegionBlockCount]builderSpan
	scratch []byte
}

// builderSpan locates one encoded block in the builder's scratch buffer.
type builderSpan struct {
	off, n uint32
}

// NewRegionBuilder returns an empty region builder.
func NewRegionBuilder() *RegionBuilder {
	return &RegionBuilder{}
}

func newRegionBuilderFromBlocks(blocks []Block) (*RegionBuilder, error) {
	if len(blocks) > RegionBlockCount {
		return nil, fmt.Errorf("geo/block: region has %d blocks, want at most %d", len(blocks), RegionBlockCount)
	}

	rb := NewRegionBuilder()
	for i, b := range blocks {
		if b == nil {
			continue
		}
		switch b.Kind() {
		case KindNull:
		case KindFlat:
			rb.SetFlat(i, b.Cells(0, 0)[0].Height)
		case KindComplex:
			var cells [CellCount]Cell
			for x := 0; x < CellsX; x++ {
				for y := 0; y < CellsY; y++ {
					cells[cellIndex(x, y)] = b.Cells(x, y)[0]
				}
			}
			rb.SetComplex(i, cells)
		case KindMultilayer:
			var cells [CellCount][]Cell
			for x := 0; x < CellsX; x++ {
				for y := 0; y < CellsY; y++ {
					cells[cellIndex(x, y)] = b.Cells(x, y)
				}
			}
			if err := rb.SetMultilayer(i, cells); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("geo/block: block %d has unknown kind %v", i, b.Kind())
		}
	}
	return rb, nil
}

// SetFlat stores a flat block at blockIndex. Its height is stored exactly.
func (b *RegionBuilder) SetFlat(blockIndex int, height int16) {
	b.kinds[blockIndex] = regionFlat
	b.heights[blockIndex] = height
}

// SetComplex stores a complex block at blockIndex. Heights are encoded in
// the on-disk cell format, so non-flat block heights are quantized to
// CellHeight units; SetFlat stores its height exactly.
func (b *RegionBuilder) SetComplex(blockIndex int, cells [CellCount]Cell) {
	var encoded [CellCount]uint16
	for i, c := range cells {
		encoded[i] = encodeCell(c)
	}
	b.SetComplexEncoded(blockIndex, encoded)
}

// SetComplexEncoded stores a complex block from raw little-endian cell codes.
func (b *RegionBuilder) SetComplexEncoded(blockIndex int, cells [CellCount]uint16) {
	off := len(b.scratch)
	b.scratch = appendComplex(b.scratch, &cells)
	b.setPacked(blockIndex, regionComplex, off)
}

// SetMultilayer stores a multilayer block at blockIndex. Heights are encoded
// in the on-disk cell format, so non-flat block heights are quantized to
// CellHeight units; SetFlat stores its height exactly.
func (b *RegionBuilder) SetMultilayer(blockIndex int, cells [CellCount][]Cell) error {
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
	return b.SetMultilayerEncoded(blockIndex, counts, codes)
}

// SetMultilayerEncoded stores a multilayer block from raw little-endian cell
// codes, each cell's layers ordered lowest to highest.
func (b *RegionBuilder) SetMultilayerEncoded(blockIndex int, counts [CellCount]uint8, cells []uint16) error {
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

	off := len(b.scratch)
	b.scratch = appendMultilayer(b.scratch, &counts, cells)
	b.setPacked(blockIndex, regionMultilayer, off)
	return nil
}

func (b *RegionBuilder) setPacked(blockIndex int, kind uint8, off int) {
	b.kinds[blockIndex] = kind
	b.spans[blockIndex] = builderSpan{off: uint32(off), n: uint32(len(b.scratch) - off)}
}

// Build lays the blocks out superblock by superblock into a Region whose
// storage is one exactly sized allocation. The builder stays usable.
func (b *RegionBuilder) Build() *Region {
	var wide [superblockCount]bool
	wideRows, dataLen := 0, 0
	for sb := range superblockCount {
		n := 0
		for _, i := range superblockOrder(sb) {
			if b.packed(i) {
				if n > maxSuperblockOff {
					wide[sb] = true
				}
				n += int(b.spans[i].n)
			}
		}
		if wide[sb] {
			wideRows++
		}
		dataLen += n
	}

	stored := make([]byte, storedSize(wideRows, dataLen+dataPad))
	binary.LittleEndian.PutUint32(stored, uint32(wideRows))
	binary.LittleEndian.PutUint32(stored[4:], uint32(dataLen+dataPad))
	r := viewRegion(stored, wideRows)

	for i, kind := range b.kinds {
		if kind == regionFlat {
			r.setEntry(i, kind, uint16(b.heights[i]))
		}
	}
	off, row := 0, 0
	for sb := range superblockCount {
		base := off
		if wide[sb] {
			binary.LittleEndian.PutUint32(r.bases[sb*4:], wideSuperblock|uint32(row))
		} else {
			binary.LittleEndian.PutUint32(r.bases[sb*4:], uint32(base))
		}
		for local, i := range superblockOrder(sb) {
			if !b.packed(i) {
				continue
			}
			value := 0
			if wide[sb] {
				binary.LittleEndian.PutUint32(r.wide[(row*superblockBlocks+local)*4:], uint32(off))
			} else {
				value = off - base
			}
			r.setEntry(i, b.kinds[i], uint16(value))
			s := b.spans[i]
			off += copy(r.data[off:], b.scratch[s.off:s.off+s.n])
		}
		if wide[sb] {
			row++
		}
	}
	return r
}

// setEntry stores block i's kind and value into a fresh region's index.
func (r *Region) setEntry(i int, kind uint8, value uint16) {
	g, j := i/groupBlocks*groupSize, i%groupBlocks
	binary.LittleEndian.PutUint32(r.index[g:], binary.LittleEndian.Uint32(r.index[g:])|uint32(kind)<<(j*2))
	binary.LittleEndian.PutUint16(r.index[g+groupValues+j*2:], value)
}

func (b *RegionBuilder) packed(i int) bool {
	return b.kinds[i] == regionComplex || b.kinds[i] == regionMultilayer
}

// superblockOrder returns the block indexes of superblock sb in storage
// order, which is also the order of its wide row.
func superblockOrder(sb int) [superblockBlocks]int {
	var order [superblockBlocks]int
	x0, y0 := sb/superblocksY*superblockSide, sb%superblocksY*superblockSide
	for local := range order {
		order[local] = blockIndex(x0+local/superblockSide, y0+local%superblockSide)
	}
	return order
}

// storedSize is the size of a stored region with the given wide rows and
// data length.
func storedSize(wideRows, dataLen int) int {
	return regionHeaderSize + basesSize + indexSize + wideRows*wideRowSize + dataLen
}

// viewRegion slices a stored region's sections out of stored.
func viewRegion(stored []byte, wideRows int) *Region {
	r := &Region{stored: stored}
	s := stored[regionHeaderSize:]
	r.bases, s = (*[basesSize]byte)(s), s[basesSize:]
	r.index, s = (*[indexSize]byte)(s), s[indexSize:]
	n := wideRows * wideRowSize
	r.wide, r.data = s[:n:n], s[n:]
	return r
}

// heightUnits returns a cell code's height in CellHeight units.
func heightUnits(code uint16) int32 {
	return int32(int16(code)) >> 4
}

// palette maps the distinct NSWE nibbles of one block to packed indexes.
type palette struct {
	bits   uint // index width p
	index  [16]uint8
	packed [8]byte // 2^bits nibbles
}

func newPalette(seen uint16) palette {
	var p palette
	n := 0
	for v := range 16 {
		if seen&(1<<v) != 0 {
			p.index[v] = uint8(n)
			p.packed[n>>1] |= uint8(v) << (uint(n&1) * 4)
			n++
		}
	}
	p.bits = uint(bits.Len(uint(n - 1)))
	return p
}

func (p *palette) append(dst []byte) []byte {
	return append(dst, p.packed[:paletteBytes(p.bits)]...)
}

// bitWriter appends little-endian bit fields to a byte slice.
type bitWriter struct {
	acc uint64
	n   uint
}

func (w *bitWriter) write(dst []byte, v uint64, width uint) []byte {
	w.acc |= v << w.n
	w.n += width
	for w.n >= 8 {
		dst = append(dst, byte(w.acc))
		w.acc >>= 8
		w.n -= 8
	}
	return dst
}

func (w *bitWriter) flush(dst []byte) []byte {
	if w.n > 0 {
		dst = append(dst, byte(w.acc))
	}
	return dst
}

// appendComplex encodes a complex block. It fits a plane to the cell
// heights and keeps it when its residuals need fewer bits than residuals
// over the plain minimum height.
func appendComplex(dst []byte, codes *[CellCount]uint16) []byte {
	var q [CellCount]int32
	var seen uint16
	lo, hi := int32(math.MaxInt32), int32(math.MinInt32)
	for i, code := range codes {
		q[i] = heightUnits(code)
		seen |= 1 << (code & 0xf)
		lo, hi = min(lo, q[i]), max(hi, q[i])
	}
	pal := newPalette(seen)

	k := uint(bits.Len32(uint32(hi - lo)))
	base, gx, gy := lo, int32(0), int32(0)
	if pgx, pgy, plo, span, ok := fitPlane(&q); ok {
		if pk := uint(bits.Len32(uint32(span))); pk < k {
			k, base, gx, gy = pk, plo, pgx, pgy
		}
	}

	mode := byte(k) | byte(pal.bits)<<modePaletteShift
	if gx != 0 || gy != 0 {
		mode |= modeFlag
	}
	dst = append(dst, mode, byte(base), byte(base>>8))
	if mode&modeFlag != 0 {
		dst = append(dst, byte(int8(gx)), byte(int8(gy)))
	}
	dst = pal.append(dst)

	var w bitWriter
	for i, code := range codes {
		residual := q[i] - base - gx*int32(i/CellsY) - gy*int32(i%CellsY)
		dst = w.write(dst, uint64(residual)|uint64(pal.index[code&0xf])<<k, k+pal.bits)
	}
	return w.flush(dst)
}

// fitPlane returns the integer slopes along x and y whose plane leaves the
// narrowest span of cell height residuals, with the lowest residual and that
// span. It starts from the rounded least-squares slopes and walks to better
// neighbouring slopes; the span is convex in the slopes, so the walk ends at
// or next to the best plane. ok is false when the slopes do not fit an i8.
func fitPlane(q *[CellCount]int32) (gx, gy, lo, span int32, ok bool) {
	var sx, sy int32
	for i, v := range q {
		sx += int32(2*(i/CellsY)-(CellsX-1)) * v
		sy += int32(2*(i%CellsY)-(CellsY-1)) * v
	}
	// Each axis sums (2x-7)^2 = 168 over x, times 8 cells per line.
	const denom = 168 * 8
	gx = int32(math.Round(float64(sx) / denom))
	gy = int32(math.Round(float64(sy) / denom))
	fits := func(g int32) bool { return g >= math.MinInt8 && g <= math.MaxInt8 }
	if !fits(gx) || !fits(gy) {
		return 0, 0, 0, 0, false
	}
	lo, hi := planeResiduals(q, gx, gy)
	span = hi - lo
	for improved := true; improved && span > 0; {
		improved = false
		for _, d := range [...][2]int32{{-1, -1}, {-1, 0}, {-1, 1}, {0, -1}, {0, 1}, {1, -1}, {1, 0}, {1, 1}} {
			nx, ny := gx+d[0], gy+d[1]
			if !fits(nx) || !fits(ny) {
				continue
			}
			if l, h := planeResiduals(q, nx, ny); h-l < span {
				gx, gy, lo, span, improved = nx, ny, l, h-l, true
			}
		}
	}
	return gx, gy, lo, span, true
}

func planeResiduals(q *[CellCount]int32, gx, gy int32) (lo, hi int32) {
	lo, hi = math.MaxInt32, math.MinInt32
	for i, v := range q {
		r := v - gx*int32(i/CellsY) - gy*int32(i%CellsY)
		lo, hi = min(lo, r), max(hi, r)
	}
	return lo, hi
}

// appendMultilayer encodes a multilayer block; its layer heights are stored
// over their minimum.
func appendMultilayer(dst []byte, counts *[CellCount]uint8, codes []uint16) []byte {
	var seen uint16
	lo, hi := int32(math.MaxInt32), int32(math.MinInt32)
	for _, code := range codes {
		q := heightUnits(code)
		seen |= 1 << (code & 0xf)
		lo, hi = min(lo, q), max(hi, q)
	}
	pal := newPalette(seen)
	k := uint(bits.Len32(uint32(hi - lo)))

	mode := byte(k) | byte(pal.bits)<<modePaletteShift
	wideStarts := len(codes) > math.MaxUint8
	if wideStarts {
		mode |= modeFlag
	}
	dst = append(dst, mode, byte(lo), byte(lo>>8))
	dst = pal.append(dst)
	start := 0
	for c := 0; c <= CellCount; c++ {
		if wideStarts {
			dst = binary.LittleEndian.AppendUint16(dst, uint16(start))
		} else {
			dst = append(dst, byte(start))
		}
		if c < CellCount {
			start += int(counts[c])
		}
	}

	var w bitWriter
	for _, code := range codes {
		dst = w.write(dst, uint64(heightUnits(code)-lo)|uint64(pal.index[code&0xf])<<k, k+pal.bits)
	}
	return w.flush(dst)
}

func encodeCell(c Cell) uint16 {
	return uint16(c.Height)<<1&0xfff0 | uint16(c.NSWE)&0x000f
}
