package dynamic

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

// Block overlays dynamic objects on top of one static geodata block.
type Block struct {
	blockX int
	blockY int

	kind     block.Kind
	original [block.CellCount][]block.Cell
	current  [block.CellCount][]block.Cell
	objects  []Object
}

// NewBlock wraps one static geodata block at block-space coordinates.
func NewBlock(blockX, blockY int, base block.Block) *Block {
	b := &Block{
		blockX: blockX,
		blockY: blockY,
		kind:   base.Kind(),
	}
	for x := 0; x < block.CellsX; x++ {
		for y := 0; y < block.CellsY; y++ {
			i := cellIndex(x, y)
			layers := baseLayers(base, x, y)
			b.original[i] = layers
			b.current[i] = append([]block.Cell(nil), layers...)
		}
	}
	return b
}

func (b *Block) Kind() block.Kind       { return b.kind }
func (b *Block) HasGeodata() bool       { return true }
func (b *Block) Layers(x, y int) int    { return len(b.current[cellIndex(x, y)]) }
func (b *Block) Height(layer int) int16 { return b.current[layer/layerSlot][layer%layerSlot].Height }
func (b *Block) NSWE(layer int) block.NSWE {
	return b.current[layer/layerSlot][layer%layerSlot].NSWE
}

func (b *Block) HeightNearest(x, y int, z int32) int16 {
	return b.heightNearestFrom(b.current, x, y, z)
}
func (b *Block) NSWENearest(x, y int, z int32) block.NSWE {
	return b.nsweNearestFrom(b.current, x, y, z)
}
func (b *Block) Nearest(x, y int, z int32) int {
	return nearestHandle(b.current[cellIndex(x, y)], cellIndex(x, y), z)
}
func (b *Block) Above(x, y int, z int32) int {
	return aboveHandle(b.current[cellIndex(x, y)], cellIndex(x, y), z)
}
func (b *Block) Below(x, y int, z int32) int {
	return belowHandle(b.current[cellIndex(x, y)], cellIndex(x, y), z)
}
func (b *Block) HeightNearestIgnore(x, y int, z int32, ignore Object) int16 {
	return b.heightNearestFrom(b.view(ignore), x, y, z)
}
func (b *Block) NSWENearestIgnore(x, y int, z int32, ignore Object) block.NSWE {
	return b.nsweNearestFrom(b.view(ignore), x, y, z)
}

// Add applies obj to the block if it overlaps.
func (b *Block) Add(obj Object) {
	if b.hasObject(obj) {
		return
	}
	b.objects = append(b.objects, obj)
	b.rebuild()
}

// Remove drops obj from the block.
func (b *Block) Remove(obj Object) {
	for i := range b.objects {
		if b.objects[i] == obj {
			b.objects = append(b.objects[:i], b.objects[i+1:]...)
			b.rebuild()
			return
		}
	}
}

func (b *Block) hasObject(obj Object) bool {
	for _, existing := range b.objects {
		if existing == obj {
			return true
		}
	}
	return false
}

func (b *Block) view(ignore Object) [block.CellCount][]block.Cell {
	if b.hasObject(ignore) {
		return b.original
	}
	return b.current
}

func (b *Block) rebuild() {
	for i := range b.original {
		b.current[i] = append(b.current[i][:0], b.original[i]...)
	}

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

				ci := cellIndex(gx-minBX, gy-minBY)
				currentIndex := nearestLayerIndex(b.current[ci], int32(minOZ))
				originalIndex := nearestLayerIndex(b.original[ci], int32(minOZ))
				if currentIndex < 0 || originalIndex < 0 {
					continue
				}
				if b.current[ci][currentIndex].Height != b.original[ci][originalIndex].Height {
					continue
				}

				if objNSWE == block.NoDirections {
					z := maxOZ
					if len(b.current[ci]) > 1 {
						above := firstAboveIndex(b.current[ci], int32(minOZ))
						if above >= 0 {
							az := int(b.current[ci][above].Height)
							if az <= maxOZ {
								z = az - block.CellIgnoreHeight
							}
						}
					}
					b.current[ci][currentIndex] = block.Cell{
						Height: int16(z),
						NSWE:   block.NoDirections,
					}
					continue
				}

				if abs32(int(b.current[ci][currentIndex].Height)-minOZ) > block.CellIgnoreHeight {
					continue
				}
				b.current[ci][currentIndex].NSWE &= objNSWE
			}
		}
	}
}

func (b *Block) heightNearestFrom(cells [block.CellCount][]block.Cell, x, y int, z int32) int16 {
	ci := cellIndex(x, y)
	return cells[ci][nearestLayerIndex(cells[ci], z)].Height
}

func (b *Block) nsweNearestFrom(cells [block.CellCount][]block.Cell, x, y int, z int32) block.NSWE {
	ci := cellIndex(x, y)
	return cells[ci][nearestLayerIndex(cells[ci], z)].NSWE
}

const layerSlot = block.MaxLayers + 1

func baseLayers(base block.Block, x, y int) []block.Cell {
	switch b := base.(type) {
	case *block.Flat:
		return []block.Cell{b.Cell(x, y)}
	case *block.Complex:
		return []block.Cell{b.Cell(x, y)}
	case *block.Multilayer:
		return b.CellLayers(x, y)
	case *block.Null:
		return []block.Cell{{Height: 0, NSWE: block.AllDirections}}
	default:
		panic("geo/dynamic: unsupported block type")
	}
}

func nearestHandle(layers []block.Cell, cellIndex int, z int32) int {
	return cellIndex*layerSlot + nearestLayerIndex(layers, z)
}

func aboveHandle(layers []block.Cell, cellIndex int, z int32) int {
	i := firstAboveIndex(layers, z)
	if i < 0 {
		return -1
	}
	return cellIndex*layerSlot + i
}

func belowHandle(layers []block.Cell, cellIndex int, z int32) int {
	i := firstBelowIndex(layers, z)
	if i < 0 {
		return -1
	}
	return cellIndex*layerSlot + i
}

func nearestLayerIndex(layers []block.Cell, z int32) int {
	best := 0
	limit := int32(^uint32(0) >> 1)
	for i, c := range layers {
		d := abs(int32(c.Height) - z)
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func abs32(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func abs(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}
