package block

import "testing"

func TestNull(t *testing.T) {
	b := &Null{}

	if got := b.Kind(); got != KindNull {
		t.Errorf("Kind() = %v, want %v", got, KindNull)
	}
	if b.HasGeodata() {
		t.Errorf("HasGeodata() = true, want false")
	}
	if got := b.Layers(3, 4); got != 1 {
		t.Errorf("Layers(3,4) = %d, want 1", got)
	}

	// HeightNearest passes worldZ through unchanged, unlike every other
	// block kind, since there is no stored geodata to consult.
	for _, z := range []int32{0, 12345, -500} {
		if got := b.HeightNearest(0, 0, z); got != int16(z) {
			t.Errorf("HeightNearest(0,0,%d) = %d, want %d", z, got, int16(z))
		}
	}

	if got := b.NSWENearest(0, 0, 0); got != AllDirections {
		t.Errorf("NSWENearest = %v, want all", got)
	}
	if got := b.Nearest(0, 0, 0); got != 0 {
		t.Errorf("Nearest = %d, want 0", got)
	}
	if got := b.Above(0, 0, 0); got != 0 {
		t.Errorf("Above = %d, want 0", got)
	}
	if got := b.Below(0, 0, 0); got != 0 {
		t.Errorf("Below = %d, want 0", got)
	}
	if got := b.Height(0); got != 0 {
		t.Errorf("Height(0) = %d, want 0", got)
	}
	if got := b.NSWE(0); got != AllDirections {
		t.Errorf("NSWE(0) = %v, want all", got)
	}
}
