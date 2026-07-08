package dynamic

import "github.com/fatal10110/acis_golang/internal/gameserver/geo/block"

// Object is one dynamic geodata shape positioned in geodata cell space.
type Object interface {
	GeoX() int
	GeoY() int
	GeoZ() int
	Height() int
	GeoData() [][]block.NSWE
}

type object struct {
	geoX   int
	geoY   int
	geoZ   int
	height int
	data   [][]block.NSWE
}

func (o *object) GeoX() int               { return o.geoX }
func (o *object) GeoY() int               { return o.geoY }
func (o *object) GeoZ() int               { return o.geoZ }
func (o *object) Height() int             { return o.height }
func (o *object) GeoData() [][]block.NSWE { return o.data }

// CalculateGeoObject converts an object's inside/outside cell mask into
// geodata NSWE edits matching GeoEngine.calculateGeoObject.
func CalculateGeoObject(inside [][]bool) [][]block.NSWE {
	width := len(inside)
	if width == 0 {
		return nil
	}
	height := len(inside[0])
	out := make([][]block.NSWE, width)
	for x := 0; x < width; x++ {
		out[x] = make([]block.NSWE, height)
		for y := 0; y < height; y++ {
			if inside[x][y] {
				out[x][y] = block.NoDirections
				continue
			}
			nswe := block.AllDirections
			if y < height-1 && inside[x][y+1] {
				nswe &^= block.South
			}
			if y > 0 && inside[x][y-1] {
				nswe &^= block.North
			}
			if x < width-1 && inside[x+1][y] {
				nswe &^= block.East
			}
			if x > 0 && inside[x-1][y] {
				nswe &^= block.West
			}
			out[x][y] = nswe
		}
	}
	return out
}
