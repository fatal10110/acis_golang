package relation

// The client is sent a friend or block list in the order of the hash set the
// list is gathered into, the ids added in the order the relation store walks
// its pairs (see pairTable). idSet reproduces that set: a table of 16
// buckets, doubled once it holds more than three quarters of its size, and
// doubled too, while under 64 buckets, when a ninth id lands in one bucket.
// From 64 buckets on, a bucket that gets a ninth id becomes a red-black tree
// keyed by hash, and the order of its ids follows the tree (see putTree).
const (
	setInitialBuckets  = 16
	treeifyThreshold   = 8 // entries a bucket holds before the next one treeifies it
	untreeifyThreshold = 6 // a tree split this small turns back into a list
	minTreeifyBuckets  = 64
)

// clientOrder puts ids, given in the order the relation store walks them, in
// the order the client is sent them. It reuses ids' backing array.
func clientOrder(ids []int32) []int32 {
	if len(ids) < 2 {
		return ids
	}
	var s idSet
	for _, id := range ids {
		s.add(id)
	}
	out := ids[:0]
	for _, head := range s.buckets {
		for e := head; e != nil; e = e.next {
			out = append(out, e.id)
		}
	}
	return out
}

// idNode is one id of an idSet. next and prev chain a bucket in iteration
// order; parent, left, right and red are its place in a treeified bucket.
type idNode struct {
	id, hash                        int32
	next, prev, parent, left, right *idNode
	red, inTree                     bool
}

// idSet is the hash set a list is gathered into. Its ids are distinct (each
// is the other side of a distinct pair), so add never meets one it holds.
type idSet struct {
	buckets   []*idNode
	size      int
	threshold int
}

// idHash folds an id onto its own high half; distinct ids keep distinct
// hashes, so a tree never holds two equal keys.
func idHash(id int32) int32 {
	h := uint32(id)
	return int32(h ^ h>>16)
}

func bucketOf(hash int32, buckets int) int {
	return int(uint32(hash) & uint32(buckets-1))
}

func (s *idSet) add(id int32) {
	if s.buckets == nil {
		s.buckets = make([]*idNode, setInitialBuckets)
		s.threshold = setInitialBuckets * 3 / 4
	}
	x := &idNode{id: id, hash: idHash(id)}
	i := bucketOf(x.hash, len(s.buckets))
	switch p := s.buckets[i]; {
	case p == nil:
		s.buckets[i] = x
	case p.inTree:
		s.putTree(p, x)
	default:
		held := 1
		for ; p.next != nil; p = p.next {
			held++
		}
		p.next = x
		if held >= treeifyThreshold {
			s.treeifyBucket(i)
		}
	}
	s.size++
	if s.size > s.threshold {
		s.resize()
	}
}

// resize doubles the table. Each bucket splits in iteration order between
// its own index and the one a table size higher; see place for a tree.
func (s *idSet) resize() {
	old := s.buckets
	n := len(old)
	s.buckets = make([]*idNode, 2*n)
	s.threshold *= 2
	for j, e := range old {
		var loHead, loTail, hiHead, hiTail *idNode
		lc, hc := 0, 0
		for e != nil {
			next := e.next
			e.next = nil
			if uint32(e.hash)&uint32(n) == 0 {
				if e.prev = loTail; loTail == nil {
					loHead = e
				} else {
					loTail.next = e
				}
				loTail = e
				lc++
			} else {
				if e.prev = hiTail; hiTail == nil {
					hiHead = e
				} else {
					hiTail.next = e
				}
				hiTail = e
				hc++
			}
			e = next
		}
		s.place(j, loHead, lc, hiHead != nil)
		s.place(j+n, hiHead, hc, loHead != nil)
	}
}

// place puts one half of a split bucket at index i. A half of a tree bucket
// stays a tree only above untreeifyThreshold ids; it is rebuilt when the
// bucket was split between both halves and keeps its tree otherwise.
func (s *idSet) place(i int, head *idNode, count int, split bool) {
	if head == nil {
		return
	}
	s.buckets[i] = head
	if !head.inTree {
		return
	}
	if count <= untreeifyThreshold {
		for e := head; e != nil; e = e.next {
			e.prev, e.parent, e.left, e.right, e.red, e.inTree = nil, nil, nil, nil, false, false
		}
		return
	}
	if split {
		s.treeify(head)
	}
}

