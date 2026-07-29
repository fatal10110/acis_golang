package engine

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
)

type blockKey struct {
	x, y int
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
	if next != nil {
		e.dynamicBlocks.Store(&next)
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
