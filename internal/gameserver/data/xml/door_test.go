package xml

import (
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
)

func TestLoadDoors(t *testing.T) {
	path := datapackPath(t, filepath.Join("data", "xml", "doors.xml"))

	table, err := LoadDoors(path)
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

	if _, err := LoadDoors(path); err == nil {
		t.Fatal("LoadDoors() error = nil, want error")
	}
}
