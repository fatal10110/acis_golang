package probe

import (
	"path/filepath"
	"testing"
)

func TestResolveDir(t *testing.T) {
	root := filepath.Join("srv", "acis")
	abs, err := filepath.Abs(filepath.Join("geo", "files"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ configured, want string }{
		{"", filepath.Join(root, "data", "geodata")},
		{"  ", filepath.Join(root, "data", "geodata")},
		{"data/geodata/", filepath.Join(root, "data", "geodata")},
		{"data", filepath.Join(root, "data")},
		{"./data/geo", filepath.Join(root, "data", "geo")},
		{"database/geo", filepath.Join("database", "geo")},
		{"geo", "geo"},
		{abs, abs},
	} {
		if got := ResolveDir(root, tc.configured); got != tc.want {
			t.Errorf("ResolveDir(%q, %q) = %q, want %q", root, tc.configured, got, tc.want)
		}
	}
}
