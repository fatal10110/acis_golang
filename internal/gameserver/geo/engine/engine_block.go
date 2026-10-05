package engine

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
)

func (e *Engine) heightNearest(geoX, geoY, worldZ int) int16 {
	return e.blockAtGeo(geoX, geoY).HeightNearest(localCell(geoX), localCell(geoY), int32(worldZ))
}

func (e *Engine) nsweNearest(geoX, geoY, worldZ int) block.NSWE {
	return e.blockAtGeo(geoX, geoY).NSWENearest(localCell(geoX), localCell(geoY), int32(worldZ))
}

func (e *Engine) blockAtGeo(geoX, geoY int) engineBlock {
	regionX := TileXMin + geoX/regionCellsX
	regionY := TileYMin + geoY/regionCellsY
	if geoX < 0 || geoY < 0 || regionX < TileXMin || regionX > TileXMax || regionY < TileYMin || regionY > TileYMax {
		return engineBlock{}
	}
	tileX := regionX - TileXMin
	tileY := regionY - TileYMin
	blockX := (geoX % regionCellsX) / block.CellsX
	blockY := (geoY % regionCellsY) / block.CellsY

	// dynamicMask gates dynamicBlocks: a block can only carry an overlay if
	// its bit is set, so the overwhelmingly common no-overlay cell resolves
	// with an atomic load and a bit test instead of a map hash.
	if mask := e.dynamicMask[tileX][tileY].Load(); mask != nil && mask.has(blockX, blockY) {
		if p := e.dynamicBlocks.Load(); p != nil {
			if b := (*p)[blockKey{geoX / block.CellsX, geoY / block.CellsY}]; b != nil {
				return engineBlock{dyn: b}
			}
		}
	}
	region := e.regions[tileX][tileY]
	if region == nil {
		return engineBlock{}
	}
	return engineBlock{region: region, blockX: blockX, blockY: blockY}
}

type engineBlock struct {
	dyn    *dynamic.Block
	region *block.Region
	blockX int
	blockY int
}

func (b engineBlock) static() regionBlock {
	return regionBlock{region: b.region, blockX: b.blockX, blockY: b.blockY}
}

func (b engineBlock) HasGeodata() bool {
	if b.dyn != nil {
		return b.dyn.HasGeodata()
	}
	return b.static().HasGeodata()
}

func (b engineBlock) HeightNearest(cellX, cellY int, worldZ int32) int16 {
	if b.dyn != nil {
		return b.dyn.HeightNearest(cellX, cellY, worldZ)
	}
	return b.static().HeightNearest(cellX, cellY, worldZ)
}

func (b engineBlock) NSWENearest(cellX, cellY int, worldZ int32) block.NSWE {
	if b.dyn != nil {
		return b.dyn.NSWENearest(cellX, cellY, worldZ)
	}
	return b.static().NSWENearest(cellX, cellY, worldZ)
}

func (b engineBlock) Above(cellX, cellY int, worldZ int32) int {
	return b.AboveIgnoring(cellX, cellY, worldZ, nil)
}

func (b engineBlock) AboveIgnoring(cellX, cellY int, worldZ int32, ignore dynamic.Object) int {
	if b.dyn != nil {
		return b.dyn.AboveIgnoring(cellX, cellY, worldZ, ignore)
	}
	return b.static().Above(cellX, cellY, worldZ)
}

func (b engineBlock) Below(cellX, cellY int, worldZ int32) int {
	return b.BelowIgnoring(cellX, cellY, worldZ, nil)
}

func (b engineBlock) BelowIgnoring(cellX, cellY int, worldZ int32, ignore dynamic.Object) int {
	if b.dyn != nil {
		return b.dyn.BelowIgnoring(cellX, cellY, worldZ, ignore)
	}
	return b.static().Below(cellX, cellY, worldZ)
}

func (b engineBlock) Height(layer int) int16 {
	if b.dyn != nil {
		return b.dyn.Height(layer)
	}
	return b.static().Height(layer)
}

func (b engineBlock) NSWE(layer int) block.NSWE {
	if b.dyn != nil {
		return b.dyn.NSWE(layer)
	}
	return b.static().NSWE(layer)
}

func (b engineBlock) Cells(cellX, cellY int) []block.Cell {
	if b.dyn != nil {
		return b.dyn.Cells(cellX, cellY)
	}
	return b.static().Cells(cellX, cellY)
}

// AppendCells appends the cell's layers, in Cells order, to dst without
// allocating when dst has room for block.MaxLayers. A dynamic block's active
// override is copied under its one read lock; an untouched cell of a dynamic
// block reads straight from the static block it wraps.
func (b engineBlock) AppendCells(dst []block.Cell, cellX, cellY int) []block.Cell {
	if b.dyn == nil {
		return b.static().AppendCells(dst, cellX, cellY)
	}
	if cells, ok := b.dyn.AppendActiveCells(dst, cellX, cellY); ok {
		return cells
	}
	if base, ok := b.dyn.Base().(regionBlock); ok {
		return base.AppendCells(dst, cellX, cellY)
	}
	return append(dst, b.dyn.Cells(cellX, cellY)...)
}
