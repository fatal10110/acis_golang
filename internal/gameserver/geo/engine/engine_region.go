package engine

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

func (e *Engine) blockAtBlock(blockX, blockY int) regionBlock {
	if blockX < 0 || blockY < 0 {
		return regionBlock{}
	}
	regionX := TileXMin + blockX/block.RegionBlocksX
	regionY := TileYMin + blockY/block.RegionBlocksY
	if regionX < TileXMin || regionX > TileXMax || regionY < TileYMin || regionY > TileYMax {
		return regionBlock{}
	}
	region := e.regions[regionX-TileXMin][regionY-TileYMin]
	if region == nil {
		return regionBlock{}
	}
	return regionBlock{
		region: region,
		blockX: blockX % block.RegionBlocksX,
		blockY: blockY % block.RegionBlocksY,
	}
}

type regionBlock struct {
	region *block.Region
	blockX int
	blockY int
}

func (b regionBlock) Kind() block.Kind {
	if b.region == nil {
		return block.KindNull
	}
	return b.region.KindAt(b.blockX, b.blockY)
}

func (b regionBlock) HasGeodata() bool {
	return b.region != nil && b.region.HasGeodata(b.blockX, b.blockY)
}

func (b regionBlock) Layers(cellX, cellY int) int {
	if b.region == nil {
		return 1
	}
	return b.region.Layers(b.blockX, b.blockY, cellX, cellY)
}

func (b regionBlock) HeightNearest(cellX, cellY int, worldZ int32) int16 {
	if b.region == nil {
		return block.NullHeight(worldZ)
	}
	return b.region.HeightNearest(b.blockX, b.blockY, cellX, cellY, worldZ)
}

func (b regionBlock) NSWENearest(cellX, cellY int, worldZ int32) block.NSWE {
	if b.region == nil {
		return block.AllDirections
	}
	return b.region.NSWENearest(b.blockX, b.blockY, cellX, cellY, worldZ)
}

func (b regionBlock) Nearest(cellX, cellY int, worldZ int32) int {
	if b.region == nil {
		return 0
	}
	return b.region.Nearest(b.blockX, b.blockY, cellX, cellY, worldZ)
}

func (b regionBlock) Above(cellX, cellY int, worldZ int32) int {
	if b.region == nil {
		return 0
	}
	return b.region.Above(b.blockX, b.blockY, cellX, cellY, worldZ)
}

func (b regionBlock) Below(cellX, cellY int, worldZ int32) int {
	if b.region == nil {
		return 0
	}
	return b.region.Below(b.blockX, b.blockY, cellX, cellY, worldZ)
}

func (b regionBlock) Height(layer int) int16 {
	if b.region == nil {
		return 0
	}
	return b.region.Height(b.blockX, b.blockY, layer)
}

func (b regionBlock) NSWE(layer int) block.NSWE {
	if b.region == nil {
		return block.AllDirections
	}
	return b.region.NSWE(b.blockX, b.blockY, layer)
}

func (b regionBlock) Cells(cellX, cellY int) []block.Cell {
	if b.region == nil {
		return []block.Cell{{Height: 0, NSWE: block.AllDirections}}
	}
	return b.region.Cells(b.blockX, b.blockY, cellX, cellY)
}
