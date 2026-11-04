package pathfind

import "github.com/fatal10110/acis_golang/internal/gameserver/geo/block"

// cellCache remembers one search's candidateNSWE answers. Neighboring
// expansions read the same cells at the same z over and over (each cell is
// a neighbor of up to eight others, from both frontiers), and every read is
// a block lookup plus, on multilayer blocks, a layer scan.
//
// It is direct-mapped: a colliding read simply replaces the entry. An entry
// belongs to the current search only while its generation matches, so reset
// starts the next search by bumping the generation instead of clearing.
// A cell read is answered from geodata once per search, so a door that
// changes a cell mid-search reaches this search only through cells it has
// not read yet; the next search reads every cell fresh.
type cellCache struct {
	entries []cellEntry
	gen     uint32
}

type cellEntry struct {
	key    uint64
	gen    uint32
	height int16
	nswe   block.NSWE
	ok     bool
}

const cellCacheBits = 14

func (c *cellCache) reset() {
	if c.entries == nil {
		c.entries = make([]cellEntry, 1<<cellCacheBits)
	}
	c.gen++
	if c.gen == 0 {
		clear(c.entries)
		c.gen = 1
	}
}

// cellReadKey packs a cell read: 20 bits each of geodata X and Y (masked, so
// a read one cell outside the world still gets its own key) and 20 bits of
// the query z, which covers every int16 height plus the step allowance.
func cellReadKey(gx, gy, z int) uint64 {
	return uint64(uint32(gx)&0xFFFFF)<<40 | uint64(uint32(gy)&0xFFFFF)<<20 | uint64(uint32(z)&0xFFFFF)
}

// readCell is candidateNSWE through the search's cell cache.
func (f *Finder) readCell(cells *cellCache, gx, gy, z int) cellRead {
	key := cellReadKey(gx, gy, z)
	e := &cells.entries[(key*0x9E3779B97F4A7C15)>>(64-cellCacheBits)]
	if e.gen == cells.gen && e.key == key {
		return cellRead{height: int(e.height), nswe: e.nswe, ok: e.ok}
	}
	height, nswe, ok := f.candidateNSWE(gx, gy, z)
	*e = cellEntry{key: key, gen: cells.gen, height: int16(height), nswe: nswe, ok: ok}
	return cellRead{height: height, nswe: nswe, ok: ok}
}
