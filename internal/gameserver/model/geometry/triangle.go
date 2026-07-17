package geometry

// Triangle is a 2D triangle, a Polygon specialized to exactly three
// vertices.
type Triangle struct {
	vertexList
}

// NewTriangle builds a Triangle from its three vertices.
func NewTriangle(a, b, c Point) (Triangle, error) {
	vl, err := newVertexList([]Point{a, b, c})
	if err != nil {
		return Triangle{}, err
	}
	return Triangle{vertexList: vl}, nil
}
