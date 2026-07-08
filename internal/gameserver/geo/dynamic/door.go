package dynamic

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

const (
	tileXMin  = 16
	tileYMin  = 10
	tileSize  = 32768
	worldXMin = (tileXMin - 20) * tileSize
	worldYMin = (tileYMin - 18) * tileSize
)

// Sampler provides the static geodata lookups door shaping needs.
type Sampler interface {
	HeightNearest(geoX, geoY, worldZ int) int16
	Above(geoX, geoY, worldZ int) (int16, bool)
}

// NewDoorObject converts one static door template into a toggleable
// geodata object by scanning a grid of sample points around the door's
// polygon footprint and marking which cells fall inside it.
func NewDoorObject(tmpl *door.Template, sampler Sampler) (Object, error) {
	if tmpl == nil {
		return nil, fmt.Errorf("geo/dynamic: nil door template")
	}
	if sampler == nil {
		return nil, fmt.Errorf("geo/dynamic: nil sampler")
	}

	minX, maxX, minY, maxY := bounds(tmpl.Coordinates)
	x := geoX(minX) - 1
	y := geoY(minY) - 1
	sizeX := (geoX(maxX) + 1) - x + 1
	sizeY := (geoY(maxY) + 1) - y + 1
	originX := geoX(tmpl.Position.X)
	originY := geoY(tmpl.Position.Y)
	originZ := int(sampler.HeightNearest(originX, originY, tmpl.Position.Z))
	height := tmpl.Height
	if above, ok := sampler.Above(originX, originY, originZ); ok {
		layerDiff := int(above) - originZ
		if height > layerDiff {
			height = layerDiff - block.CellIgnoreHeight
		}
	}

	limit := block.CellIgnoreHeight
	if tmpl.Kind == door.KindWall {
		limit *= 4
	}
	inside := make([][]bool, sizeX)
	for ix := range inside {
		inside[ix] = make([]bool, sizeY)
		for iy := range inside[ix] {
			gx := x + ix
			gy := y + iy
			z := int(sampler.HeightNearest(gx, gy, tmpl.Position.Z))
			if absInt(z-tmpl.Position.Z) > limit {
				continue
			}
			wx := worldX(gx)
			wy := worldY(gy)
		cell:
			for sx := wx - 6; sx <= wx+6; sx += 2 {
				for sy := wy - 6; sy <= wy+6; sy += 2 {
					if pointInPolygon(tmpl.Coordinates, location.Point{X: sx, Y: sy}) {
						inside[ix][iy] = true
						break cell
					}
				}
			}
		}
	}

	return &object{
		geoX:   x,
		geoY:   y,
		geoZ:   originZ,
		height: height,
		data:   CalculateGeoObject(inside),
	}, nil
}

func bounds(points []location.Point) (minX, maxX, minY, maxY int) {
	minX, minY = points[0].X, points[0].Y
	maxX, maxY = minX, minY
	for _, p := range points[1:] {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	return
}

func pointInPolygon(poly []location.Point, p location.Point) bool {
	inside := false
	j := len(poly) - 1
	for i := 0; i < len(poly); i++ {
		yi := poly[i].Y
		yj := poly[j].Y
		if (yi > p.Y) != (yj > p.Y) {
			xi := poly[i].X
			xj := poly[j].X
			cross := float64(xj-xi)*float64(p.Y-yi)/float64(yj-yi) + float64(xi)
			if float64(p.X) < cross {
				inside = !inside
			}
		}
		j = i
	}
	return inside
}

func geoX(worldX int) int { return (worldX - worldXMin) >> 4 }
func geoY(worldY int) int { return (worldY - worldYMin) >> 4 }
func worldX(geoX int) int { return (geoX << 4) + worldXMin + 8 }
func worldY(geoY int) int { return (geoY << 4) + worldYMin + 8 }
func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
