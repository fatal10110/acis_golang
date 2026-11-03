package block

import (
	"bytes"
	"encoding/binary"
	"math/rand/v2"
	"strings"
	"testing"
)

// expectBlock compares every cell of a region block with the block object
// holding the same decoded cells, layer handles included, at heights around
// and between every layer.
func expectBlock(t *testing.T, r *Region, blockX, blockY int, want Block) {
	t.Helper()
	if got := r.KindAt(blockX, blockY); got != want.Kind() {
		t.Fatalf("block %d_%d kind = %v, want %v", blockX, blockY, got, want.Kind())
	}
	for x := range CellsX {
		for y := range CellsY {
			cells := want.Cells(x, y)
			if got := r.Cells(blockX, blockY, x, y); !equalCells(got, cells) {
				t.Fatalf("block %d_%d cell %d,%d = %v, want %v", blockX, blockY, x, y, got, cells)
			}
			if got := r.Layers(blockX, blockY, x, y); got != want.Layers(x, y) {
				t.Fatalf("block %d_%d cell %d,%d layers = %d, want %d", blockX, blockY, x, y, got, want.Layers(x, y))
			}
			for _, z := range sampleHeights(cells) {
				got := [3]int{r.Nearest(blockX, blockY, x, y, z), r.Above(blockX, blockY, x, y, z), r.Below(blockX, blockY, x, y, z)}
				exp := [3]int{want.Nearest(x, y, z), want.Above(x, y, z), want.Below(x, y, z)}
				if got != exp {
					t.Fatalf("block %d_%d cell %d,%d z %d: nearest/above/below = %v, want %v", blockX, blockY, x, y, z, got, exp)
				}
				for _, h := range got {
					if h >= 0 && (r.Height(blockX, blockY, h) != want.Height(h) || r.NSWE(blockX, blockY, h) != want.NSWE(h)) {
						t.Fatalf("block %d_%d handle %d = %d/%v, want %d/%v", blockX, blockY, h,
							r.Height(blockX, blockY, h), r.NSWE(blockX, blockY, h), want.Height(h), want.NSWE(h))
					}
				}
				if r.HeightNearest(blockX, blockY, x, y, z) != want.HeightNearest(x, y, z) || r.NSWENearest(blockX, blockY, x, y, z) != want.NSWENearest(x, y, z) {
					t.Fatalf("block %d_%d cell %d,%d z %d: nearest cell mismatch", blockX, blockY, x, y, z)
				}
			}
		}
	}
}

func equalCells(a, b []Cell) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sampleHeights(cells []Cell) []int32 {
	zs := []int32{-40000, 40000}
	for i, c := range cells {
		h := int32(c.Height)
		zs = append(zs, h-1, h, h+1)
		if i > 0 {
			zs = append(zs, (h+int32(cells[i-1].Height))/2)
		}
	}
	return zs
}

func randomCode(rng *rand.Rand) uint16 { return uint16(rng.Uint32()) }

// TestRegionRoundTripsCellCodes packs complex and multilayer blocks of every
// shape the encoder distinguishes (full-range random codes, exact planes,
// single NSWE values, more than 255 layers) and reads every cell and layer
// handle back, both from the built region and from its parsed stored form.
func TestRegionRoundTripsCellCodes(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	b := NewRegionBuilder()
	want := map[int]Block{}

	complexCodes := []func(i int) uint16{
		func(int) uint16 { return randomCode(rng) },
		func(i int) uint16 { return uint16(int16(i/CellsY*-127+i%CellsY*127)<<4) | 0xf },
		func(i int) uint16 { return uint16(int16(2047-i)<<4) | uint16(i%2) },
		func(int) uint16 { low := int16(-2048); return uint16(low<<4) | 0x3 },
	}
	next := 0
	for _, code := range complexCodes {
		for range 50 {
			var codes [CellCount]uint16
			var cells [CellCount]Cell
			for i := range codes {
				codes[i] = code(i)
				cells[i] = DecodeCell(codes[i])
			}
			b.SetComplexEncoded(next, codes)
			want[next] = NewComplex(cells)
			next += 37
		}
	}
	for _, maxLayers := range []int{1, 3, MaxLayers} {
		for range 50 {
			var counts [CellCount]uint8
			var cells [CellCount][]Cell
			var codes []uint16
			for c := range counts {
				counts[c] = uint8(1 + rng.IntN(maxLayers))
				for range counts[c] {
					code := randomCode(rng)
					codes = append(codes, code)
					cells[c] = append(cells[c], DecodeCell(code))
				}
			}
			if err := b.SetMultilayerEncoded(next, counts, codes); err != nil {
				t.Fatal(err)
			}
			ml, err := NewMultilayer(cells)
			if err != nil {
				t.Fatal(err)
			}
			want[next] = ml
			next += 37
		}
	}
	b.SetFlat(next, -32768)
	want[next] = NewFlat(-32768)

	built := b.Build()
	var stored bytes.Buffer
	if _, err := built.WriteTo(&stored); err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseRegion(stored.Bytes())
	if err != nil {
		t.Fatalf("ParseRegion: %v", err)
	}
	for _, r := range []*Region{built, parsed} {
		for i, blk := range want {
			expectBlock(t, r, i/RegionBlocksY, i%RegionBlocksY, blk)
		}
		expectBlock(t, r, 255, 255, &Null{})
	}
}

