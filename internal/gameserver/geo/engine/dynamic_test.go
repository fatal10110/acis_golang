package engine

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

type dynamicStub struct {
	x, y, z int
	height  int
	data    [][]block.NSWE
}

func (d dynamicStub) GeoX() int               { return d.x }
func (d dynamicStub) GeoY() int               { return d.y }
func (d dynamicStub) GeoZ() int               { return d.z }
func (d dynamicStub) Height() int             { return d.height }
func (d dynamicStub) GeoData() [][]block.NSWE { return d.data }

func TestEngineDynamicObjectBlocksAndRestoresMovement(t *testing.T) {
	e := New()
	region, err := block.NewRegionFromBlocks([]block.Block{block.NewFlat(0)})
	if err != nil {
		t.Fatalf("NewRegionFromBlocks: %v", err)
	}
	if err := e.SetRegion(TileXMin, TileYMin, region); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}

	originX, originY := WorldX(0), WorldY(0)
	targetX, targetY := WorldX(1), WorldY(0)
	if !e.CanMove(originX, originY, 0, targetX, targetY, 0) {
		t.Fatal("flat geodata CanMove() = false before adding a dynamic object")
	}

	obj := &dynamicStub{
		x:      0,
		y:      0,
		z:      0,
		height: 32,
		data:   [][]block.NSWE{{block.NoDirections}},
	}
	e.AddObject(obj)

	if e.CanMove(originX, originY, 0, targetX, targetY, 0) {
		t.Fatal("CanMove() = true through a closed dynamic object")
	}

	e.RemoveObject(obj)

	if !e.CanMove(originX, originY, 0, targetX, targetY, 0) {
		t.Fatal("CanMove() = false after removing the dynamic object")
	}
}

func TestEngineEvictsDynamicBlockAfterLastObjectRemove(t *testing.T) {
	e := New()
	region, err := block.NewRegionFromBlocks([]block.Block{block.NewFlat(0)})
	if err != nil {
		t.Fatalf("NewRegionFromBlocks: %v", err)
	}
	if err := e.SetRegion(TileXMin, TileYMin, region); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}

	obj := &dynamicStub{
		x:      0,
		y:      0,
		z:      0,
		height: 32,
		data:   [][]block.NSWE{{block.NoDirections}},
	}

	e.AddObject(obj)
	if got := dynamicBlockCount(e); got != 1 {
		t.Fatalf("dynamic block count after AddObject = %d, want 1", got)
	}

	e.RemoveObject(obj)

	if got := dynamicBlockCount(e); got != 0 {
		t.Fatalf("dynamic block count after RemoveObject = %d, want 0", got)
	}
}

// TestEngineConcurrentDoorToggleAndQueries covers #513's correctness
// requirement: swapping dynamicBlocks to a lock-free atomic-pointer read path
// must stay race-safe while a door concurrently opens/closes (toggleObject's
// clone-and-swap on first creation, and repeated in-place Add/Remove on an
// already-created block).
//
// The querying goroutine targets the same block as the toggled door so a
// query can hold a dynamic layer handle across a concurrent Add/Remove. That
// covers both the atomic pointer swap around dynamicBlocks and the dynamic
// block's own stale-handle safety.
func TestEngineConcurrentDoorToggleAndQueries(t *testing.T) {
	e := New()
	region, err := block.NewRegionFromBlocks([]block.Block{block.NewFlat(0)})
	if err != nil {
		t.Fatalf("NewRegionFromBlocks: %v", err)
	}
	if err := e.SetRegion(TileXMin, TileYMin, region); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}

	doorOriginX, doorOriginY := WorldX(0), WorldY(0)
	doorTargetX, doorTargetY := WorldX(1), WorldY(0)
	obj := &dynamicStub{
		x:      0,
		y:      0,
		z:      0,
		height: 32,
		data:   [][]block.NSWE{{block.NoDirections}},
	}
	// other shares obj's block so the two toggling goroutines race on the
	// same clone-and-swap insertion the first time either creates the
	// block, then race on in-place Add/Remove against the same
	// *dynamic.Block afterward.
	other := &dynamicStub{
		x:      0,
		y:      0,
		z:      0,
		height: 32,
		data:   [][]block.NSWE{{block.NoDirections}},
	}

	queryOriginX, queryOriginY := doorOriginX, doorOriginY
	queryTargetX, queryTargetY := doorTargetX, doorTargetY

	const iterations = 500
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			e.AddObject(obj)
			e.RemoveObject(obj)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			e.AddObject(other)
			e.RemoveObject(other)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = e.CanMove(queryOriginX, queryOriginY, 0, queryTargetX, queryTargetY, 0)
			_ = e.CanSee(queryOriginX, queryOriginY, 0, queryTargetX, queryTargetY, 0)
			_ = e.Height(queryOriginX, queryOriginY, 0)
		}
	}()
	wg.Wait()

	if !e.CanMove(doorOriginX, doorOriginY, 0, doorTargetX, doorTargetY, 0) {
		t.Fatal("CanMove() = false after every toggling goroutine finished on Remove, want the door left open")
	}
}

func dynamicBlockCount(e *Engine) int {
	current := e.dynamicBlocks.Load()
	if current == nil {
		return 0
	}
	return len(*current)
}
