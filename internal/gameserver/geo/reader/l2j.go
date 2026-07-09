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
func ReadL2J(r io.Reader) (*block.Region, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("geo/reader: read l2j region: %w", err)
	}

	p := l2jParser{data: data}
	region := block.NewRegion()
	for i := 0; i < block.RegionBlockCount; i++ {
		if err := p.block(region, i); err != nil {
			return nil, err
		}
	}
	if p.off != len(data) {
		return nil, fmt.Errorf("geo/reader: l2j region has %d trailing bytes", len(data)-p.off)
	}
	return region, nil
}

type l2jParser struct {
	data []byte
	off  int
}

func (p *l2jParser) block(region *block.Region, i int) error {
	typ, err := p.u8()
	if err != nil {
		return fmt.Errorf("geo/reader: block %d: read type: %w", i, err)
	}

	switch typ {
	case l2jFlat:
		height, err := p.i16()
		if err != nil {
			return fmt.Errorf("geo/reader: block %d flat: read height: %w", i, err)
		}
		region.SetFlat(i, height)
		return nil
	case l2jComplex:
		var cells [block.CellCount]uint16
		for c := range cells {
			code, err := p.u16()
			if err != nil {
				return fmt.Errorf("geo/reader: block %d complex: cell %d: %w", i, c, err)
			}
			cells[c] = code
		}
		region.SetComplexEncoded(i, cells)
		return nil
	case l2jMultilayer:
		var counts [block.CellCount]uint8
		cells := make([]uint16, 0, block.CellCount)
		for c := range counts {
			count, err := p.u8()
			if err != nil {
				return fmt.Errorf("geo/reader: block %d multilayer: cell %d: read layer count: %w", i, c, err)
			}
			if count == 0 || int(count) > block.MaxLayers {
				return fmt.Errorf("geo/reader: block %d multilayer: cell %d: invalid layer count %d", i, c, count)
			}
			counts[c] = count
			for l := 0; l < int(count); l++ {
				code, err := p.u16()
				if err != nil {
					return fmt.Errorf("geo/reader: block %d multilayer: cell %d layer %d: %w", i, c, l, err)
				}
				cells = append(cells, code)
			}
		}
		if err := region.SetMultilayerEncoded(i, counts, cells); err != nil {
			return fmt.Errorf("geo/reader: block %d multilayer: %w", i, err)
		}
		return nil
	default:
		return fmt.Errorf("geo/reader: block %d: unknown l2j block type %#x", i, typ)
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
