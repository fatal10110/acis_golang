package reader

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

func TestReadL2JDecodesRegionBlocks(t *testing.T) {
	blocks := map[int][]byte{
		0: flatBlock(-32),
	}

	var complexCells [block.CellCount]uint16
	complexCells[cellIndex(2, 3)] = l2jCellCode(64, block.North|block.East)
	blocks[1] = complexBlock(complexCells)

	var multilayerCells [block.CellCount][]uint16
	for i := range multilayerCells {
		multilayerCells[i] = []uint16{l2jCellCode(0, block.AllDirections)}
	}
	multilayerCells[cellIndex(4, 5)] = []uint16{
		l2jCellCode(-16, block.West),
		l2jCellCode(48, block.South|block.East),
	}
	blocks[2] = multilayerBlock(multilayerCells)

	data := flatRegion(0, blocks)
	region, err := ReadL2J(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ReadL2J: %v", err)
	}

	if got := region.KindAt(0, 0); got != block.KindFlat {
		t.Fatalf("block 0 kind = %v, want flat", got)
	}
	if got := region.HeightNearest(0, 0, 7, 7, 123); got != -32 {
		t.Errorf("flat height = %d, want -32", got)
	}

	if got := region.KindAt(0, 1); got != block.KindComplex {
		t.Fatalf("block 1 kind = %v, want complex", got)
	}
	if got := region.HeightNearest(0, 1, 2, 3, 0); got != 64 {
		t.Errorf("complex height = %d, want 64", got)
	}
	if got := region.NSWENearest(0, 1, 2, 3, 0); got != block.North|block.East {
		t.Errorf("complex nswe = %v, want NE", got)
	}

	if got := region.KindAt(0, 2); got != block.KindMultilayer {
		t.Fatalf("block 2 kind = %v, want multilayer", got)
	}
	if got := region.Layers(0, 2, 4, 5); got != 2 {
		t.Errorf("multilayer layers = %d, want 2", got)
	}
	if got := region.HeightNearest(0, 2, 4, 5, 40); got != 48 {
		t.Errorf("multilayer nearest height = %d, want 48", got)
	}
	if got := region.NSWENearest(0, 2, 4, 5, 40); got != block.South|block.East {
		t.Errorf("multilayer nearest nswe = %v, want SE", got)
	}

	if got := region.KindAt(0, 3); got != block.KindFlat {
		t.Errorf("block 3 kind = %v, want default flat", got)
	}
}

func TestReadL2JRejectsMalformedRegions(t *testing.T) {
	valid := flatRegion(0, nil)
	withTrailing := append(append([]byte(nil), valid...), 0xff)

	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "short flat", data: []byte{l2jFlat, 0x01}},
		{name: "short complex cell", data: []byte{l2jComplex, 0x01}},
		{name: "zero multilayer count", data: []byte{l2jMultilayer, 0}},
		{name: "too many multilayer layers", data: []byte{l2jMultilayer, block.MaxLayers + 1}},
		{name: "unknown block type", data: []byte{0x7f}},
		{name: "trailing bytes", data: withTrailing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ReadL2J(bytes.NewReader(tt.data)); err == nil {
				t.Fatal("ReadL2J err = nil, want error")
			}
		})
	}
}

func flatRegion(height int16, blocks map[int][]byte) []byte {
	data := make([]byte, 0, block.RegionBlockCount*3)
	for i := range block.RegionBlockCount {
		if b := blocks[i]; b != nil {
			data = append(data, b...)
			continue
		}
		data = append(data, flatBlock(height)...)
	}
	return data
}

func flatBlock(height int16) []byte {
	var data [3]byte
	data[0] = l2jFlat
	binary.LittleEndian.PutUint16(data[1:], uint16(height))
	return data[:]
}

func complexBlock(cells [block.CellCount]uint16) []byte {
	data := []byte{l2jComplex}
	for _, cell := range cells {
		data = binary.LittleEndian.AppendUint16(data, cell)
	}
	return data
}

func multilayerBlock(cells [block.CellCount][]uint16) []byte {
	data := []byte{l2jMultilayer}
	for _, layers := range cells {
		data = append(data, byte(len(layers)))
		for _, cell := range layers {
			data = binary.LittleEndian.AppendUint16(data, cell)
		}
	}
	return data
}

func l2jCellCode(height int16, nswe block.NSWE) uint16 {
	return uint16(int16(height<<1))&0xfff0 | uint16(nswe)
}

func cellIndex(x, y int) int {
	return x*block.CellsY + y
}
