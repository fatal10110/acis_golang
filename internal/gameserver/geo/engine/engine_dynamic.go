package engine

import (
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
)

type blockKey struct {
	x, y int
}

// regionMask is one bit per block of a single region, set while that
// block's dynamicBlocks entry carries at least one object; see
// Engine.dynamicMask. Bits flip in place with atomic word operations, so a
// toggle never copies the bitmap.
type regionMask [block.RegionBlockCount / 64]atomic.Uint64

func maskBit(blockX, blockY int) (word int, bit uint64) {
	i := blockX*block.RegionBlocksY + blockY
	return i >> 6, 1 << (uint(i) & 63)
}

func (m *regionMask) has(blockX, blockY int) bool {
	word, bit := maskBit(blockX, blockY)
	return m[word].Load()&bit != 0
}

func (e *Engine) toggleObject(obj dynamic.Object, add bool) {
	if obj == nil {
		return
	}
	data := obj.GeoData()
	if len(data) == 0 || len(data[0]) == 0 {
		return
	}

	minBX := obj.GeoX() / block.CellsX
	maxBX := (obj.GeoX() + len(data) - 1) / block.CellsX
	minBY := obj.GeoY() / block.CellsY
	maxBY := (obj.GeoY() + len(data[0]) - 1) / block.CellsY

	e.dynamicBlocksMu.Lock()
	defer e.dynamicBlocksMu.Unlock()

	if !add {
		// A block whose last object leaves stays in the map, so the next
		// close of the same door finds it there and the map is never
		// copied again. Clearing its bit sends readers back to the static
		// block; until then they read the emptied overlay, which answers
		// exactly as the static block does.
		current := e.dynamicBlocks.Load()
		if current == nil {
			return
		}
		for bx := minBX; bx <= maxBX; bx++ {
			for by := minBY; by <= maxBY; by++ {
				b := (*current)[blockKey{bx, by}]
				if b == nil {
					continue
				}
				b.Remove(obj)
				if b.Empty() {
					e.setMaskBit(bx, by, false)
				}
			}
		}
		return
	}

	current := e.dynamicBlocks.Load()
	var next map[blockKey]*dynamic.Block
	for bx := minBX; bx <= maxBX; bx++ {
		for by := minBY; by <= maxBY; by++ {
			key := blockKey{bx, by}
			var b *dynamic.Block
			if next != nil {
				b = next[key]
			} else if current != nil {
				b = (*current)[key]
			}
			if b == nil {
				base := e.blockAtBlock(bx, by)
				if !base.HasGeodata() {
					continue
				}
				if next == nil {
					next = cloneDynamicBlocks(current)
				}
				b = dynamic.NewBlock(bx, by, base)
				next[key] = b
				e.overlaid.LoadOrStore(key, b.Kind() == block.KindMultilayer)
			}
			b.Add(obj)
		}
	}

	// A set bit must always find its entry, so a block the map did not
	// have yet is published before any bit is set for it.
	if next != nil {
		// A variable of its own, so only this branch heap-allocates it.
		published := next
		e.dynamicBlocks.Store(&published)
		current = &published
	}
	if current == nil {
		return
	}
	for bx := minBX; bx <= maxBX; bx++ {
		for by := minBY; by <= maxBY; by++ {
			if (*current)[blockKey{bx, by}] != nil {
				e.setMaskBit(bx, by, true)
			}
		}
	}
}

// setMaskBit flips the gate bit of one global block, allocating its
// region's bitmap on first use. Requires dynamicBlocksMu.
func (e *Engine) setMaskBit(bx, by int, on bool) {
	if bx < 0 || by < 0 {
		return
	}
	tileX := bx / block.RegionBlocksX
	tileY := by / block.RegionBlocksY
	if tileX >= regionTilesX || tileY >= regionTilesY {
		return
	}
	mask := e.dynamicMask[tileX][tileY].Load()
	if mask == nil {
		if !on {
			return
		}
		mask = new(regionMask)
		e.dynamicMask[tileX][tileY].Store(mask)
	}
	word, bit := maskBit(bx%block.RegionBlocksX, by%block.RegionBlocksY)
	if on {
		mask[word].Or(bit)
	} else {
		mask[word].And(^bit)
	}
}

// cloneDynamicBlocks copies current's entries into a fresh map so
// toggleObject can insert a newly-created dynamic block without mutating
// the map any concurrent reader may already be holding a pointer to. Each
// block key is cloned in at most once: entries are never deleted.
func cloneDynamicBlocks(current *map[blockKey]*dynamic.Block) map[blockKey]*dynamic.Block {
	size := 0
	if current != nil {
		size = len(*current)
	}
	next := make(map[blockKey]*dynamic.Block, size+1)
	if current != nil {
		for k, v := range *current {
			next[k] = v
		}
	}
	return next
}

// blockAtBlock resolves the static region block underlying (blockX, blockY).
// Safe without synchronization: regions is only ever written by SetRegion
// during boot, before the engine is handed to any concurrent caller.
