package xml

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

// loadOneZone loads a single cuboid zone of the kind file names, carrying
// stats as its <stat name val> children.
func loadOneZone(t *testing.T, file string, stats map[string]string) (zone.Kind, error) {
	t.Helper()
	var b strings.Builder
	for name, val := range stats {
		fmt.Fprintf(&b, `<stat name=%q val=%q/>`, name, val)
	}
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, file),
		`<list><zone shape="Cuboid" minZ="0" maxZ="10">`+b.String()+`<node x="0" y="0"/><node x="5" y="5"/></zone></list>`)
	index, err := LoadZones(dir)
	if err != nil {
		return nil, err
	}
	all := index.All()
	if len(all) != 1 {
		t.Fatalf("LoadZones(%s) = %d zones, want 1", file, len(all))
	}
	return all[0], nil
}

// zoneSettingKeys lists every <stat> a settings-taking zone kind reads as a
// plain decimal int.
var zoneSettingKeys = map[string][]string{
	"BossZone":           {"InvadeTime", "oustX", "oustY", "oustZ"},
	"CastleTeleportZone": {"castleId", "spawnMinX", "spawnMaxX", "spawnMinY", "spawnMaxY", "spawnZ"},
	"CastleZone":         {"castleId"},
	"ClanHallZone":       {"clanHallId"},
	"DamageZone":         {"castleId", "eventId", "hpDamage", "initialDelay", "reuseDelay"},
	"EffectZone":         {"chance", "initialDelay", "reuseDelay"},
	"MotherTreeZone":     {"enterMsgId", "leaveMsgId", "MpRegenBonus", "HpRegenBonus", "affectedRace"},
	"SiegeZone":          {"castleId", "clanHallId"},
	"SwampZone":          {"castleId", "eventId", "move_bonus"},
	"TownZone":           {"townId", "castleId"},
}

// TestZoneSettingsRejectMalformedInts: a present int setting that is empty,
// space-padded or non-numeric fails the load instead of reading as the
// default, while a kind that takes no settings ignores its stats.
func TestZoneSettingsRejectMalformedInts(t *testing.T) {
	t.Parallel()
	for file, keys := range zoneSettingKeys {
		for _, key := range keys {
			for _, bad := range []string{"", " 1", "1 ", "x", "1.5"} {
				if _, err := loadOneZone(t, file+".xml", map[string]string{key: bad}); err == nil {
					t.Errorf("%s %s=%q loaded, want an error", file, key, bad)
				}
			}
		}
	}
	if _, err := loadOneZone(t, "PeaceZone.xml", map[string]string{"castleId": "x"}); err != nil {
		t.Errorf("PeaceZone with a malformed unused stat: %v", err)
	}
	for _, stats := range []map[string]string{{"targetType": "Monster"}, {"skill": "1-x"}, {"skill": "1-1;"}} {
		if _, err := loadOneZone(t, "EffectZone.xml", stats); err == nil {
			t.Errorf("EffectZone %v loaded, want an error", stats)
		}
	}
}

