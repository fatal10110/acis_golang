package engine

import (
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
