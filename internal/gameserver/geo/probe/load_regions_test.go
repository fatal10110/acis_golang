package probe

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/rs/zerolog"
)

// TestListedRegions pins the reference GeoEngine region selection
// (props.containsKey(rx + "_" + ry) over the tile range): a bare or valued
// "X_Y" key enables its tile, a "#X_Y" comment line and an out-of-range key
// do not, and unrelated keys are ignored.
func TestListedRegions(t *testing.T) {
	props, err := config.ParseString(`
GeoDataPath = ./data/geodata/
GeoDataType = L2OFF
16_10
16_11 = yes
#16_13 - not supported by L2 client
17_25
10_10
26_25
26_26
016_12
`)
	if err != nil {
		t.Fatal(err)
	}

	got := ListedRegions(props)
	want := []Region{{X: 16, Y: 10}, {X: 16, Y: 11}, {X: 17, Y: 25}, {X: 26, Y: 25}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListedRegions() = %v, want %v", got, want)
	}

	if got := ListedRegions(nil); got != nil {
		t.Errorf("ListedRegions(nil) = %v, want nil", got)
	}
}

func TestLoadRegions(t *testing.T) {
	listedX, listedY := engine.TileXMin, engine.TileYMin
	listed := []Region{{X: listedX, Y: listedY}}
	inListed := func() (int, int) { return engine.WorldX(0), engine.WorldY(0) }

	t.Run("loads every listed region and logs the count", func(t *testing.T) {
		dir := t.TempDir()
		writeFlatL2OFF(t, dir, listedX, listedY, 80)

		var logs bytes.Buffer
		e, err := LoadRegions(dir, L2OFF, listed, zerolog.New(&logs))
		if err != nil {
			t.Fatalf("LoadRegions(): %v", err)
		}
		x, y := inListed()
		if got := e.Height(x, y, 0); got != 80 {
			t.Errorf("Height() in listed region = %d, want 80", got)
		}
		if out := logs.String(); !strings.Contains(out, `"region_files":1`) || !strings.Contains(out, `"geodata_type":"L2OFF"`) {
			t.Errorf("log = %s, want region_files 1 for L2OFF", out)
		}
	})

	t.Run("an unlisted region file on disk stays null", func(t *testing.T) {
		dir := t.TempDir()
		writeFlatL2OFF(t, dir, listedX, listedY, 80)

		e, err := LoadRegions(dir, L2OFF, nil, zerolog.Nop())
		if err != nil {
			t.Fatalf("LoadRegions(): %v", err)
		}
		x, y := inListed()
		if got := e.Height(x, y, 4321); got != 4321 {
			t.Errorf("Height() in unlisted region = %d, want unchanged worldZ 4321", got)
		}
	})

	t.Run("loads a listed L2J region", func(t *testing.T) {
		dir := t.TempDir()
		writeFlatL2J(t, dir, listedX, listedY, -40)

		e, err := LoadRegions(dir, L2J, listed, zerolog.Nop())
		if err != nil {
			t.Fatalf("LoadRegions(): %v", err)
		}
		x, y := inListed()
		if got := e.Height(x, y, 0); got != -40 {
			t.Errorf("Height() = %d, want -40", got)
		}
	})

	t.Run("a missing listed region fails naming every failed file", func(t *testing.T) {
		dir := t.TempDir()
		writeFlatL2OFF(t, dir, listedX, listedY, 80)
		regions := append([]Region{}, listed...)
		regions = append(regions, Region{X: 17, Y: 25}, Region{X: 22, Y: 16})

		var logs bytes.Buffer
		_, err := LoadRegions(dir, L2OFF, regions, zerolog.New(&logs))
		if err == nil {
			t.Fatal("LoadRegions() error = nil, want error for missing listed regions")
		}
		msg := err.Error()
		for _, want := range []string{
			"failed to load 2 L2OFF region files",
			filepath.Join(dir, "17_25_conv.dat"),
			filepath.Join(dir, "22_16_conv.dat"),
			"GeoDataPath " + dir,
			"GeoDataType L2OFF",
		} {
			if !strings.Contains(msg, want) {
				t.Errorf("error %q does not contain %q", msg, want)
			}
		}
		if strings.Contains(msg, filepath.Join(dir, "16_10_conv.dat")) {
			t.Errorf("error %q names the region that loaded", msg)
		}
		if out := logs.String(); !strings.Contains(out, `"region_files":1`) {
			t.Errorf("log = %s, want the loaded count reported before failing", out)
		}
	})

	t.Run("a malformed listed region fails", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, fileNameFor(L2OFF, listedX, listedY))
		if err := os.WriteFile(path, []byte("too short"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := LoadRegions(dir, L2OFF, listed, zerolog.Nop())
		if err == nil || !strings.Contains(err.Error(), path) {
			t.Fatalf("LoadRegions() error = %v, want failure naming %s", err, path)
		}
	})

	t.Run("an unreadable listed region fails", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root reads files regardless of mode")
		}
		dir := t.TempDir()
		writeFlatL2OFF(t, dir, listedX, listedY, 80)
		path := filepath.Join(dir, fileNameFor(L2OFF, listedX, listedY))
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
		_, err := LoadRegions(dir, L2OFF, listed, zerolog.Nop())
		if err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("LoadRegions() error = %v, want permission failure", err)
		}
	})

	t.Run("unknown geo type errors", func(t *testing.T) {
		if _, err := LoadRegions(t.TempDir(), "bogus", listed, zerolog.Nop()); err == nil {
			t.Fatal("LoadRegions() error = nil, want error")
		}
	})
}
