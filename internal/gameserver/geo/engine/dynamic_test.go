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

// TestEngineConcurrentDoorToggleAndQueries covers #513's correctness
// requirement: swapping dynamicBlocks to a lock-free atomic-pointer read path
// must stay race-safe while a door concurrently opens/closes (toggleObject's
// clone-and-swap on first creation, and repeated in-place Add/Remove on an
// already-created block).
//
// The querying goroutine deliberately targets a different block than the
// door: reusing the same block would also exercise a pre-existing hazard in
// dynamic.Block itself (Below/Above hands back a layer index that Height/NSWE
// re-decodes later, and a concurrent Add/Remove's rebuild can invalidate that
// index in between — reproducible against dynamicBlocks's *old* RWMutex-based
// code too, so it's not something this change introduced or is scoped to
// fix; tracked separately). This test's job is only #513's own claim: the
// atomic pointer swap and the map it publishes are safe to read concurrently
// with a writer, and door state itself stays correct under concurrent
// toggles.
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

	// Far outside block (0,0): null geodata, untouched by obj/other, so the
	// query goroutine only exercises blockAtGeo's atomic read of
	// dynamicBlocks concurrently with the toggling goroutines' writes.
	queryOriginX, queryOriginY := WorldX(800), WorldY(800)
	queryTargetX, queryTargetY := WorldX(801), WorldY(800)

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