// treeifyBucket turns bucket i into a tree, or doubles the table instead
// while it has fewer than minTreeifyBuckets buckets.
func (s *idSet) treeifyBucket(i int) {
	if len(s.buckets) < minTreeifyBuckets {
		s.resize()
		return
	}
	var prev *idNode
	for e := s.buckets[i]; e != nil; e = e.next {
		e.inTree, e.prev = true, prev
		prev = e
	}
	s.treeify(s.buckets[i])
}

// treeify builds the tree of the bucket chained from head, inserting its ids
// in chain order, then moves the root to the front of the chain.
func (s *idSet) treeify(head *idNode) {
	var root *idNode
	for x := head; x != nil; x = x.next {
		x.left, x.right = nil, nil
		if root == nil {
			x.parent, x.red = nil, false
			root = x
			continue
		}
		xp := treeParent(root, x.hash)
		x.parent = xp
		if xp.hash > x.hash {
			xp.left = x
		} else {
			xp.right = x
		}
		root = balanceInsertion(root, x)
	}
	s.moveRootToFront(root)
}

// putTree adds x to the tree bucket whose chain starts at root (every change
// to a tree moves its root to the front): x is chained right after its tree
// parent, and the rebalanced root is moved to the front of the chain.
func (s *idSet) putTree(root, x *idNode) {
	xp := treeParent(root, x.hash)
	if xp.hash > x.hash {
		xp.left = x
	} else {
		xp.right = x
	}
	x.inTree = true
	x.next = xp.next
	xp.next = x
	x.parent, x.prev = xp, xp
	if x.next != nil {
		x.next.prev = x
	}
	s.moveRootToFront(balanceInsertion(root, x))
}

// treeParent is the node a new hash hangs under: the last one a search for
// it from root passes.
func treeParent(root *idNode, hash int32) *idNode {
	for p := root; ; {
		next := p.right
		if p.hash > hash {
			next = p.left
		}
		if next == nil {
			return p
		}
		p = next
	}
}

func (s *idSet) moveRootToFront(root *idNode) {
	i := bucketOf(root.hash, len(s.buckets))
	first := s.buckets[i]
	if root == first {
		return
	}
	s.buckets[i] = root
	if root.next != nil {
		root.next.prev = root.prev
	}
	if root.prev != nil {
		root.prev.next = root.next
	}
	if first != nil {
		first.prev = root
	}
	root.next, root.prev = first, nil
}

// rotateLeft and rotateRight are called only on a node with the child they
// rotate up.
func rotateLeft(root, p *idNode) *idNode {
	r := p.right
	if p.right = r.left; p.right != nil {
		p.right.parent = p
	}
	pp := p.parent
	r.parent = pp
	switch {
	case pp == nil:
		root = r
		r.red = false
	case pp.left == p:
		pp.left = r
	default:
		pp.right = r
	}
	r.left = p
	p.parent = r
	return root
}

func rotateRight(root, p *idNode) *idNode {
	l := p.left
	if p.left = l.right; p.left != nil {
		p.left.parent = p
	}
	pp := p.parent
	l.parent = pp
	switch {
	case pp == nil:
		root = l
		l.red = false
	case pp.right == p:
		pp.right = l
	default:
		pp.left = l
	}
	l.right = p
	p.parent = l
	return root
}

// balanceInsertion restores the red-black invariants after x was inserted
// and returns the root.
func balanceInsertion(root, x *idNode) *idNode {
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
		left := xp == xpp.left
		uncle := xpp.left
		if left {
			uncle = xpp.right
		}
		if uncle != nil && uncle.red {
			uncle.red, xp.red, xpp.red = false, false, true
			x = xpp
			continue
		}
		if inner := (left && x == xp.right) || (!left && x == xp.left); inner {
			x = xp
			if left {
				root = rotateLeft(root, x)
			} else {
				root = rotateRight(root, x)
			}
			xp, xpp = x.parent, nil
			if xp != nil {
				xpp = xp.parent
			}
		}
		if xp != nil {
			xp.red = false
			if xpp != nil {
				xpp.red = true
				if left {
					root = rotateRight(root, xpp)
				} else {
					root = rotateLeft(root, xpp)
				}
			}
		}
	}
}
