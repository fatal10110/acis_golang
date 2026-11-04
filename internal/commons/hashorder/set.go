// Package hashorder keeps sets of ids in the order a concurrent hash table
// lists them in, the order several client-visible lists (clan wars, clan
// members and skills, clan hall bidders) are sent in.
package hashorder

import "slices"

// Hash-table geometry of the listing order.
const (
	setMinTable    = 16 // buckets of a fresh table
	setTreeifyAt   = 8  // chain length at which a bucket is treated as crowded
	setTreeMinSize = 64 // smallest table whose crowded bucket becomes a tree
	setUntreeifyAt = 6  // a tree bucket this small or smaller turns back into a chain
)

// Set is a set of ids (clan ids, skill ids) that remembers the order a
// concurrent hash table lists them in: the table starts at 16 buckets and
// doubles whenever it holds three quarters of its size, a bucket chains its
// ids in insertion order, and a doubling reorders a chain (see grow).
// Removing an id only unlinks it, so the table never shrinks and a set that
// once held twelve or more ids keeps its larger table after they leave.
//
// A chain that already holds eight ids when a ninth arrives is crowded: in
// a table below 64 buckets the table doubles until three quarters of its
// size reach four times the size it had (presize: 16 buckets become 128,
// 32 become 256); from 64 buckets the chain becomes a tree bucket. A tree
// bucket lists its ids newest first and keeps their order when the table
// doubles, but its ids also form a red-black tree, and the shape of that
// tree decides when the bucket turns back into a chain: on a removal when
// the tree is too small (see shrinks), or on a doubling that leaves six or
// fewer ids in a half.
//
// The zero value is an empty set. It is not safe for concurrent use; the
// owner's lock guards it.
type Set struct {
	table []bucket
	count int
}

// bucket is one bucket: its ids in listing order and, for a tree bucket,
// the root of the red-black tree over them.
type bucket struct {
	ids  []int32
	root *treeNode
}

// treeNode is a node of a tree bucket's red-black tree, ordered by hash and
// then by id. kid[0] is the left child, kid[1] the right one.
type treeNode struct {
	id     int32
	parent *treeNode
	kid    [2]*treeNode
	red    bool
}

// Len returns how many ids the set holds.
func (s *Set) Len() int { return s.count }

// Has reports whether id is in the set.
func (s *Set) Has(id int32) bool {
	if s.count == 0 {
		return false
	}
	return slices.Contains(s.table[s.index(id)].ids, id)
}

func (s *Set) index(id int32) int {
	return int(foldedHash(id) & uint32(len(s.table)-1))
}

// Add puts id in the set; an id already in it keeps its place.
func (s *Set) Add(id int32) {
	if s.table == nil {
		s.table = make([]bucket, setMinTable)
	}
	if s.Has(id) {
		return
	}
	bin := &s.table[s.index(id)]
	crowded := bin.root == nil && len(bin.ids) >= setTreeifyAt
	if bin.root != nil {
		bin.ids = slices.Insert(bin.ids, 0, id)
		bin.root = treeInsert(bin.root, id)
	} else {
		bin.ids = append(bin.ids, id)
	}
	if crowded {
		if len(s.table) < setTreeMinSize {
			// presize: double until three quarters of the table
			// reach four times its current size
			for want := 4 * len(s.table); want > threshold(len(s.table)); {
				s.grow()
			}
		} else {
			bin.root = buildTree(bin.ids)
		}
	}
	s.count++
	for s.count >= threshold(len(s.table)) {
		s.grow()
	}
}

// threshold is the count at which a table of n buckets doubles.
func threshold(n int) int { return n - n/4 }

// Remove takes id out of the set; the table keeps its size.
func (s *Set) Remove(id int32) {
	if !s.Has(id) {
		return
	}
	bin := &s.table[s.index(id)]
	at := slices.Index(bin.ids, id)
	bin.ids = slices.Delete(bin.ids, at, at+1)
	s.count--
	switch {
	case bin.root == nil:
	case len(bin.ids) == 0 || shrinks(bin.root):
		bin.root = nil // the bucket turns back into a chain, in listing order
	default:
		bin.root = treeRemove(bin.root, treeFind(bin.root, id))
	}
}

// Clear empties the set; the table keeps its size.
func (s *Set) Clear() {
	clear(s.table)
	s.count = 0
}

