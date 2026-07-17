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
	goal := scratch.newNodeFromWorld(target.X, target.Y, int(f.engine.Height(target.X, target.Y, target.Z)))
	start.h = f.heuristic(start, goal)
	start.f = start.h

	opened := &scratch.opened
	heap.Init(opened)
	heap.Push(opened, start)

	openSet := scratch.openSet
	openSet[start.key()] = struct{}{}
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

		closed[current.key()] = struct{}{}
		f.expand(current, goal, &seq, scratch)

		iterations++
	}

	return dst, 0, false
}

type node struct {
	gx     int
	gy     int
	z      int
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
	opened  nodeHeap
	openSet map[nodeKey]struct{}
	closed  map[nodeKey]struct{}
	nodes   []node
}

func (s *searchScratch) reset() {
	clear(s.opened)
	s.opened = s.opened[:0]
	if s.openSet == nil {
		s.openSet = make(map[nodeKey]struct{})
	} else {
		clear(s.openSet)
	}
	if s.closed == nil {
		s.closed = make(map[nodeKey]struct{})
	} else {
		clear(s.closed)
	}
	clear(s.nodes)
	s.nodes = s.nodes[:0]
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

var steps = [...]struct {
	dx       int
	dy       int
	diagonal bool
}{
	{dx: 0, dy: -1},
	{dx: 0, dy: 1},
	{dx: -1, dy: 0},
	{dx: 1, dy: 0},
	{dx: -1, dy: -1, diagonal: true},
	{dx: 1, dy: -1, diagonal: true},
	{dx: -1, dy: 1, diagonal: true},
	{dx: 1, dy: 1, diagonal: true},
}

func (f *Finder) expand(current, goal *node, seq *int64, scratch *searchScratch) {
	for _, step := range steps {
		if step.diagonal && !f.canMoveDiagonal(current, step.dx, step.dy) {
			continue
		}

		worldX := engine.WorldX(current.gx + step.dx)
		worldY := engine.WorldY(current.gy + step.dy)
		if engine.OutOfWorld(worldX, worldY) {
			continue
		}

		nextZ := int(f.engine.Height(worldX, worldY, current.z+block.CellIgnoreHeight))
		if !f.engine.CanMove(engine.WorldX(current.gx), engine.WorldY(current.gy), current.z, worldX, worldY, nextZ) {
			continue
		}

		key := nodeKey{gx: current.gx + step.dx, gy: current.gy + step.dy, z: nextZ}
		if _, ok := scratch.closed[key]; ok {
			continue
		}
		if _, ok := scratch.openSet[key]; ok {
			continue
		}

		weight := f.options.MoveWeight
		if step.diagonal {
			weight = f.options.MoveWeightDiag
		}
		if !f.cellOpen(worldX, worldY, nextZ) {
			if step.diagonal {
				weight = f.options.obstacleWeightDiag()
			} else {
				weight = f.options.ObstacleWeight
			}
		}

		n := scratch.newNode(key.gx, key.gy, key.z)
		n.g = current.g + weight
		n.parent = current
		n.seq = *seq
		*seq = *seq + 1
		n.h = f.heuristic(n, goal)
		n.f = n.g + n.h
		heap.Push(&scratch.opened, n)
		scratch.openSet[key] = struct{}{}
	}
}

func (f *Finder) canMoveDiagonal(current *node, dx, dy int) bool {
	sideX := engine.WorldX(current.gx + dx)
	sideY := engine.WorldY(current.gy)
	if engine.OutOfWorld(sideX, sideY) {
		return false
	}
	sideZX := int(f.engine.Height(sideX, sideY, current.z+block.CellIgnoreHeight))
	if !f.engine.CanMove(engine.WorldX(current.gx), engine.WorldY(current.gy), current.z, sideX, sideY, sideZX) {
		return false
	}

	sideX = engine.WorldX(current.gx)
	sideY = engine.WorldY(current.gy + dy)
	if engine.OutOfWorld(sideX, sideY) {
		return false
	}
	sideZY := int(f.engine.Height(sideX, sideY, current.z+block.CellIgnoreHeight))
	return f.engine.CanMove(engine.WorldX(current.gx), engine.WorldY(current.gy), current.z, sideX, sideY, sideZY)
}

func (f *Finder) cellOpen(worldX, worldY, worldZ int) bool {
	for _, step := range steps[:4] {
		nextX := engine.WorldX(engine.GeoX(worldX) + step.dx)
		nextY := engine.WorldY(engine.GeoY(worldY) + step.dy)
		if engine.OutOfWorld(nextX, nextY) {
			return false
		}

		nextZ := int(f.engine.Height(nextX, nextY, worldZ+block.CellIgnoreHeight))
		if !f.engine.CanMove(worldX, worldY, worldZ, nextX, nextY, nextZ) {
			return false
		}
	}
	return true
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
