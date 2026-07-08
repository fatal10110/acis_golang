package dynamic

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

var _ block.Block = (*Block)(nil)

// cellOverride is one cell's dynamic state: original is the baseline
// captured from base.Cells the first time an object touched the cell,
// current is that baseline with every active object's edits applied.
type cellOverride struct {
	original []block.Cell
	current  []block.Cell
}

// Block overlays dynamic objects on top of one static geodata block. A
// cell with no object touching it, past or present, is answered
// straight from base; only cells an object has actually touched get a
// materialized override, so Block never needs to know which concrete
// Block implementation base is.
//
// Add and Remove mutate Block, unlike every static Block implementation.
// The mutex below makes that safe to call concurrently with reads, but a
// handle returned by Nearest, Above, or Below is only valid until the
// next Add or Remove on this Block.
type Block struct {
	blockX int
	blockY int
	base   block.Block

	mu        sync.RWMutex
	overrides map[int]*cellOverride
	objects   []Object
}

// NewBlock wraps one static geodata block at block-space coordinates.
func NewBlock(blockX, blockY int, base block.Block) *Block {
	return &Block{
		blockX:    blockX,
		blockY:    blockY,
		base:      base,
		overrides: make(map[int]*cellOverride),
	}
}

func (b *Block) Kind() block.Kind { return b.base.Kind() }
func (b *Block) HasGeodata() bool { return b.base.HasGeodata() }

func (b *Block) Layers(x, y int) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if ov, ok := b.overrides[cellIndex(x, y)]; ok {
		return len(ov.current)
	}
	return b.base.Layers(x, y)
}

func (b *Block) HeightNearest(x, y int, z int32) int16 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if ov, ok := b.overrides[cellIndex(x, y)]; ok {
		return ov.current[nearestLayerIndex(ov.current, z)].Height
	}
	return b.base.HeightNearest(x, y, z)
}

func (b *Block) NSWENearest(x, y int, z int32) block.NSWE {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if ov, ok := b.overrides[cellIndex(x, y)]; ok {
		return ov.current[nearestLayerIndex(ov.current, z)].NSWE
	}
	return b.base.NSWENearest(x, y, z)
}

func (b *Block) Nearest(x, y int, z int32) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	ci := cellIndex(x, y)
	if ov, ok := b.overrides[ci]; ok {
		return ci*layerSlot + nearestLayerIndex(ov.current, z)
	}
	return encodeDelegated(b.base.Nearest(x, y, z))
}

func (b *Block) Above(x, y int, z int32) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	ci := cellIndex(x, y)
	if ov, ok := b.overrides[ci]; ok {
		if i := firstAboveIndex(ov.current, z); i >= 0 {
			return ci*layerSlot + i
		}
		return -1
	}
	return encodeDelegated(b.base.Above(x, y, z))
}

func (b *Block) Below(x, y int, z int32) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	ci := cellIndex(x, y)
	if ov, ok := b.overrides[ci]; ok {
		if i := firstBelowIndex(ov.current, z); i >= 0 {
			return ci*layerSlot + i
		}
		return -1
	}
	return encodeDelegated(b.base.Below(x, y, z))
}

func (b *Block) Height(layer int) int16 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if layer >= 0 {
		return b.overrides[layer/layerSlot].current[layer%layerSlot].Height
	}
	return b.base.Height(decodeDelegated(layer))
}

func (b *Block) NSWE(layer int) block.NSWE {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if layer >= 0 {
		return b.overrides[layer/layerSlot].current[layer%layerSlot].NSWE
	}
	return b.base.NSWE(decodeDelegated(layer))
}

func (b *Block) Cells(x, y int) []block.Cell {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if ov, ok := b.overrides[cellIndex(x, y)]; ok {
		return append([]block.Cell(nil), ov.current...)
	}
	return b.base.Cells(x, y)
}

func (b *Block) HeightNearestIgnore(x, y int, z int32, ignore Object) int16 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	ov, ok := b.overrides[cellIndex(x, y)]
	if !ok {
		return b.base.HeightNearest(x, y, z)
	}
	cells := ov.current
	if b.hasObject(ignore) {
		cells = ov.original
	}
	return cells[nearestLayerIndex(cells, z)].Height
}

func (b *Block) NSWENearestIgnore(x, y int, z int32, ignore Object) block.NSWE {
	b.mu.RLock()
	defer b.mu.RUnlock()
	ov, ok := b.overrides[cellIndex(x, y)]
	if !ok {
		return b.base.NSWENearest(x, y, z)
	}
	cells := ov.current
	if b.hasObject(ignore) {
		cells = ov.original
	}
	return cells[nearestLayerIndex(cells, z)].NSWE
}

// Add applies obj to the block if it overlaps.
func (b *Block) Add(obj Object) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.hasObject(obj) {
		return
	}
	b.objects = append(b.objects, obj)
	b.rebuild()
}

