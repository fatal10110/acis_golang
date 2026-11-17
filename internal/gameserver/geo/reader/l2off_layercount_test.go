package reader

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

// l2offMultilayerRawCount is a multilayer block whose cell 10 stores its
// layer count as the raw value count followed by layers layers; every other
// cell has one layer.
func l2offMultilayerRawCount(count uint16, layers int) []byte {
	var data []byte
	data = put16(data, 1)
	for i := 0; i < block.CellCount; i++ {
		n := 1
		if i == 10 {
			data = put16(data, count)
			n = layers
		} else {
			data = put16(data, 1)
		}
		for l := 0; l < n; l++ {
			data = put16(data, cellCode(block.Cell{Height: int16(8 * l), NSWE: block.AllDirections}))
		}
	}
	return data
}

// Only the low byte of a stored layer count counts: 0x0105 is five layers.
func TestDecodeL2OFFLayerCountUsesLowByte(t *testing.T) {
	region, _, err := decodeL2OFF(l2offRegion(map[int][]byte{2: l2offMultilayerRawCount(0x0105, 5)}))
	if err != nil {
		t.Fatalf("decodeL2OFF(count 0x0105): %v", err)
	}
	if got := region.Layers(0, 2, 1, 2); got != 5 {
		t.Fatalf("Layers = %d, want 5", got)
	}
}

// The low byte is signed, so 0x0080-0x00ff are negative counts.
func TestDecodeL2OFFRejectsNegativeLayerCount(t *testing.T) {
	for _, count := range []uint16{0x0080, 0x00ff, 0x0100} {
		_, _, err := decodeL2OFF(l2offRegion(map[int][]byte{2: l2offMultilayerRawCount(count, 1)}))
		if err == nil || !strings.Contains(err.Error(), "invalid layer count") {
			t.Fatalf("decodeL2OFF(count %#04x) error = %v, want invalid layer count", count, err)
		}
	}
}
