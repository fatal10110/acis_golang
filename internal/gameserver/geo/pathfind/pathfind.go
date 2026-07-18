package pathfind

import (
	"container/heap"
	"math"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Finder computes geodata paths with A* over cell centers.
type Finder struct {
	engine  *engine.Engine
	options Options
	scratch sync.Pool
}

// New builds a Finder over one geodata engine.
func New(e *engine.Engine, options Options) *Finder {
	return &Finder{
		engine:  e,
		options: options,
		scratch: sync.Pool{New: func() any {
			return &searchScratch{}
		}},
	}
}

// Find returns a path from origin to target as corner points plus the final target cell,
// and the path's total weighted cost. The returned slice omits the origin. ok is false
// when no path was found within MaxIterations, in which case the other results are zero.
func (f *Finder) Find(origin, target location.Location) ([]location.Location, int, bool) {
	return f.FindInto(nil, origin, target)
}

// FindInto is Find with caller-owned result storage. dst is reset before use.
func (f *Finder) FindInto(dst []location.Location, origin, target location.Location) ([]location.Location, int, bool) {
	return f.find(dst, origin, target, true)
}

// HasPath reports whether origin can reach target without building a result path.
func (f *Finder) HasPath(origin, target location.Location) bool {
	_, _, ok := f.find(nil, origin, target, false)
	return ok
}

func (f *Finder) find(dst []location.Location, origin, target location.Location, buildResult bool) ([]location.Location, int, bool) {
	dst = dst[:0]
	if f == nil || f.engine == nil || engine.OutOfWorld(origin.X, origin.Y) || engine.OutOfWorld(target.X, target.Y) {
		return dst, 0, false
	}

	scratch, _ := f.scratch.Get().(*searchScratch)
	if scratch == nil {
		scratch = &searchScratch{}
	}
	scratch.reset()
	defer f.scratch.Put(scratch)

	start := scratch.newNodeFromWorld(origin.X, origin.Y, int(f.engine.Height(origin.X, origin.Y, origin.Z)))
	start.nswe = f.engine.NSWENearest(start.gx, start.gy, start.z)
	goal := scratch.newNodeFromWorld(target.X, target.Y, int(f.engine.Height(target.X, target.Y, target.Z)))
	goal.nswe = f.engine.NSWENearest(goal.gx, goal.gy, goal.z)

	if start.key() == goal.key() {
		return dst, 0, true
	}
	if f.options.Bidirectional {
		return f.findBidirectional(dst, start, goal, buildResult, scratch)
	}
	return f.findForward(dst, start, goal, buildResult, scratch)
}

func (f *Finder) findForward(dst []location.Location, start, goal *node, buildResult bool, scratch *searchScratch) ([]location.Location, int, bool) {
	start.h = f.heuristic(start, goal)
	start.f = start.h

	opened := &scratch.opened
	heap.Init(opened)
	heap.Push(opened, start)

	openSet := scratch.openSet
	openSet[start.key()] = start
	closed := scratch.closed
	seq := int64(1)
	iterations := 0

	for opened.Len() > 0 && iterations < f.options.MaxIterations {
		current := heap.Pop(opened).(*node)
		delete(openSet, current.key())

		if current.gx == goal.gx && current.gy == goal.gy && current.z == goal.z {
			if !buildResult {
				return dst, current.g, true
			}
			return buildPath(dst, current), current.g, true
		}

		closed[current.key()] = current
		f.expand(current, goal, &seq, scratch)

		iterations++
	}

	return dst, 0, false
}

func (f *Finder) findBidirectional(dst []location.Location, start, goal *node, buildResult bool, scratch *searchScratch) ([]location.Location, int, bool) {
	start.h = f.heuristic(start, goal)
	start.f = start.h
	start.seq = 1
	goal.h = f.heuristic(goal, start)
	goal.f = goal.h
	goal.seq = 2

	heap.Init(&scratch.opened)
	heap.Push(&scratch.opened, start)
	scratch.openSet[start.key()] = start
	heap.Init(&scratch.backOpened)
	heap.Push(&scratch.backOpened, goal)
	scratch.backOpenSet[goal.key()] = goal

	seq := int64(3)
	iterations := 0
	for scratch.opened.Len() > 0 && scratch.backOpened.Len() > 0 && iterations < f.options.MaxIterations {
		if scratch.opened[0].f <= scratch.backOpened[0].f {
			current := heap.Pop(&scratch.opened).(*node)
			delete(scratch.openSet, current.key())
			if meet := scratch.backClosed[current.key()]; meet != nil {
				if !buildResult {
					return dst, current.g + meet.g, true
				}
				return buildBidirectionalPath(dst, current, meet, scratch), current.g + meet.g, true
			}
			if meet := scratch.backOpenSet[current.key()]; meet != nil {
				if !buildResult {
					return dst, current.g + meet.g, true
				}
				return buildBidirectionalPath(dst, current, meet, scratch), current.g + meet.g, true
			}
			scratch.closed[current.key()] = current
			f.expandForward(current, goal, &seq, scratch, &scratch.opened, scratch.openSet, scratch.closed)
		} else {
			current := heap.Pop(&scratch.backOpened).(*node)
			delete(scratch.backOpenSet, current.key())
			if meet := scratch.closed[current.key()]; meet != nil {
				if !buildResult {
					return dst, current.g + meet.g, true
				}
				return buildBidirectionalPath(dst, meet, current, scratch), current.g + meet.g, true
			}
			if meet := scratch.openSet[current.key()]; meet != nil {
				if !buildResult {
					return dst, current.g + meet.g, true
				}
				return buildBidirectionalPath(dst, meet, current, scratch), current.g + meet.g, true
			}
			scratch.backClosed[current.key()] = current
			f.expandBackward(current, start, &seq, scratch)
		}
		iterations++
	}

	return dst, 0, false
}

type node struct {
	gx     int
	gy     int
	z      int
	nswe   block.NSWE
	g      int
	h      int
	f      int
	seq    int64
	parent *node
	index  int
}

func (n *node) key() nodeKey {
	return nodeKey{gx: n.gx, gy: n.gy, z: n.z}
}

type nodeKey struct {
	gx int
	gy int
	z  int
}

type nodeHeap []*node

func (h nodeHeap) Len() int { return len(h) }

func (h nodeHeap) Less(i, j int) bool {
	if h[i].f != h[j].f {
		return h[i].f < h[j].f
	}
	return h[i].seq < h[j].seq
}

func (h nodeHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *nodeHeap) Push(x any) {
	n := x.(*node)
	n.index = len(*h)
	*h = append(*h, n)
}

func (h *nodeHeap) Pop() any {
	old := *h
	n := old[len(old)-1]
	n.index = -1
	old[len(old)-1] = nil
	*h = old[:len(old)-1]
	return n
}

type searchScratch struct {
	opened      nodeHeap
	openSet     map[nodeKey]*node
	closed      map[nodeKey]*node
	backOpened  nodeHeap
	backOpenSet map[nodeKey]*node
	backClosed  map[nodeKey]*node
	nodes       []node
	pathNodes   []*node
}

func (s *searchScratch) reset() {
	clear(s.opened)
	s.opened = s.opened[:0]
	if s.openSet == nil {
		s.openSet = make(map[nodeKey]*node)
	} else {
		clear(s.openSet)
	}
	if s.closed == nil {
		s.closed = make(map[nodeKey]*node)
	} else {
		clear(s.closed)
	}
	clear(s.backOpened)
	s.backOpened = s.backOpened[:0]
	if s.backOpenSet == nil {
		s.backOpenSet = make(map[nodeKey]*node)
	} else {
		clear(s.backOpenSet)
	}
	if s.backClosed == nil {
		s.backClosed = make(map[nodeKey]*node)
	} else {
		clear(s.backClosed)
	}
	clear(s.nodes)
	s.nodes = s.nodes[:0]
	clear(s.pathNodes)
	s.pathNodes = s.pathNodes[:0]
}

func (s *searchScratch) newNodeFromWorld(worldX, worldY, z int) *node {
	return s.newNode(engine.GeoX(worldX), engine.GeoY(worldY), z)
}

func (s *searchScratch) newNode(gx, gy, z int) *node {
	s.nodes = append(s.nodes, node{
		gx:    gx,
		gy:    gy,
		z:     z,
		index: -1,
	})
	return &s.nodes[len(s.nodes)-1]
}

// cardinalSteps mirrors the reference PathFinder's four addDirectionalNode
// calls: N, S, W, E, in that order. Corner (diagonal) gating below indexes
// into this array by direction, matching addCornerNode's directionFlagX/Y
// parameters.
var cardinalSteps = [4]struct {
	dx, dy int
	flag   block.NSWE
}{
	{dx: 0, dy: -1, flag: block.North},
	{dx: 0, dy: 1, flag: block.South},
	{dx: -1, dy: 0, flag: block.West},
	{dx: 1, dy: 0, flag: block.East},
}

const maxSmoothCells = 32

const (
	dirN = iota
	dirS
	dirW
	dirE
)

// cornerSteps mirrors the reference PathFinder's four addCornerNode calls:
// NW, NE, SW, SE. xDir/yDir name which cardinalSteps entries supply the
// mutual-mask gate for this diagonal.
var cornerSteps = [4]struct {
	dx, dy     int
	xDir, yDir int
}{
	{dx: -1, dy: -1, xDir: dirW, yDir: dirN},
	{dx: 1, dy: -1, xDir: dirE, yDir: dirN},
	{dx: -1, dy: 1, xDir: dirW, yDir: dirS},
	{dx: 1, dy: 1, xDir: dirE, yDir: dirS},
}

// expand generates current's neighbor candidates from each cell's decoded
// NSWE mask, then addCandidate may smooth the parent link when a bounded
// direct movement check proves that shortcut is cheaper.
func (f *Finder) expand(current, goal *node, seq *int64, scratch *searchScratch) {
	f.expandForward(current, goal, seq, scratch, &scratch.opened, scratch.openSet, scratch.closed)
}

func (f *Finder) expandForward(current, goal *node, seq *int64, scratch *searchScratch, opened *nodeHeap, openSet, closed map[nodeKey]*node) {
	if current.nswe == block.NoDirections {
		return
	}

	z := current.z + block.CellIgnoreHeight

	var cardinalNSWE [4]block.NSWE
	for i, step := range cardinalSteps {
		if !current.nswe.Allows(step.flag) {
			continue
		}
		gx, gy := current.gx+step.dx, current.gy+step.dy
		height, nswe, ok := f.candidateNSWE(gx, gy, z)
		if !ok {
			continue
		}
		cardinalNSWE[i] = nswe
		f.addCandidateTo(current, goal, seq, scratch, opened, openSet, closed, gx, gy, height, nswe, false)
	}

	for _, corner := range cornerSteps {
		nsweX := cardinalNSWE[corner.xDir]
		nsweY := cardinalNSWE[corner.yDir]
		flagX := cardinalSteps[corner.xDir].flag
		flagY := cardinalSteps[corner.yDir].flag
		if !nsweX.Allows(flagY) || !nsweY.Allows(flagX) {
			continue
		}

		// Mirrors the reference's extra getNodeNswe(x+dx, y, z) corner
		// check: a fresh resolve of the X-direction neighbor (same cell and
		// z as nsweX above, kept as its own call for structural fidelity
		// with addCornerNode rather than reusing nsweX's value). Provably
		// value-identical to nsweX given static geodata — this line is only
		// reachable once nsweX itself came from a successful candidateNSWE
		// call at this exact cell/z — so it's a candidate for collapsing to
		// nsweX if profiling ever calls for it; the only behavioral
		// difference would surface under a door toggling this block mid-
		// search, which isn't a case the reference itself accounts for.
		xGX, xGY := current.gx+corner.dx, current.gy
		_, recheckNSWE, ok := f.candidateNSWE(xGX, xGY, z)
		if !ok || !recheckNSWE.Allows(flagY) {
			continue
		}

		gx, gy := current.gx+corner.dx, current.gy+corner.dy
		height, nswe, ok := f.candidateNSWE(gx, gy, z)
		if !ok {
			continue
		}
		f.addCandidateTo(current, goal, seq, scratch, opened, openSet, closed, gx, gy, height, nswe, true)
	}
}

func (f *Finder) expandBackward(current, goal *node, seq *int64, scratch *searchScratch) {
	for _, step := range cardinalSteps {
		gx, gy := current.gx-step.dx, current.gy-step.dy
		height, nswe, ok := f.backwardCandidateNSWE(gx, gy, current)
		if !ok || !nswe.Allows(step.flag) {
			continue
		}
		f.addBackwardCandidate(current, goal, seq, scratch, gx, gy, height, nswe, false)
	}

	for _, corner := range cornerSteps {
		flagX := cardinalSteps[corner.xDir].flag
		flagY := cardinalSteps[corner.yDir].flag
		gx, gy := current.gx-corner.dx, current.gy-corner.dy
		height, nswe, ok := f.backwardCandidateNSWE(gx, gy, current)
		if !ok || !nswe.Allows(flagX|flagY) {
			continue
		}

		candidateZ := height + block.CellIgnoreHeight
		_, nsweX, ok := f.candidateNSWE(gx+corner.dx, gy, candidateZ)
		if !ok || !nsweX.Allows(flagY) {
			continue
		}
		_, nsweY, ok := f.candidateNSWE(gx, gy+corner.dy, candidateZ)
		if !ok || !nsweY.Allows(flagX) {
			continue
		}
		f.addBackwardCandidate(current, goal, seq, scratch, gx, gy, height, nswe, true)
	}
}

// candidateNSWE resolves a candidate cell's own decoded height and NSWE
// mask, mirroring the reference's getIndexBelow/getHeight/getNswe sequence.
func (f *Finder) candidateNSWE(gx, gy, z int) (height int, nswe block.NSWE, ok bool) {
	worldX, worldY := engine.WorldX(gx), engine.WorldY(gy)
	if engine.OutOfWorld(worldX, worldY) {
		return 0, 0, false
	}
	h, m, found := f.engine.NodeBelow(gx, gy, z)
	if !found {
		return 0, 0, false
	}
	return int(h), m, true
}

func (f *Finder) candidateHeightMatches(gx, gy, z, wantHeight int) bool {
	height, _, ok := f.candidateNSWE(gx, gy, z)
	return ok && height == wantHeight
}

func (f *Finder) backwardCandidateNSWE(gx, gy int, current *node) (height int, nswe block.NSWE, ok bool) {
	worldX, worldY := engine.WorldX(gx), engine.WorldY(gy)
	if engine.OutOfWorld(worldX, worldY) {
		return 0, 0, false
	}

	z := current.z + block.CellIgnoreHeight
	height, nswe, ok = f.candidateNSWE(gx, gy, z)
	if ok && f.candidateHeightMatches(current.gx, current.gy, height+block.CellIgnoreHeight, current.z) {
		return height, nswe, true
	}

	h, n, found := f.engine.NodeAtOrAbove(gx, gy, z)
	if !found {
		return 0, 0, false
	}
	height = int(h)
	if !f.candidateHeightMatches(current.gx, current.gy, height+block.CellIgnoreHeight, current.z) {
		return 0, 0, false
	}
	return height, n, true
}

// addCandidate dedups against already explored/queued nodes, weights the grid
// step by the candidate's own NSWE mask, then keeps a parent-skip shortcut
// only when its straight-line cost is lower and the bounded direct movement
// check succeeds.
func (f *Finder) addCandidate(current, goal *node, seq *int64, scratch *searchScratch, gx, gy, height int, nswe block.NSWE, diagonal bool) {
	f.addCandidateTo(current, goal, seq, scratch, &scratch.opened, scratch.openSet, scratch.closed, gx, gy, height, nswe, diagonal)
}

func (f *Finder) addCandidateTo(current, goal *node, seq *int64, scratch *searchScratch, opened *nodeHeap, openSet, closed map[nodeKey]*node, gx, gy, height int, nswe block.NSWE, diagonal bool) {
	key := nodeKey{gx: gx, gy: gy, z: height}
	if _, ok := closed[key]; ok {
		return
	}
	if _, ok := openSet[key]; ok {
		return
	}

	weight := f.options.MoveWeight
	if diagonal {
		weight = f.options.MoveWeightDiag
	}
	if nswe != block.AllDirections {
		if diagonal {
			weight = f.options.obstacleWeightDiag()
		} else {
			weight = f.options.ObstacleWeight
		}
	}

	n := scratch.newNode(gx, gy, height)
	n.nswe = nswe
	parent := current
	cost := current.g + weight
	if current.parent != nil {
		smoothed := current.parent.g + f.straightLineCost(current.parent, gx, gy, height, nswe)
		if smoothed < cost &&
			withinSmoothRange(current.parent, gx, gy, height) &&
			f.canMoveDirect(current.parent, gx, gy, height) {
			parent = current.parent
			cost = smoothed
		}
	}
	n.g = cost
	n.parent = parent
	n.seq = *seq
	*seq = *seq + 1
	n.h = f.heuristic(n, goal)
	n.f = n.g + n.h
	heap.Push(opened, n)
	openSet[key] = n
}

func (f *Finder) addBackwardCandidate(current, goal *node, seq *int64, scratch *searchScratch, gx, gy, height int, nswe block.NSWE, diagonal bool) {
	key := nodeKey{gx: gx, gy: gy, z: height}
	if _, ok := scratch.backClosed[key]; ok {
		return
	}
	if _, ok := scratch.backOpenSet[key]; ok {
		return
	}

	weight := f.options.MoveWeight
	if diagonal {
		weight = f.options.MoveWeightDiag
	}
	if current.nswe != block.AllDirections {
		if diagonal {
			weight = f.options.obstacleWeightDiag()
		} else {
			weight = f.options.ObstacleWeight
		}
	}

	parent := current
	cost := current.g + weight
	if current.parent != nil {
		smoothed := current.parent.g + f.straightLineCostFrom(gx, gy, height, current.parent.gx, current.parent.gy, current.parent.z, current.parent.nswe)
		if smoothed < cost &&
			withinSmoothRangeFrom(gx, gy, height, current.parent.gx, current.parent.gy, current.parent.z) &&
			f.engine.CanMove(
				engine.WorldX(gx), engine.WorldY(gy), height,
				engine.WorldX(current.parent.gx), engine.WorldY(current.parent.gy), current.parent.z,
			) {
			parent = current.parent
			cost = smoothed
		}
	}

	n := scratch.newNode(gx, gy, height)
	n.nswe = nswe
	n.g = cost
	n.parent = parent
	n.seq = *seq
	*seq = *seq + 1
	n.h = f.heuristic(n, goal)
	n.f = n.g + n.h
	heap.Push(&scratch.backOpened, n)
	scratch.backOpenSet[key] = n
}

func (f *Finder) canMoveDirect(from *node, gx, gy, height int) bool {
	return f.engine.CanMove(
		engine.WorldX(from.gx), engine.WorldY(from.gy), from.z,
		engine.WorldX(gx), engine.WorldY(gy), height,
	)
}

func withinSmoothRange(from *node, gx, gy, height int) bool {
	return withinSmoothRangeFrom(from.gx, from.gy, from.z, gx, gy, height)
}

func withinSmoothRangeFrom(fromGX, fromGY, fromZ, toGX, toGY, toZ int) bool {
	dx := toGX - fromGX
	dy := toGY - fromGY
	dz := (toZ - fromZ) / block.CellHeight
	return location.In3DRange(0, 0, 0, dx, dy, dz, maxSmoothCells)
}

func (f *Finder) straightLineCost(from *node, gx, gy, height int, nswe block.NSWE) int {
	return f.straightLineCostFrom(from.gx, from.gy, from.z, gx, gy, height, nswe)
}

func (f *Finder) straightLineCostFrom(fromGX, fromGY, fromZ, toGX, toGY, toZ int, nswe block.NSWE) int {
	dx := toGX - fromGX
	dy := toGY - fromGY
	dz := (toZ - fromZ) / block.CellHeight
	weight := f.options.MoveWeight
	if nswe != block.AllDirections {
		weight = f.options.ObstacleWeight
	}
	return int(math.Sqrt(float64(dx*dx+dy*dy+dz*dz)) * float64(weight))
}

func (f *Finder) heuristic(from, to *node) int {
	dx := from.gx - to.gx
	if dx < 0 {
		dx = -dx
	}
	dy := from.gy - to.gy
	if dy < 0 {
		dy = -dy
	}
	dz := from.z - to.z
	if dz < 0 {
		dz = -dz
	}
	return int(math.Sqrt(float64(dx*dx+dy*dy+(dz/block.CellHeight)*(dz/block.CellHeight))) * float64(f.options.HeuristicWeight))
}

func buildPath(path []location.Location, goal *node) []location.Location {
	dx, dy := 0, 0

	for current, parent := goal, goal.parent; parent != nil; current, parent = parent, parent.parent {
		nx := parent.gx - current.gx
		ny := parent.gy - current.gy
		if dx != nx || dy != ny {
			path = append(path, location.Location{
				X: engine.WorldX(current.gx),
				Y: engine.WorldY(current.gy),
				Z: current.z,
			})
			dx = nx
			dy = ny
		}
	}

	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

func buildBidirectionalPath(path []location.Location, fromStart, fromGoal *node, scratch *searchScratch) []location.Location {
	nodes := scratch.pathNodes[:0]
	for n := fromStart; n != nil; n = n.parent {
		nodes = append(nodes, n)
	}
	for i, j := 0, len(nodes)-1; i < j; i, j = i+1, j-1 {
		nodes[i], nodes[j] = nodes[j], nodes[i]
	}
	for n := fromGoal.parent; n != nil; n = n.parent {
		nodes = append(nodes, n)
	}
	scratch.pathNodes = nodes
	return buildPathFromNodes(path, nodes)
}

func buildPathFromNodes(path []location.Location, nodes []*node) []location.Location {
	for i := 1; i < len(nodes); i++ {
		dx := nodes[i].gx - nodes[i-1].gx
		dy := nodes[i].gy - nodes[i-1].gy
		if i < len(nodes)-1 {
			nextDX := nodes[i+1].gx - nodes[i].gx
			nextDY := nodes[i+1].gy - nodes[i].gy
			if dx == nextDX && dy == nextDY {
				continue
			}
		}
		path = append(path, location.Location{
			X: engine.WorldX(nodes[i].gx),
			Y: engine.WorldY(nodes[i].gy),
			Z: nodes[i].z,
		})
	}
	return path
}
