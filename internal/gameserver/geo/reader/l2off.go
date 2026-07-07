package reader

import (
	"encoding/binary"
	"fmt"
	"os"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

const (
	l2offHeaderSize  = 18
	l2offTypeFlat    = 0x0000
	l2offTypeComplex = 0x0040
)

// ReadL2OFF loads a little-endian L2OFF _conv.dat geodata region.
func ReadL2OFF(path string) ([]block.Block, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read L2OFF region %s: %w", path, err)
	}
	blocks, err := decodeL2OFF(data)
	if err != nil {
		return nil, fmt.Errorf("read L2OFF region %s: %w", path, err)
	}
	return blocks, nil
}

func decodeL2OFF(data []byte) ([]block.Block, error) {
	r := l2offReader{data: data}
	if !r.skip(l2offHeaderSize) {
		return nil, shortL2OFF(-1, "header", r.pos)
	}

	blocks := make([]block.Block, block.RegionBlockCount)
	for i := 0; i < block.RegionBlockCount; i++ {
		typ, ok := r.u16()
		if !ok {
			return nil, shortL2OFF(i, "block type", r.pos)
		}

		var (
			b   block.Block
			err error
		)
		switch typ {
		case l2offTypeFlat:
			b, err = r.flat(i)
		case l2offTypeComplex:
			b, err = r.complex(i)
		default:
			b, err = r.multilayer(i)
		}
		if err != nil {
			return nil, err
		}
		blocks[i] = b
	}
	return blocks, nil
}

type l2offReader struct {
	data []byte
	pos  int
}

func (r *l2offReader) skip(n int) bool {
	if len(r.data)-r.pos < n {
		return false
	}
	r.pos += n
	return true
}

func (r *l2offReader) u16() (uint16, bool) {
	if len(r.data)-r.pos < 2 {
		return 0, false
	}
	v := binary.LittleEndian.Uint16(r.data[r.pos:])
	r.pos += 2
	return v, true
}

func (r *l2offReader) flat(blockIndex int) (block.Block, error) {
	height, ok := r.u16()
	if !ok {
		return nil, shortL2OFF(blockIndex, "flat height", r.pos)
	}
	if _, ok := r.u16(); !ok {
		return nil, shortL2OFF(blockIndex, "flat dummy", r.pos)
	}
	return block.NewFlat(int16(height)), nil
}

func (r *l2offReader) complex(blockIndex int) (block.Block, error) {
	var cells [block.CellCount]block.Cell
	for i := range cells {
		code, ok := r.u16()
		if !ok {
			return nil, shortL2OFF(blockIndex, "complex cell", r.pos)
		}
		cells[i] = block.DecodeCell(code)
	}
	return block.NewComplex(cells), nil
}

func (r *l2offReader) multilayer(blockIndex int) (block.Block, error) {
	var cells [block.CellCount][]block.Cell
	for cell := range cells {
		count, ok := r.u16()
		if !ok {
			return nil, shortL2OFF(blockIndex, "layer count", r.pos)
		}
		if count == 0 || count > block.MaxLayers {
			return nil, fmt.Errorf("geo/reader: block %d cell %d: invalid layer count %d", blockIndex, cell, count)
		}

		layers := make([]block.Cell, int(count))
		for layer := range layers {
			code, ok := r.u16()
			if !ok {
				return nil, shortL2OFF(blockIndex, "layer data", r.pos)
			}
			layers[layer] = block.DecodeCell(code)
		}
		cells[cell] = layers
	}

	b, err := block.NewMultilayer(cells)
	if err != nil {
		return nil, fmt.Errorf("geo/reader: block %d: %w", blockIndex, err)
	}
	return b, nil
}

func shortL2OFF(blockIndex int, field string, offset int) error {
	if blockIndex < 0 {
		return fmt.Errorf("geo/reader: short L2OFF region: read %s at offset %d", field, offset)
	}
	return fmt.Errorf("geo/reader: short L2OFF region: block %d: read %s at offset %d", blockIndex, field, offset)
}