// Order lists the ids in the Order the client is sent them.
func (s *Set) Order() []int32 {
	out := make([]int32, 0, s.count)
	for _, bin := range s.table {
		out = append(out, bin.ids...)
	}
	return out
}

// foldedHash is an id's hash: the id folded with its high half, without
// the sign bit.
func foldedHash(id int32) uint32 {
	h := uint32(id)
	return (h ^ h>>16) & 0x7fffffff
}

// grow doubles the table. A chain splits by the id's next hash bit into the
// bucket at the same index (low half) and the one n above (high half): the
// chain's trailing run of ids sharing one bit moves as it is, every id
// before that run is put in front of its half's chain one by one, so those
// come out in reverse. A tree bucket splits keeping its order: a half of six
// ids or fewer becomes a chain, a half holding every id keeps the tree as it
// is, and any other half gets a new tree built in listing order.
func (s *Set) grow() {
	n := len(s.table)
	next := make([]bucket, 2*n)
	for i, bin := range s.table {
		if len(bin.ids) == 0 {
			continue
		}
		bit := func(id int32) bool { return foldedHash(id)&uint32(n) != 0 }
		var lo, hi []int32
		if bin.root != nil {
			for _, id := range bin.ids {
				if bit(id) {
					hi = append(hi, id)
				} else {
					lo = append(lo, id)
				}
			}
			next[i].root = splitTree(bin.root, lo, len(hi))
			next[i+n].root = splitTree(bin.root, hi, len(lo))
		} else {
			chain := bin.ids
			runBit, runStart := bit(chain[0]), 0
			for j := 1; j < len(chain); j++ {
				if b := bit(chain[j]); b != runBit {
					runBit, runStart = b, j
				}
			}
			for _, id := range chain[:runStart] {
				if bit(id) {
					hi = append([]int32{id}, hi...)
				} else {
					lo = append([]int32{id}, lo...)
				}
			}
			if runBit {
				hi = append(hi, chain[runStart:]...)
			} else {
				lo = append(lo, chain[runStart:]...)
			}
		}
		next[i].ids, next[i+n].ids = lo, hi
	}
	s.table = next
}

// splitTree is the tree of one half of a tree bucket split by grow: none
// for six ids or fewer, the bucket's own tree when the other half is empty,
// else a new one.
func splitTree(root *treeNode, ids []int32, others int) *treeNode {
	switch {
	case len(ids) <= setUntreeifyAt:
		return nil
	case others == 0:
		return root
	default:
		return buildTree(ids)
	}
}

// buildTree makes the tree of a bucket that becomes a tree, inserting its
// ids in listing order.
func buildTree(ids []int32) *treeNode {
	var root *treeNode
	for _, id := range ids {
		root = treeInsert(root, id)
	}
	return root
}

// shrinks reports whether a tree bucket losing an id is too small to stay a
// tree, judged by its shape before the id's node is taken out.
func shrinks(root *treeNode) bool {
	return root.kid[1] == nil || root.kid[0] == nil || root.kid[0].kid[0] == nil
}

