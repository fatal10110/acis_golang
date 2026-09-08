package engine

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
)

type blockKey struct {
	x, y int
}

// regionMask is one bit per block of a single region, set when
// dynamicBlocks holds an overlay for that block; see Engine.dynamicMask.
type regionMask [block.RegionBlockCount / 64]uint64

func maskBit(blockX, blockY int) (word int, bit uint64) {
	i := blockX*block.RegionBlocksY + blockY
	return i >> 6, 1 << (uint(i) & 63)
}

func (m *regionMask) has(blockX, blockY int) bool {
	word, bit := maskBit(blockX, blockY)
	return m[word]&bit != 0
}

// maskUpdate is one region's rebuilt bitmap, not yet published.
type maskUpdate struct {
	tileX, tileY int
	mask         *regionMask
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
				if !add {
					continue
				}
				base := e.blockAtBlock(bx, by)
				if !base.HasGeodata() {
					continue
				}
				if next == nil {
					next = cloneDynamicBlocks(current)
				}
				b = dynamic.NewBlock(bx, by, base)
				next[key] = b
			}
			if add {
				b.Add(obj)
			} else {
				b.Remove(obj)
				if b.Empty() {
					if next == nil {
						next = cloneDynamicBlocks(current)
					}
					delete(next, key)
				}
			}
		}
	}
	if next == nil {
		return
	}

	// A bit visible before its map entry only costs a wasted hash lookup; a
	// map entry visible before its bit would hide a live overlay from
	// concurrent readers. So publish the mask first when adding and last
	// when removing.
	updates := e.rebuildMasks(next, minBX, maxBX, minBY, maxBY)
	if add {
		e.storeMasks(updates)
		e.dynamicBlocks.Store(&next)
		return
	}
	e.dynamicBlocks.Store(&next)
	e.storeMasks(updates)
}

// rebuildMasks returns a fresh bitmap for every region the given block span
// touches, cloned from the current one so concurrent readers keep reading a
// consistent snapshot. Only blocks inside the span can have changed, so
// every other bit is carried over untouched.
func (e *Engine) rebuildMasks(next map[blockKey]*dynamic.Block, minBX, maxBX, minBY, maxBY int) []maskUpdate {
	var updates []maskUpdate
	for bx := minBX; bx <= maxBX; bx++ {
		for by := minBY; by <= maxBY; by++ {
			if bx < 0 || by < 0 {
				continue
			}
			tileX := bx / block.RegionBlocksX
			tileY := by / block.RegionBlocksY
			if tileX >= regionTilesX || tileY >= regionTilesY {
				continue
			}
			i := -1
			for u := range updates {
				if updates[u].tileX == tileX && updates[u].tileY == tileY {
					i = u
					break
				}
			}
			if i < 0 {
				mask := new(regionMask)
				if current := e.dynamicMask[tileX][tileY].Load(); current != nil {
					*mask = *current
				}
				updates = append(updates, maskUpdate{tileX: tileX, tileY: tileY, mask: mask})
				i = len(updates) - 1
			}
			word, bit := maskBit(bx%block.RegionBlocksX, by%block.RegionBlocksY)
			if next[blockKey{bx, by}] != nil {
				updates[i].mask[word] |= bit
			} else {
				updates[i].mask[word] &^= bit
			}
		}
	}
	return updates
}

func (e *Engine) storeMasks(updates []maskUpdate) {
	for _, u := range updates {
		e.dynamicMask[u.tileX][u.tileY].Store(u.mask)
	}
}

// cloneDynamicBlocks copies current's entries into a fresh map so
// toggleObject can insert a newly-created dynamic block without mutating
// the map any concurrent reader may already be holding a pointer to.
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
