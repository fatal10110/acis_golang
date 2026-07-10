package manager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

func TestNewSpawnsCreatesMissingStateRows(t *testing.T) {
	dir := t.TempDir()
	writeSpawnFixture(t, filepath.Join(dir, "20_20.xml"), `
<list>
	<territory name="field" minZ="-10" maxZ="10">
		<node x="0" y="0"/>
		<node x="100" y="0"/>
		<node x="100" y="100"/>
		<node x="0" y="100"/>
	</territory>
	<npcmaker name="maker" territory="field" maximumNpcs="2">
		<npc id="1" total="1" dbName="existing"/>
		<npc id="2" total="1" dbName="missing"/>
		<npc id="3" total="1"/>
	</npcmaker>
</list>`)

	table, err := xml.LoadSpawnlist(dir)
	if err != nil {
		t.Fatalf("LoadSpawnlist() unexpected error: %v", err)
	}

	existing := &spawn.State{Name: "existing", Status: spawn.StatusAlive, CurrentHP: 10}
	spawns := NewSpawns(table, map[string]*spawn.State{"existing": existing})

	if got, ok := spawns.State("existing"); !ok || got != existing {
		t.Fatalf("State(existing) = %p, %v; want original row", got, ok)
	}
	missing, ok := spawns.State("missing")
	if !ok {
		t.Fatal("State(missing) = missing")
	}
	if missing.Status != spawn.StatusUninitialized {
		t.Fatalf("missing status = %d, want %d", missing.Status, spawn.StatusUninitialized)
	}
	if got, ok := spawns.State(""); ok || got != nil {
		t.Fatalf("State(empty) = %p, %v; want nil, false", got, ok)
	}
	if got, want := spawns.StateCount(), 2; got != want {
		t.Fatalf("StateCount() = %d, want %d", got, want)
	}
}

func writeSpawnFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(`<?xml version="1.0" encoding="utf-8"?>`+body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
