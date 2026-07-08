package reader

import (
	"fmt"
	"io"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

const (
	l2jFlat       = 0
	l2jComplex    = 1
	l2jMultilayer = 2
)

// ReadL2J decodes one .l2j geodata region.
func ReadL2J(r io.Reader) ([]block.Block, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("geo/reader: read l2j region: %w", err)
	}

	p := l2jParser{data: data}
	blocks := make([]block.Block, block.RegionBlockCount)
	for i := range blocks {
		b, err := p.block(i)
		if err != nil {
			return nil, err
		}
		blocks[i] = b
	}
	if p.off != len(data) {
		return nil, fmt.Errorf("geo/reader: l2j region has %d trailing bytes", len(data)-p.off)
	}
	return blocks, nil
}

type l2jParser struct {
	data []byte
	off  int
}

func (p *l2jParser) block(i int) (block.Block, error) {
	typ, err := p.u8()
	if err != nil {
		return nil, fmt.Errorf("geo/reader: block %d: read type: %w", i, err)
	}

	switch typ {
	case l2jFlat:
		height, err := p.i16()
		if err != nil {
			return nil, fmt.Errorf("geo/reader: block %d flat: read height: %w", i, err)
		}
		return block.NewFlat(height), nil
	case l2jComplex:
		var cells [block.CellCount]block.Cell
		for c := range cells {
			code, err := p.u16()
			if err != nil {
				return nil, fmt.Errorf("geo/reader: block %d complex: cell %d: %w", i, c, err)
			}
			cells[c] = block.DecodeCell(code)
		}
		return block.NewComplex(cells), nil
	case l2jMultilayer:
		var cells [block.CellCount][]block.Cell
		for c := range cells {
			count, err := p.u8()
			if err != nil {
				return nil, fmt.Errorf("geo/reader: block %d multilayer: cell %d: read layer count: %w", i, c, err)
			}
			if count == 0 || int(count) > block.MaxLayers {
				return nil, fmt.Errorf("geo/reader: block %d multilayer: cell %d: invalid layer count %d", i, c, count)
			}
			layers := make([]block.Cell, count)
			for l := range layers {
				code, err := p.u16()
				if err != nil {
					return nil, fmt.Errorf("geo/reader: block %d multilayer: cell %d layer %d: %w", i, c, l, err)
				}
				layers[l] = block.DecodeCell(code)
			}
			cells[c] = layers
		}
		b, err := block.NewMultilayer(cells)
		if err != nil {
			return nil, fmt.Errorf("geo/reader: block %d multilayer: %w", i, err)
		}
		return b, nil
	default:
		return nil, fmt.Errorf("geo/reader: block %d: unknown l2j block type %#x", i, typ)
	}
}

func (p *l2jParser) u8() (byte, error) {
	if len(p.data)-p.off < 1 {
		return 0, io.ErrUnexpectedEOF
	}
	v := p.data[p.off]
	p.off++
	return v, nil
}

func (p *l2jParser) u16() (uint16, error) {
	if len(p.data)-p.off < 2 {
		return 0, io.ErrUnexpectedEOF
	}
	v := uint16(p.data[p.off]) | uint16(p.data[p.off+1])<<8
	p.off += 2
	return v, nil
}

func (p *l2jParser) i16() (int16, error) {
	v, err := p.u16()
	return int16(v), err
}
