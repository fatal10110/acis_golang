package xml

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

const zoneStatsBox = `<node x="0" y="0"/><node x="5" y="5"/>`

func loadZoneStatsFixture(t *testing.T, file, content string) *zone.Index {
	t.Helper()
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, file), content)
	index, err := LoadZones(dir)
	if err != nil {
		t.Fatalf("LoadZones(%s): %v", file, err)
	}
	return index
}

// A damage zone reads hpDamage, initialDelay and reuseDelay as written (the
// delays in milliseconds); hpDamage is matched case-sensitively and a stat
// that is not one of the kind's is ignored.
func TestLoadZonesDamageZoneStats(t *testing.T) {
	t.Parallel()
	index := loadZoneStatsFixture(t, "DamageZone.xml", `<list>
		<zone id="1" shape="Cuboid" minZ="0" maxZ="10">
			<stat name="hpDamage" val="40"/><stat name="initialDelay" val="7000"/><stat name="reuseDelay" val="9000"/><stat name="move_bonus" val="-30"/>`+zoneStatsBox+`
		</zone>
		<zone id="2" shape="Cuboid" minZ="0" maxZ="10">
			<stat name="HPDAMAGE" val="40"/>`+zoneStatsBox+`
		</zone>
	</list>`)
	zones := zone.OfKind[*zone.Damage](index)
	if len(zones) != 2 {
		t.Fatalf("damage zones = %d, want 2", len(zones))
	}
	byID := map[int]*zone.Damage{}
	for _, z := range zones {
		byID[z.ID()] = z
	}
	if z := byID[1]; z.HPDamage != 40 || z.InitialDelay != 7*time.Second || z.ReuseDelay != 9*time.Second {
		t.Errorf("zone 1 = hp %d, initial %v, reuse %v; want 40, 7s, 9s", z.HPDamage, z.InitialDelay, z.ReuseDelay)
	}
	if z := byID[2]; z.HPDamage != 200 {
		t.Errorf("zone 2 with a wrong-case hpDamage = %d, want the default 200", z.HPDamage)
	}
}

// A swamp zone reads move_bonus as written and matches its case.
func TestLoadZonesSwampZoneStats(t *testing.T) {
	t.Parallel()
	index := loadZoneStatsFixture(t, "SwampZone.xml", `<list>
		<zone id="1" shape="Cuboid" minZ="0" maxZ="10"><stat name="move_bonus" val="-30"/>`+zoneStatsBox+`</zone>
		<zone id="2" shape="Cuboid" minZ="0" maxZ="10"><stat name="MOVE_BONUS" val="-30"/>`+zoneStatsBox+`</zone>
	</list>`)
	byID := map[int]*zone.Swamp{}
	for _, z := range zone.OfKind[*zone.Swamp](index) {
		byID[z.ID()] = z
	}
	if len(byID) != 2 {
		t.Fatalf("swamp zones = %d, want 2", len(byID))
	}
	if got := byID[1].MoveBonus; got != -30 {
		t.Errorf("zone 1 MoveBonus = %d, want -30", got)
	}
	if got, want := byID[2].MoveBonus, zone.NewSwamp(0, nil).MoveBonus; got != want {
		t.Errorf("zone 2 with a wrong-case move_bonus = %d, want the default %d", got, want)
	}
}

// An effect zone reads chance, both delays, defaultStatus, the skill list and
// the target scope; the keys are case-sensitive.
func TestLoadZonesEffectZoneStats(t *testing.T) {
	t.Parallel()
	index := loadZoneStatsFixture(t, "EffectZone.xml", `<list>
		<zone id="1" shape="Cuboid" minZ="0" maxZ="10">
			<stat name="chance" val="35"/><stat name="initialDelay" val="2000"/><stat name="reuseDelay" val="4000"/>
			<stat name="defaultStatus" val="false"/><stat name="skill" val="4070-1;4071-2"/><stat name="targetType" val="Playable"/>`+zoneStatsBox+`
		</zone>
		<zone id="2" shape="Cuboid" minZ="0" maxZ="10">
			<stat name="Chance" val="35"/><stat name="REUSEDELAY" val="4000"/>`+zoneStatsBox+`
		</zone>
	</list>`)
	byID := map[int]*zone.Effect{}
	for _, z := range zone.OfKind[*zone.Effect](index) {
		byID[z.ID()] = z
	}
	if len(byID) != 2 {
		t.Fatalf("effect zones = %d, want 2", len(byID))
	}
	z := byID[1]
	if z.Chance != 35 || z.InitialDelay != 2*time.Second || z.ReuseDelay != 4*time.Second || z.Enabled() || z.Target != zone.ScopePlayable {
		t.Errorf("zone 1 = chance %d, initial %v, reuse %v, enabled %v, target %v", z.Chance, z.InitialDelay, z.ReuseDelay, z.Enabled(), z.Target)
	}
	if want := []zone.SkillRef{{ID: 4070, Level: 1}, {ID: 4071, Level: 2}}; !slices.Equal(z.Skills, want) {
		t.Errorf("zone 1 skills = %v, want %v", z.Skills, want)
	}
	def := zone.NewEffect(0, nil)
	if z := byID[2]; z.Chance != def.Chance || z.ReuseDelay != def.ReuseDelay {
		t.Errorf("zone 2 with wrong-case keys = chance %d, reuse %v; want the defaults %d, %v", z.Chance, z.ReuseDelay, def.Chance, def.ReuseDelay)
	}
}
