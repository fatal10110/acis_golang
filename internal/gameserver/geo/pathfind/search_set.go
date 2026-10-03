package pathfind

// cellKey packs a search cell into one table key: 24 bits each of geodata X
// and Y (the world spans fewer than 2^15 cells per axis, and both are
// non-negative once the search has rejected out-of-world endpoints and
// candidates) and the int16-sourced height in the low 16 bits, so distinct
// cells never share a key.
func cellKey(gx, gy, z int) uint64 {
	return uint64(uint32(gx)&0xFFFFFF)<<40 | uint64(uint32(gy)&0xFFFFFF)<<16 | uint64(uint16(int16(z)))
}

// nodeSet records which cells each frontier of the current search has
// reached. A cell's forward or backward node is set once, when that frontier
// queues it, and stays set after the node is popped: no decision in the
// search distinguishes a queued node from an expanded one, only whether the
// frontier holds the cell at all.
//
// It is an open-addressing, linear-probing table over cellKey. A slot
// belongs to the current search only while its generation matches; reset
// starts the next search by bumping the generation instead of clearing the
// slots.
type nodeSet struct {
	slots []setSlot
	shift uint
	gen   uint32
	used  int
}

type setSlot struct {
	key uint64
	gen uint32
	// fwd and back are the frontiers' nodeArena ids for this cell, 0 when
	// that frontier has not reached it.
	fwd, back int32
}

const minSetBits = 12

func (s *nodeSet) reset() {
	if s.slots == nil {
		s.slots = make([]setSlot, 1<<minSetBits)
		s.shift = 64 - minSetBits
	}
	s.gen++
	if s.gen == 0 {
		clear(s.slots)
		s.gen = 1
	}
	s.used = 0
}

func (s *nodeSet) home(key uint64) uint64 {
	return (key * 0x9E3779B97F4A7C15) >> s.shift
}

// lookup returns key's slot in the current search, or nil when no frontier
// has reached the cell.
func (s *nodeSet) lookup(key uint64) *setSlot {
	mask := uint64(len(s.slots) - 1)
	for i := s.home(key); ; i = (i + 1) & mask {
		slot := &s.slots[i]
		if slot.gen != s.gen {
			return nil
		}
		if slot.key == key {
			return slot
		}
	}
}

// slot returns key's slot in the current search, claiming an empty one when
// no frontier has reached the cell yet. The pointer stays valid until the
// next call to slot.
func (s *nodeSet) slot(key uint64) *setSlot {
	if 2*(s.used+1) > len(s.slots) {
		s.grow()
	}
	mask := uint64(len(s.slots) - 1)
	for i := s.home(key); ; i = (i + 1) & mask {
		slot := &s.slots[i]
		if slot.gen != s.gen {
			*slot = setSlot{key: key, gen: s.gen}
			s.used++
			return slot
		}
		if slot.key == key {
			return slot
		}
	}
}

func (s *nodeSet) grow() {
	old := s.slots
	s.slots = make([]setSlot, 2*len(old))
	s.shift--
	mask := uint64(len(s.slots) - 1)
	for _, slot := range old {
		if slot.gen != s.gen {
			continue
		}
		i := s.home(slot.key)
		for s.slots[i].gen == s.gen {
			i = (i + 1) & mask
		}
		s.slots[i] = slot
	}
}

const nodeChunkSize = 1024

// nodeArena hands out a search's nodes from fixed-size chunks, so a node's
// address never moves while the search links parents to it, and gives each
// node a 1-based id that nodeSet stores in place of a pointer.
type nodeArena struct {
	chunks []*[nodeChunkSize]node
	n      int32
}

func (a *nodeArena) reset() {
	a.n = 0
}

func (a *nodeArena) alloc() *node {
	i := int(a.n)
	if i/nodeChunkSize == len(a.chunks) {
		a.chunks = append(a.chunks, new([nodeChunkSize]node))
	}
	a.n++
	n := &a.chunks[i/nodeChunkSize][i%nodeChunkSize]
	*n = node{id: a.n, index: -1}
	return n
}

// at returns the node with id, which must have come from alloc since the
// last reset.
func (a *nodeArena) at(id int32) *node {
	i := int(id - 1)
	return &a.chunks[i/nodeChunkSize][i%nodeChunkSize]
}
