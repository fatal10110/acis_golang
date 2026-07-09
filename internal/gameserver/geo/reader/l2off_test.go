package reader

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

func TestReadL2OFF(t *testing.T) {
	custom := map[int][]byte{
		0: l2offFlatBlock(112),
		1: l2offComplexBlock(func(i int) block.Cell {
			if i == 9 {
				return block.Cell{Height: -16, NSWE: block.North | block.West}
			}
			return block.Cell{Height: int16(i * block.CellHeight), NSWE: block.NSWE(i & 0x0f)}
		}),
		2: l2offMultilayerBlock(func(i int) []block.Cell {
			if i == 10 {
				return []block.Cell{
					{Height: -24, NSWE: block.South},
					{Height: 8, NSWE: block.East | block.West},
					{Height: 40, NSWE: block.North},
				}
			}
			return []block.Cell{{Height: 0, NSWE: block.AllDirections}}
		}),
	}

	path := filepath.Join(t.TempDir(), "20_18_conv.dat")
	if err := os.WriteFile(path, l2offRegion(custom), 0o600); err != nil {
		t.Fatal(err)
	}

	blocks, err := ReadL2OFF(path)
	if err != nil {
		t.Fatalf("ReadL2OFF: %v", err)
	}

	if got := blocks.HeightNearest(0, 0, 7, 7, 0); got != 112 {
		t.Errorf("flat height = %d, want 112", got)
	}
	if got := blocks.NSWENearest(0, 0, 0, 0, 0); got != block.AllDirections {
		t.Errorf("flat nswe = %v, want all", got)
	}

	if got := blocks.KindAt(0, 1); got != block.KindComplex {
		t.Fatalf("block 1 kind = %v, want complex", got)
	}
	if got := blocks.HeightNearest(0, 1, 1, 1, 0); got != -16 {
		t.Errorf("complex cell height = %d, want -16", got)
	}
	if got := blocks.NSWENearest(0, 1, 1, 1, 0); got != block.North|block.West {
		t.Errorf("complex cell nswe = %v, want NW", got)
	}

	if got := blocks.KindAt(0, 2); got != block.KindMultilayer {
		t.Fatalf("block 2 kind = %v, want multilayer", got)
	}
	if got := blocks.Layers(0, 2, 1, 2); got != 3 {
		t.Errorf("multilayer layer count = %d, want 3", got)
	}
	if got := blocks.HeightNearest(0, 2, 1, 2, 7); got != 8 {
		t.Errorf("multilayer nearest height = %d, want 8", got)
	}
	if got := blocks.NSWENearest(0, 2, 1, 2, 7); got != block.East|block.West {
		t.Errorf("multilayer nearest nswe = %v, want EW", got)
	}

	if got := blocks.KindAt(0, 3); got != block.KindFlat {
		t.Errorf("default fixture block kind = %v, want flat", got)
	}
}

func TestDecodeL2OFFRejectsShortHeader(t *testing.T) {
	_, err := decodeL2OFF(make([]byte, l2offHeaderSize-1))
	if err == nil || !strings.Contains(err.Error(), "header") {
		t.Fatalf("decodeL2OFF(short header) error = %v, want header error", err)
	}
}

func TestDecodeL2OFFRejectsTruncatedBlock(t *testing.T) {
	data := make([]byte, l2offHeaderSize)
	data = put16(data, l2offTypeFlat)
	data = put16(data, 80)

	_, err := decodeL2OFF(data)
	if err == nil || !strings.Contains(err.Error(), "flat dummy") {
		t.Fatalf("decodeL2OFF(truncated flat) error = %v, want flat dummy error", err)
	}
}

func TestDecodeL2OFFRejectsBadLayerCount(t *testing.T) {
	data := make([]byte, l2offHeaderSize)
	data = put16(data, 1)
	data = put16(data, 0)

	_, err := decodeL2OFF(data)
	if err == nil || !strings.Contains(err.Error(), "invalid layer count 0") {
		t.Fatalf("decodeL2OFF(bad layer count) error = %v, want invalid layer count", err)
	}
}

func l2offRegion(custom map[int][]byte) []byte {
	data := make([]byte, l2offHeaderSize)
	for i := range data {
		data[i] = 0xff
	}
	for i := 0; i < block.RegionBlockCount; i++ {
		if b, ok := custom[i]; ok {
			data = append(data, b...)
		} else {
			data = append(data, l2offFlatBlock(0)...)
		}
	}
	return data
}

func l2offFlatBlock(height int16) []byte {
	var data []byte
	data = put16(data, l2offTypeFlat)
	data = put16(data, uint16(height))
	data = put16(data, 0)
	return data
}

func l2offComplexBlock(cell func(int) block.Cell) []byte {
	var data []byte
	data = put16(data, l2offTypeComplex)
	for i := 0; i < block.CellCount; i++ {
		data = put16(data, cellCode(cell(i)))
	}
	return data
}

func l2offMultilayerBlock(layersFor func(int) []block.Cell) []byte {
	var data []byte
	data = put16(data, 1)
	for i := 0; i < block.CellCount; i++ {
		layers := layersFor(i)
		data = put16(data, uint16(len(layers)))
		for _, layer := range layers {
			data = put16(data, cellCode(layer))
		}
	}
	return data
}

func cellCode(c block.Cell) uint16 {
	return uint16(c.Height)<<1&0xfff0 | uint16(c.NSWE)&0x000f
}

func put16(data []byte, v uint16) []byte {
	return binary.LittleEndian.AppendUint16(data, v)
}
