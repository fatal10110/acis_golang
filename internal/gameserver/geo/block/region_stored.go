package block

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// A stored region is a u32 wide-row count and a u32 data length (dataPad
// included), then the kinds, values, bases, wide and data sections.

// WriteTo writes the region's stored form, which ParseRegion reads back.
func (r *Region) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(r.stored)
	return int64(n), err
}

// StoredSize returns the size of the region's stored form.
func (r *Region) StoredSize() int {
	return len(r.stored)
}

// ParseRegion returns the region stored in b, as WriteTo wrote it, after
// checking that every block lies inside it; a malformed region is an error,
// never a later out-of-range read. The region reads b in place, so b must
// stay unchanged for as long as the region is used.
func ParseRegion(b []byte) (*Region, error) {
	if len(b) < regionHeaderSize {
		return nil, errors.New("geo/block: stored region is truncated")
	}
	wideRows := binary.LittleEndian.Uint32(b)
	dataLen := binary.LittleEndian.Uint32(b[4:])
	if wideRows > superblockCount || dataLen < dataPad || dataLen >= wideSuperblock {
		return nil, fmt.Errorf("geo/block: stored region has %d wide rows and %d data bytes", wideRows, dataLen)
	}
	if want := storedSize(int(wideRows), int(dataLen)); len(b) != want {
		return nil, fmt.Errorf("geo/block: stored region is %d bytes, want %d", len(b), want)
	}
	r := viewRegion(b[:len(b):len(b)], int(wideRows))
	if err := r.validate(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Region) validate() error {
	limit := len(r.data) - dataPad
	for sb := range superblockCount {
		base := binary.LittleEndian.Uint32(r.bases[sb*4:])
		if base&wideSuperblock != 0 && int(base&^wideSuperblock) >= len(r.wide)/wideRowSize ||
			base&wideSuperblock == 0 && int(base) > limit {
			return fmt.Errorf("geo/block: superblock %d has invalid base %#x", sb, base)
		}
	}
	for bx := range RegionBlocksX {
		for by := range RegionBlocksY {
			kind := r.kind(blockIndex(bx, by))
			if kind != regionComplex && kind != regionMultilayer {
				continue
			}
			off := r.offset(bx, by)
			n, err := packedSize(r.data[:limit], off, kind)
			if err == nil && off+n > limit {
				err = errors.New("block overruns region data")
			}
			if err != nil {
				return fmt.Errorf("geo/block: block %d_%d: %w", bx, by, err)
			}
		}
	}
	return nil
}

// packedSize returns the size of the packed block at data[off:], checking
// its header and, for a multilayer block, its layer starts.
func packedSize(data []byte, off int, kind uint8) (int, error) {
	if off >= len(data) {
		return 0, errors.New("block starts past region data")
	}
	mode := data[off]
	k, p := uint(mode&modeResidualMask), uint(mode>>modePaletteShift&modePaletteMask)
	if k > maxResidualBits || p > maxPaletteBits {
		return 0, fmt.Errorf("invalid mode %#x", mode)
	}
	head := 3 + paletteBytes(p)
	if kind == regionComplex {
		if mode&modeFlag != 0 {
			head += 2
		}
		return head + CellCount*int(k+p)/8, nil
	}

	width := 1
	if mode&modeFlag != 0 {
		width = 2
	}
	starts := off + head
	if starts+layerStarts*width > len(data) {
		return 0, errors.New("layer starts overrun region data")
	}
	prev := 0
	for c := range layerStarts {
		var s int
		if width == 2 {
			s = int(binary.LittleEndian.Uint16(data[starts+c*2:]))
		} else {
			s = int(data[starts+c])
		}
		if c == 0 && s != 0 || c > 0 && (s-prev < 1 || s-prev > MaxLayers) {
			return 0, fmt.Errorf("cell %d has invalid layer start %d", c, s)
		}
		prev = s
	}
	return head + layerStarts*width + (prev*int(k+p)+7)/8, nil
}

// RegionStats summarizes a region's blocks and stored bytes by kind.
type RegionStats struct {
	FlatBlocks, ComplexBlocks, MultilayerBlocks int
	// IndexBytes covers the stored-region header and the kinds, values,
	// bases and wide sections; data padding counts as complex bytes.
	IndexBytes, ComplexBytes, MultilayerBytes int
}

// Add accumulates o into s.
func (s *RegionStats) Add(o RegionStats) {
	s.FlatBlocks += o.FlatBlocks
	s.ComplexBlocks += o.ComplexBlocks
	s.MultilayerBlocks += o.MultilayerBlocks
	s.IndexBytes += o.IndexBytes
	s.ComplexBytes += o.ComplexBytes
	s.MultilayerBytes += o.MultilayerBytes
}

// Stats summarizes the region's blocks and stored bytes by kind.
func (r *Region) Stats() RegionStats {
	s := RegionStats{IndexBytes: len(r.stored) - len(r.data), ComplexBytes: dataPad}
	for bx := range RegionBlocksX {
		for by := range RegionBlocksY {
			kind := r.kind(blockIndex(bx, by))
			switch kind {
			case regionFlat:
				s.FlatBlocks++
			case regionComplex:
				n, _ := packedSize(r.data, r.offset(bx, by), kind)
				s.ComplexBlocks++
				s.ComplexBytes += n
			case regionMultilayer:
				n, _ := packedSize(r.data, r.offset(bx, by), kind)
				s.MultilayerBlocks++
				s.MultilayerBytes += n
			}
		}
	}
	return s
}
