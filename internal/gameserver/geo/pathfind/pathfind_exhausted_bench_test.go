package pathfind

import (
	"testing"
)

// BenchmarkFinderExhausted measures FindInto on real-terrain searches that
// run all MaxIterations and fail: the unreachable-target case that makes an
// escort stall a worker. It runs only where the shared datapack's geodata
// is present.
func BenchmarkFinderExhausted(b *testing.B) {
	e := loadGeodataSample(b)
	finder := New(e, DefaultOptions())
	_, searched := geodataCases(e, 2, 1000, 2000)
	var exhausted []pathCase
	for _, c := range searched {
		if exhaustsIterations(finder, c) {
			exhausted = append(exhausted, c)
		}
	}
	b.Logf("%d of %d searched cases exhaust MaxIterations bidirectionally", len(exhausted), len(searched))
	benchmarkCases(b, finder, "bidirectional", exhausted)

	forward := DefaultOptions()
	forward.Bidirectional = false
	forwardFinder := New(e, forward)
	exhausted = nil
	for _, c := range searched {
		if exhaustsIterations(forwardFinder, c) {
			exhausted = append(exhausted, c)
		}
	}
	b.Logf("%d of %d searched cases exhaust MaxIterations forward", len(exhausted), len(searched))
	benchmarkCases(b, forwardFinder, "forward", exhausted)
}

// exhaustsIterations reports whether c's search stops at MaxIterations with
// a frontier still open, rather than finding a path or emptying a frontier.
// It repeats find's endpoint setup so the search's own heaps can be read.
func exhaustsIterations(f *Finder, c pathCase) bool {
	if f.HasPath(c.origin, c.target) {
		return false
	}
	scratch := &searchScratch{}
	scratch.reset()
	start := scratch.newNodeFromWorld(c.origin.X, c.origin.Y, int(f.engine.Height(c.origin.X, c.origin.Y, c.origin.Z)))
	start.nswe = f.engine.NSWENearest(start.gx, start.gy, start.z)
	goal := scratch.newNodeFromWorld(c.target.X, c.target.Y, int(f.engine.Height(c.target.X, c.target.Y, c.target.Z)))
	goal.nswe = f.engine.NSWENearest(goal.gx, goal.gy, goal.z)
	if gap := goal.z - c.target.Z; gap > maxTargetHeightGap || -gap > maxTargetHeightGap || start.key() == goal.key() {
		return false
	}
	if f.options.Bidirectional {
		f.findBidirectional(nil, start, goal, false, scratch)
		return scratch.opened.Len() > 0 && scratch.backOpened.Len() > 0
	}
	f.findForward(nil, start, goal, false, scratch)
	return scratch.opened.Len() > 0
}
