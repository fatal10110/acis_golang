package main

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/rs/zerolog"
)

// TestDataReloadsReadZonesWithTheTownCombatRule requires the zone reload
// hook to read the datapack's zones directory into a fresh index, the
// configured town combat rule set on its towns as at boot, and to fail on a
// file it cannot read.
func TestDataReloadsReadZonesWithTheTownCombatRule(t *testing.T) {
	root := t.TempDir()
	writeDataFile(t, root, "zones", "TownZone.xml", `<?xml version="1.0" encoding="UTF-8"?>
<list>
	<zone shape="Cuboid" minZ="-3800" maxZ="-3300">
		<stat name="townId" val="9"/>
		<node x="77170" y="147420"/>
		<node x="79200" y="144780"/>
	</zone>
</list>`)
	reloads := provideDataReloads(gameServerPaths{DataRoot: root}, gameServerConfig{TownCombatRule: 2}, &gameData{}, nil, nil, nil, nil, gameplayConfig{}, zerolog.Nop())

	zones, err := reloads.Zones()
	if err != nil {
		t.Fatalf("zone reload: %v", err)
	}
	towns := zone.OfKind[*zone.Town](zones)
	if len(towns) != 1 || towns[0].TownID != 9 || towns[0].CombatRule != 2 {
		t.Fatalf("reloaded towns = %+v, want town 9 under combat rule 2", towns)
	}

	writeDataFile(t, root, "zones", "TownZone.xml", "<list><zone")
	if _, err := reloads.Zones(); err == nil {
		t.Fatal("zone reload of a broken file succeeded")
	}
}
