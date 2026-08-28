package xml

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/rs/zerolog"
)

func TestLoadDoors(t *testing.T) {
	path := datapackPath(t, filepath.Join("data", "xml", "doors.xml"))

	table, err := LoadDoors(path, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadDoors(%q) error: %v", path, err)
	}

	if got, want := table.Len(), 547; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}

	first, ok := table.Get(19210001)
	if !ok {
		t.Fatal("door 19210001 not loaded")
	}
	if first.Name != "gludio_castle_outter_001" || first.Kind != door.KindDoor || first.Level != 1 {
		t.Fatalf("door 19210001 identity = %+v", first)
	}
	if first.Position.X != -18408 || first.Position.Y != 113064 || first.Position.Z != -2768 {
		t.Fatalf("door 19210001 position = %+v", first.Position)
	}
	if len(first.Coordinates) != 4 || first.Coordinates[0].X != -18481 || first.Coordinates[0].Y != 113059 {
		t.Fatalf("door 19210001 coords = %+v", first.Coordinates)
	}
	if first.HP != 253200 || first.PDef != 644 || first.MDef != 518 || first.Height != 320 {
		t.Fatalf("door 19210001 stats = hp=%d pDef=%d mDef=%d height=%d", first.HP, first.PDef, first.MDef, first.Height)
	}
	if first.OpenKind != door.OpenNPC || first.Opened {
		t.Fatalf("door 19210001 function = openKind=%s opened=%v", first.OpenKind, first.Opened)
	}

	opened, ok := table.Get(24180019)
	if !ok {
		t.Fatal("door 24180019 not loaded")
	}
	if !opened.Opened {
		t.Fatalf("door 24180019 Opened = false, want true")
	}
}

func TestLoadDoorsErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doors.xml")
	writeXMLFixture(t, path, `<list><door id="1" type="DOOR" level="1" name="broken"><position x="1" y="2" z="3"/><coordinates><loc x="1" y="2"/></coordinates><stats hp="1" pDef="1" mDef="1" height="1"/></door></list>`)

	if _, err := LoadDoors(path, zerolog.Nop()); err == nil {
		t.Fatal("LoadDoors() error = nil, want error")
	}
}

func TestLoadDoorsSkipsOutOfWorldCoordinates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doors.xml")
	writeXMLFixture(t, path, `<list>
		<door id="1" type="DOOR" level="1" name="valid"><position x="1" y="2" z="3"/><coordinates><loc x="1" y="2"/><loc x="1" y="3"/><loc x="2" y="3"/></coordinates><stats hp="1" pDef="1" mDef="1" height="1"/></door>
		<door id="2" type="DOOR" level="1" name="out"><position x="1" y="2" z="3"/><coordinates><loc x="`+itoa(engine.WorldXMin-1)+`" y="2"/><loc x="1" y="3"/><loc x="2" y="3"/></coordinates><stats hp="1" pDef="1" mDef="1" height="1"/></door>
	</list>`)

	var logs bytes.Buffer
	table, err := LoadDoors(path, zerolog.New(&logs))
	if err != nil {
		t.Fatalf("LoadDoors() error: %v", err)
	}
	if table.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", table.Len())
	}
	if _, ok := table.Get(1); !ok {
		t.Fatal("valid door not loaded")
	}
	if _, ok := table.Get(2); ok {
		t.Fatal("out-of-world door loaded")
	}
	if !strings.Contains(logs.String(), "out-of-world door") {
		t.Fatalf("log = %q, want out-of-world door diagnostic", logs.String())
	}
}