// TestZoneSettingsDefaults pins the value each settings-taking kind takes
// when its zone carries no <stat> children.
func TestZoneSettingsDefaults(t *testing.T) {
	t.Parallel()
	load := func(file string) zone.Kind {
		t.Helper()
		k, err := loadOneZone(t, file, nil)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		return k
	}

	if z := load("MotherTreeZone.xml").(*zone.MotherTree); z.EnterMessage != 0 || z.LeaveMessage != 0 || z.HPBonus != 1 || z.MPBonus != 1 || z.AffectedRace != -1 {
		t.Errorf("mother tree defaults: messages %d/%d bonus hp %d mp %d race %d", z.EnterMessage, z.LeaveMessage, z.HPBonus, z.MPBonus, z.AffectedRace)
	}
	if z := load("TownZone.xml").(*zone.Town); z.TownID != 0 || z.CastleID != 0 || !z.Peaceful {
		t.Errorf("town defaults: id %d castle %d peaceful %v", z.TownID, z.CastleID, z.Peaceful)
	}
	if z := load("SiegeZone.xml").(*zone.Siege); z.ResidenceID != -1 {
		t.Errorf("siege default residence = %d, want -1", z.ResidenceID)
	}
	if z := load("CastleZone.xml").(*zone.Castle); z.ResidenceID != 0 {
		t.Errorf("castle default residence = %d, want 0", z.ResidenceID)
	}
	if z := load("ClanHallZone.xml").(*zone.ClanHall); z.ResidenceID != 0 {
		t.Errorf("clan hall default residence = %d, want 0", z.ResidenceID)
	}
	if z := load("CastleTeleportZone.xml").(*zone.CastleTeleport); z.CastleID != 0 || z.ExitMinX != 0 || z.ExitMaxX != 0 || z.ExitMinY != 0 || z.ExitMaxY != 0 || z.ExitZ != 0 {
		t.Errorf("castle teleport defaults: castle %d exit x %d..%d y %d..%d z %d", z.CastleID, z.ExitMinX, z.ExitMaxX, z.ExitMinY, z.ExitMaxY, z.ExitZ)
	}
	if z := load("DamageZone.xml").(*zone.Damage); z.CastleID != 0 || z.EventID != 0 || z.HPDamage != 200 || z.InitialDelay != time.Second || z.ReuseDelay != 5*time.Second {
		t.Errorf("damage defaults: castle %d event %d hp %d delays %v/%v", z.CastleID, z.EventID, z.HPDamage, z.InitialDelay, z.ReuseDelay)
	}
	if z := load("SwampZone.xml").(*zone.Swamp); z.CastleID != 0 || z.EventID != 0 || z.MoveBonus != -50 {
		t.Errorf("swamp defaults: castle %d event %d bonus %d", z.CastleID, z.EventID, z.MoveBonus)
	}
	if z := load("EffectZone.xml").(*zone.Effect); z.Skills != nil || z.Chance != 100 || z.InitialDelay != 0 || z.ReuseDelay != 30*time.Second || z.Target != zone.ScopePlayer || !z.Enabled() {
		t.Errorf("effect defaults: skills %v chance %d delays %v/%v target %d enabled %v", z.Skills, z.Chance, z.InitialDelay, z.ReuseDelay, z.Target, z.Enabled())
	}
	if z := load("BossZone.xml").(*zone.Boss); z.InvadeWindow != 0 || z.OustLoc != (location.Location{}) {
		t.Errorf("boss defaults: invade %v oust %+v", z.InvadeWindow, z.OustLoc)
	}
}

