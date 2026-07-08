package xml

import (
	"path/filepath"
	"testing"
)

func TestLoadManors(t *testing.T) {
	path := datapackPath(t, filepath.Join("data", "xml", "manors.xml"))

	table, err := LoadManors(path)
	if err != nil {
		t.Fatalf("LoadManors(%q) error: %v", path, err)
	}

	if got, want := len(table.Manors), 9; got != want {
		t.Fatalf("len(Manors) = %d, want %d", got, want)
	}
	if got, want := len(table.SeedsByID), 246; got != want {
		t.Fatalf("len(SeedsByID) = %d, want %d", got, want)
	}

	first := table.Manors[0]
	if first.ID != 1 || first.Name != "gludio" {
		t.Fatalf("first manor = %+v", first)
	}
	seed, ok := table.SeedsByID[5016]
	if !ok {
		t.Fatal("seed 5016 not loaded")
	}
	if seed.CropID != 5073 || seed.MatureID != 5103 || seed.Level != 10 || seed.Reward1 != 1864 || seed.Reward2 != 1878 {
		t.Fatalf("seed 5016 = %+v", seed)
	}
	if seed.CastleID != 1 || seed.Alternative || seed.SeedsLimit != 8100 || seed.CropsLimit != 9000 {
		t.Fatalf("seed 5016 manor fields = %+v", seed)
	}
}

func TestLoadManorAreas(t *testing.T) {
	path := datapackPath(t, filepath.Join("data", "xml", "manorAreas.xml"))

	areas, err := LoadManorAreas(path)
	if err != nil {
		t.Fatalf("LoadManorAreas(%q) error: %v", path, err)
	}

	if got, want := len(areas), 121; got != want {
		t.Fatalf("len(areas) = %d, want %d", got, want)
	}
	first := areas[0]
	if first.Name != "aden_2010_001" || first.CastleID != 5 || first.MinZ != -13296 || first.MaxZ != -3296 {
		t.Fatalf("first manor area = %+v", first)
	}
	if len(first.Nodes) != 4 || first.Nodes[0].X != 8251 || first.Nodes[0].Y != -249650 {
		t.Fatalf("first manor area nodes = %+v", first.Nodes)
	}
}

func TestLoadManorsErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manors.xml")
	writeXMLFixture(t, path, `<list><manor id="1" name="x"><crop id="1" seedId="2"/></manor></list>`)

	if _, err := LoadManors(path); err == nil {
		t.Fatal("LoadManors() error = nil, want error")
	}
}