// TestRegionWideSuperblock fills one superblock past u16 offsets, which
// moves its blocks to a wide row of absolute offsets.
func TestRegionWideSuperblock(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	b := NewRegionBuilder()
	want := map[int]Block{}
	for local := range superblockBlocks {
		i := blockIndex(16+local/superblockSide, 32+local%superblockSide)
		var counts [CellCount]uint8
		var cells [CellCount][]Cell
		var codes []uint16
		for c := range counts {
			counts[c] = 4
			for range 4 {
				code := randomCode(rng)
				codes = append(codes, code)
				cells[c] = append(cells[c], DecodeCell(code))
			}
		}
		if err := b.SetMultilayerEncoded(i, counts, codes); err != nil {
			t.Fatal(err)
		}
		want[i], _ = NewMultilayer(cells)
	}
	b.SetFlat(0, 5)
	want[0] = NewFlat(5)

	r := b.Build()
	if got := len(r.wide) / wideRowSize; got != 1 {
		t.Fatalf("wide rows = %d, want 1", got)
	}
	parsed, err := ParseRegion(r.stored)
	if err != nil {
		t.Fatalf("ParseRegion: %v", err)
	}
	for i, blk := range want {
		expectBlock(t, parsed, i/RegionBlocksY, i%RegionBlocksY, blk)
	}
}

// TestRegionBuildAllocatesExactSize pins that a built region's storage is
// one allocation of exactly its stored size: the index, every block's
// packed bytes and the read padding, with no growth slack.
func TestRegionBuildAllocatesExactSize(t *testing.T) {
	b := NewRegionBuilder()
	var codes [CellCount]uint16
	for i := range codes {
		codes[i] = uint16(i) << 4
	}
	for i := range 1000 {
		b.SetComplexEncoded(i, codes)
	}
	r := b.Build()
	s := r.Stats()
	if got, want := r.StoredSize(), s.IndexBytes+s.ComplexBytes+s.MultilayerBytes; got != want {
		t.Fatalf("stored size = %d, want index+data %d", got, want)
	}
	if cap(r.stored) != len(r.stored) {
		t.Fatalf("stored cap = %d, want len %d", cap(r.stored), len(r.stored))
	}
	if allocs := testing.AllocsPerRun(5, func() { b.Build() }); allocs != 2 {
		t.Fatalf("Build allocations = %v, want 2 (region and its storage)", allocs)
	}
}

// TestParseRegionRejectsMalformed pins that ParseRegion refuses a stored
// region whose header, block offsets or block headers do not fit, instead
// of returning a region that would read out of range.
func TestParseRegionRejectsMalformed(t *testing.T) {
	b := NewRegionBuilder()
	var counts [CellCount]uint8
	codes := make([]uint16, CellCount)
	for c := range counts {
		counts[c] = 1
	}
	if err := b.SetMultilayerEncoded(1, counts, codes); err != nil {
		t.Fatal(err)
	}
	var cells [CellCount]uint16
	b.SetComplexEncoded(0, cells)
	good := b.Build().stored
	if _, err := ParseRegion(good); err != nil {
		t.Fatalf("ParseRegion(good): %v", err)
	}

	dataStart := len(good) - int(binary.LittleEndian.Uint32(good[4:]))
	mutate := func(f func(b []byte) []byte) []byte { return f(bytes.Clone(good)) }
	cases := map[string][]byte{
		"truncated":     good[:len(good)-1],
		"short header":  good[:4],
		"too many wide": mutate(func(b []byte) []byte { binary.LittleEndian.PutUint32(b, superblockCount+1); return b }),
		"bad wide index": mutate(func(b []byte) []byte {
			binary.LittleEndian.PutUint32(b[regionHeaderSize+kindsSize+valuesSize:], wideSuperblock)
			return b
		}),
		"offset past end": mutate(func(b []byte) []byte {
			binary.LittleEndian.PutUint16(b[regionHeaderSize+kindsSize+2:], 0xffff)
			return b
		}),
		"bad mode": mutate(func(b []byte) []byte { b[dataStart] = 0x0f; return b }),
		"bad layer start": mutate(func(b []byte) []byte {
			off := dataStart + int(binary.LittleEndian.Uint16(b[regionHeaderSize+kindsSize+2:]))
			b[off+3+1+5]++
			return b
		}),
	}
	for name, stored := range cases {
		if _, err := ParseRegion(stored); err == nil || !strings.Contains(err.Error(), "geo/block") {
			t.Errorf("%s: ParseRegion error = %v, want a geo/block error", name, err)
		}
	}
}