// Remove drops obj from the block.
func (b *Block) Remove(obj Object) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.objects {
		if b.objects[i] == obj {
			b.objects = append(b.objects[:i], b.objects[i+1:]...)
			b.rebuild()
			return
		}
	}
}

// hasObject requires b.mu to already be held, for reading or writing.
func (b *Block) hasObject(obj Object) bool {
	for _, existing := range b.objects {
		if existing == obj {
			return true
		}
	}
	return false
}

// override returns the cell's override, seeding it from base.Cells the
// first time it's touched. Requires b.mu to be held for writing.
func (b *Block) override(x, y int) *cellOverride {
	ci := cellIndex(x, y)
	if ov, ok := b.overrides[ci]; ok {
		return ov
	}
	cells := append([]block.Cell(nil), b.base.Cells(x, y)...)
	ov := &cellOverride{
		original: cells,
		current:  append([]block.Cell(nil), cells...),
	}
	b.overrides[ci] = ov
	return ov
}

// rebuild recomputes every override from scratch against the current
// object list, then drops any override no object touches anymore so
// that cell goes back to answering straight from base. Requires b.mu to
// be held for writing.
func (b *Block) rebuild() {
	for _, ov := range b.overrides {
		ov.current = append(ov.current[:0], ov.original...)
	}
	touched := make(map[int]bool, len(b.overrides))

	minBX := b.blockX * block.CellsX
	minBY := b.blockY * block.CellsY
	maxBX := minBX + block.CellsX
	maxBY := minBY + block.CellsY

	for _, obj := range b.objects {
		data := obj.GeoData()
		if len(data) == 0 || len(data[0]) == 0 {
			continue
		}
		minOX, minOY := obj.GeoX(), obj.GeoY()
		minOZ, maxOZ := obj.GeoZ(), obj.GeoZ()+obj.Height()
		minGX := max(minBX, minOX)
		minGY := max(minBY, minOY)
		maxGX := min(maxBX, minOX+len(data))
		maxGY := min(maxBY, minOY+len(data[0]))

		for gx := minGX; gx < maxGX; gx++ {
			for gy := minGY; gy < maxGY; gy++ {
				objNSWE := data[gx-minOX][gy-minOY]
				if objNSWE == block.AllDirections {
					continue
				}

				lx, ly := gx-minBX, gy-minBY
				ov := b.override(lx, ly)
				touched[cellIndex(lx, ly)] = true

				currentIndex := nearestLayerIndex(ov.current, int32(minOZ))
				originalIndex := nearestLayerIndex(ov.original, int32(minOZ))
				if currentIndex < 0 || originalIndex < 0 {
					continue
				}
				if ov.current[currentIndex].Height != ov.original[originalIndex].Height {
					continue
				}

				if objNSWE == block.NoDirections {
					z := maxOZ
					if len(ov.current) > 1 {
						above := firstAboveIndex(ov.current, int32(minOZ))
						if above >= 0 {
							az := int(ov.current[above].Height)
							if az <= maxOZ {
								z = az - block.CellIgnoreHeight
							}
						}
					}
					ov.current[currentIndex] = block.Cell{
						Height: int16(z),
						NSWE:   block.NoDirections,
					}
					continue
				}

				if absInt(int(ov.current[currentIndex].Height)-minOZ) > block.CellIgnoreHeight {
					continue
				}
				ov.current[currentIndex].NSWE &= objNSWE
			}
		}
	}

	for ci := range b.overrides {
		if !touched[ci] {
			delete(b.overrides, ci)
		}
	}
}

const layerSlot = block.MaxLayers + 1

// encodeDelegated packs a handle returned by base into the negative
// range, disjoint from the non-negative range cellIndex*layerSlot+li
// uses for override handles, so Height/NSWE can tell which one it's
// given without any extra state. -1 (not found) passes through
// unchanged either way.
func encodeDelegated(h int) int {
	if h < 0 {
		return -1
	}
	return -2 - h
}

func decodeDelegated(h int) int {
	return -2 - h
}

func nearestLayerIndex(layers []block.Cell, z int32) int {
	best := 0
	limit := int32(^uint32(0) >> 1)
	for i, c := range layers {
		d := abs32(int32(c.Height) - z)
		if d > limit {
			break
		}
		limit = d
		best = i
	}
	return best
}

func firstAboveIndex(layers []block.Cell, z int32) int {
	for i := len(layers) - 1; i >= 0; i-- {
		if int32(layers[i].Height) > z {
			return i
		}
	}
	return -1
}

func firstBelowIndex(layers []block.Cell, z int32) int {
	for i := 0; i < len(layers); i++ {
		if int32(layers[i].Height) < z {
			return i
		}
	}
	return -1
}

func cellIndex(x, y int) int {
	return x*block.CellsY + y
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}
