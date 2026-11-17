package geometry

import "testing"

// The ear test's determinant is an int32 product: for a 50,000-unit right
// triangle it wraps negative, so even a point just inside is not inside.
func TestEarPointInTriangleWrapsInt32Products(t *testing.T) {
	if !earPointInTriangle(0, 0, 1000, 0, 0, 1000, 10, 10) {
		t.Fatal("small triangle: interior point not inside")
	}
	if earPointInTriangle(0, 0, 50000, 0, 0, 50000, 10, 10) {
		t.Fatal("wide triangle: interior point inside, want the wrapped int32 determinant to reject it")
	}
}