// TestZoneSettingsOverrides: every setting a zone carries replaces its
// default, booleans read case-insensitively, and a siege battlefield naming
// both residences belongs to the clan hall.
func TestZoneSettingsOverrides(t *testing.T) {
	t.Parallel()
	load := func(file string, stats map[string]string) zone.Kind {
		t.Helper()
		k, err := loadOneZone(t, file, stats)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		return k
	}

	if z := load("MotherTreeZone.xml", map[string]string{"enterMsgId": "1", "leaveMsgId": "2", "MpRegenBonus": "3", "HpRegenBonus": "4", "affectedRace": "5"}).(*zone.MotherTree); z.EnterMessage != 1 || z.LeaveMessage != 2 || z.MPBonus != 3 || z.HPBonus != 4 || z.AffectedRace != 5 {
		t.Errorf("mother tree: messages %d/%d bonus hp %d mp %d race %d", z.EnterMessage, z.LeaveMessage, z.HPBonus, z.MPBonus, z.AffectedRace)
	}
	if z := load("TownZone.xml", map[string]string{"townId": "7", "castleId": "3", "isPeaceZone": "FALSE"}).(*zone.Town); z.TownID != 7 || z.CastleID != 3 || z.Peaceful {
		t.Errorf("town: id %d castle %d peaceful %v", z.TownID, z.CastleID, z.Peaceful)
	}
	if z := load("TownZone.xml", map[string]string{"isPeaceZone": "yes"}).(*zone.Town); z.Peaceful {
		t.Error("town isPeaceZone=yes reads peaceful, want false (only \"true\" is true)")
	}
	if z := load("SiegeZone.xml", map[string]string{"castleId": "3"}).(*zone.Siege); z.ResidenceID != 3 {
		t.Errorf("siege castle residence = %d, want 3", z.ResidenceID)
	}
	if z := load("SiegeZone.xml", map[string]string{"castleId": "3", "clanHallId": "34"}).(*zone.Siege); z.ResidenceID != 34 {
		t.Errorf("siege with both residences = %d, want the clan hall 34", z.ResidenceID)
	}
	if z := load("CastleTeleportZone.xml", map[string]string{"castleId": "1", "spawnMinX": "-10", "spawnMaxX": "10", "spawnMinY": "-20", "spawnMaxY": "20", "spawnZ": "-300"}).(*zone.CastleTeleport); z.CastleID != 1 || z.ExitMinX != -10 || z.ExitMaxX != 10 || z.ExitMinY != -20 || z.ExitMaxY != 20 || z.ExitZ != -300 {
		t.Errorf("castle teleport: castle %d exit x %d..%d y %d..%d z %d", z.CastleID, z.ExitMinX, z.ExitMaxX, z.ExitMinY, z.ExitMaxY, z.ExitZ)
	}
	if z := load("DamageZone.xml", map[string]string{"castleId": "5", "eventId": "6", "hpDamage": "40", "initialDelay": "1100", "reuseDelay": "5300"}).(*zone.Damage); z.CastleID != 5 || z.EventID != 6 || z.HPDamage != 40 || z.InitialDelay != 1100*time.Millisecond || z.ReuseDelay != 5300*time.Millisecond {
		t.Errorf("damage: castle %d event %d hp %d delays %v/%v", z.CastleID, z.EventID, z.HPDamage, z.InitialDelay, z.ReuseDelay)
	}
	if z := load("SwampZone.xml", map[string]string{"castleId": "2", "move_bonus": "-80"}).(*zone.Swamp); z.CastleID != 2 || z.MoveBonus != -80 {
		t.Errorf("swamp: castle %d bonus %d", z.CastleID, z.MoveBonus)
	}
	z := load("EffectZone.xml", map[string]string{"skill": "4644-2;4645-3", "chance": "70", "initialDelay": "1000", "reuseDelay": "6000", "targetType": "Npc", "defaultStatus": "false"}).(*zone.Effect)
	if len(z.Skills) != 2 || z.Skills[0] != (zone.SkillRef{ID: 4644, Level: 2}) || z.Skills[1] != (zone.SkillRef{ID: 4645, Level: 3}) ||
		z.Chance != 70 || z.InitialDelay != time.Second || z.ReuseDelay != 6*time.Second || z.Target != zone.ScopeNPC || z.Enabled() {
		t.Errorf("effect: skills %v chance %d delays %v/%v target %d enabled %v", z.Skills, z.Chance, z.InitialDelay, z.ReuseDelay, z.Target, z.Enabled())
	}
	if z := load("EffectZone.xml", map[string]string{"targetType": ""}).(*zone.Effect); z.Target != zone.ScopePlayer {
		t.Errorf("effect empty targetType = %d, want the player default", z.Target)
	}
	if z := load("BossZone.xml", map[string]string{"InvadeTime": "600000", "oustX": "1", "oustY": "-2", "oustZ": "3"}).(*zone.Boss); z.InvadeWindow != 10*time.Minute || z.OustLoc != (location.Location{X: 1, Y: -2, Z: 3}) {
		t.Errorf("boss: invade %v oust %+v", z.InvadeWindow, z.OustLoc)
	}
}