// treeSide is the side of node a key belongs on in a tree: 0 (left) when id
// sorts before node's id, else 1.
func treeSide(node *treeNode, id int32) int {
	if h, nh := foldedHash(id), foldedHash(node.id); h != nh {
		return b2i(h > nh)
	}
	return b2i(id > node.id)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func isRed(n *treeNode) bool { return n != nil && n.red }

// side is which child of parent n is: 0 for the left one, else 1.
func side(parent, n *treeNode) int { return b2i(parent.kid[0] != n) }

// treeFind returns id's node, or nil.
func treeFind(root *treeNode, id int32) *treeNode {
	for p := root; p != nil; p = p.kid[treeSide(p, id)] {
		if p.id == id {
			return p
		}
	}
	return nil
}

// treeInsert adds id as a leaf and rebalances, returning the new root.
func treeInsert(root *treeNode, id int32) *treeNode {
	x := &treeNode{id: id}
	if root == nil {
		return x
	}
	p := root
	for {
		d := treeSide(p, id)
		if p.kid[d] == nil {
			p.kid[d], x.parent = x, p
			return balanceInsertion(root, x)
		}
		p = p.kid[d]
	}
}

// rotate turns p down to side d, raising its child from the other side,
// and returns the root.
func rotate(root, p *treeNode, d int) *treeNode {
	if p == nil || p.kid[1-d] == nil {
		return root
	}
	c := p.kid[1-d]
	if p.kid[1-d] = c.kid[d]; p.kid[1-d] != nil {
		p.kid[1-d].parent = p
	}
	pp := p.parent
	c.parent = pp
	if pp == nil {
		root = c
		c.red = false
	} else {
		pp.kid[side(pp, p)] = c
	}
	c.kid[d], p.parent = p, c
	return root
}

func balanceInsertion(root, x *treeNode) *treeNode {
	x.red = true
	for {
		xp := x.parent
		if xp == nil {
			x.red = false
			return x
		}
		xpp := xp.parent
		if !xp.red || xpp == nil {
			return root
		}
		d := side(xpp, xp)
		if uncle := xpp.kid[1-d]; isRed(uncle) {
			uncle.red, xp.red, xpp.red = false, false, true
			x = xpp
			continue
		}
		if x == xp.kid[1-d] {
			x = xp
			root = rotate(root, x, d)
			xp, xpp = x.parent, nil
			if xp != nil {
				xpp = xp.parent
			}
		}
		if xp != nil {
			xp.red = false
			if xpp != nil {
				xpp.red = true
				root = rotate(root, xpp, 1-d)
			}
		}
	}
}

// treeRemove takes node p out of the tree and returns the new root.
func treeRemove(root, p *treeNode) *treeNode {
	r := root
	var replacement *treeNode
	pl, pr := p.kid[0], p.kid[1]
	switch {
	case pl != nil && pr != nil:
		// swap p with its successor s, the leftmost node right of it
		s := pr
		for s.kid[0] != nil {
			s = s.kid[0]
		}
		s.red, p.red = p.red, s.red
		sr, pp := s.kid[1], p.parent
		if s == pr {
			p.parent = s
			s.kid[1] = p
		} else {
			sp := s.parent
			if p.parent = sp; sp != nil {
				sp.kid[side(sp, s)] = p
			}
			if s.kid[1] = pr; pr != nil {
				pr.parent = s
			}
		}
		p.kid[0] = nil
		if p.kid[1] = sr; sr != nil {
			sr.parent = p
		}
		if s.kid[0] = pl; pl != nil {
			pl.parent = s
		}
		if s.parent = pp; pp == nil {
			r = s
		} else {
			pp.kid[side(pp, p)] = s
		}
		replacement = p
		if sr != nil {
			replacement = sr
		}
	case pl != nil:
		replacement = pl
	case pr != nil:
		replacement = pr
	default:
		replacement = p
	}
	if replacement != p {
		pp := p.parent
		if replacement.parent = pp; pp == nil {
			r = replacement
		} else {
			pp.kid[side(pp, p)] = replacement
		}
		p.kid, p.parent = [2]*treeNode{}, nil
	}
	if !p.red {
		r = balanceDeletion(r, replacement)
	}
	if p == replacement {
		if pp := p.parent; pp != nil {
			if pp.kid[0] == p {
				pp.kid[0] = nil
			} else if pp.kid[1] == p {
				pp.kid[1] = nil
			}
			p.parent = nil
		}
	}
	return r
}

func balanceDeletion(root, x *treeNode) *treeNode {
	for {
		if x == nil || x == root {
			return root
		}
		xp := x.parent
		if xp == nil {
			x.red = false
			return x
		}
		if x.red {
			x.red = false
			return root
		}
		d := side(xp, x)
		sib := xp.kid[1-d]
		if isRed(sib) {
			sib.red, xp.red = false, true
			root = rotate(root, xp, d)
			if xp, sib = x.parent, nil; xp != nil {
				sib = xp.kid[1-d]
			}
		}
		if sib == nil {
			x = xp
			continue
		}
		near, far := sib.kid[d], sib.kid[1-d]
		if !isRed(far) && !isRed(near) {
			sib.red = true
			x = xp
			continue
		}
		if !isRed(far) {
			if near != nil {
				near.red = false
			}
			sib.red = true
			root = rotate(root, sib, 1-d)
			if xp, sib = x.parent, nil; xp != nil {
				sib = xp.kid[1-d]
			}
		}
		if sib != nil {
			sib.red = xp != nil && xp.red
			if far := sib.kid[1-d]; far != nil {
				far.red = false
			}
		}
		if xp != nil {
			xp.red = false
			root = rotate(root, xp, d)
		}
		x = root
	}
}
