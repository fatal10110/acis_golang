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
	blockX := geoX / block.CellsX
	blockY := geoY / block.CellsY

	if p := e.dynamicBlocks.Load(); p != nil {
		if b := (*p)[blockKey{blockX, blockY}]; b != nil {
			return engineBlock{dyn: b}
		}
	}
	region := e.regions[regionX-TileXMin][regionY-TileYMin]
	if region == nil {
		return engineBlock{}
	}

	localGeoX := geoX % regionCellsX
	localGeoY := geoY % regionCellsY
	return engineBlock{region: region, blockX: localGeoX / block.CellsX, blockY: localGeoY / block.CellsY}
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
	if b.dyn != nil {
		return b.dyn.Above(cellX, cellY, worldZ)
	}
	return b.static().Above(cellX, cellY, worldZ)
}

func (b engineBlock) Below(cellX, cellY int, worldZ int32) int {
	if b.dyn != nil {
		return b.dyn.Below(cellX, cellY, worldZ)
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
