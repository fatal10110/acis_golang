package main

import (
	"testing"

	"github.com/rs/zerolog"
)

// TestDataReloadsReadDoors requires the door reload hook to read the
// datapack's doors.xml anew, and to fail on a file it cannot read.
func TestDataReloadsReadDoors(t *testing.T) {
	root := t.TempDir()
	writeDataFile(t, root, "", "doors.xml", `<?xml version="1.0" encoding="utf-8"?>
<list>
	<door id="19210001" type="DOOR" level="1" name="gludio_castle_outter_001" opened="true">
		<position x="-18408" y="113064" z="-2768"/>
		<coordinates>
			<loc x="-18481" y="113059" z="-2799"/>
			<loc x="-18351" y="113059" z="-2799"/>
			<loc x="-18351" y="113071" z="-2799"/>
			<loc x="-18481" y="113071" z="-2799"/>
		</coordinates>
		<stats hp="253200" pDef="644" mDef="518" height="320"/>
	</door>
</list>`)
	reloads := provideDataReloads(gameServerPaths{DataRoot: root}, gameServerConfig{}, &gameData{}, nil, nil, nil, nil, gameplayConfig{}, zerolog.Nop())

	doors, err := reloads.Doors()
	if err != nil {
		t.Fatalf("door reload: %v", err)
	}
	gate, ok := doors.Get(19210001)
	if doors.Len() != 1 || !ok || gate.Name != "gludio_castle_outter_001" || !gate.Opened || gate.HP != 253200 {
		t.Fatalf("reloaded doors = %d, door 19210001 = %+v; want the one opened gate", doors.Len(), gate)
	}

	writeDataFile(t, root, "", "doors.xml", "<list><door")
	if _, err := reloads.Doors(); err == nil {
		t.Fatal("door reload of a broken file succeeded")
	}
}
