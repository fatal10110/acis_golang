package xml

import (
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
)

// TestShippedManorAreasIndex triangulates every shipped manor area, as
// ManorAreaData does at load, and finds a monster's area by its spawn point
// with both z bounds inclusive (ManorArea.isInside).
func TestShippedManorAreasIndex(t *testing.T) {
	t.Parallel()
	areas, err := LoadManorAreas(datapackPath(t, filepath.Join("data", "xml", "manorAreas.xml")))
	if err != nil {
		t.Fatalf("LoadManorAreas: %v", err)
	}
	index, skipped := manor.NewAreaIndex(areas)
	if len(skipped) != 0 || index.Len() != 121 {
		t.Fatalf("indexed %d areas, skipped %v; want all 121", index.Len(), skipped)
	}
	// gludio_1621_001: castle 1, z -5036..964, x -121528..-98340.
	for _, tc := range []struct {
		name    string
		x, y, z int
		want    string
	}{
		{"inside", -110000, 110000, -3000, "gludio_1621_001"},
		{"min z", -110000, 110000, -5036, "gludio_1621_001"},
		{"max z", -110000, 110000, 964, "gludio_1621_001"},
		{"below", -110000, 110000, -5037, ""},
		{"above", -110000, 110000, 965, ""},
		{"no area", 10, 20, 30, ""},
	} {
		area, ok := index.AreaAt(tc.x, tc.y, tc.z)
		if ok != (tc.want != "") || area.Name != tc.want {
			t.Fatalf("%s: AreaAt(%d, %d, %d) = %q, %v; want %q", tc.name, tc.x, tc.y, tc.z, area.Name, ok, tc.want)
		}
		if ok && area.CastleID != 1 {
			t.Fatalf("%s: castle %d, want 1", tc.name, area.CastleID)
		}
	}
}
