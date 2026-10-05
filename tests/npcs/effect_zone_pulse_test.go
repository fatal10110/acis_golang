package npcs

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// areaHaste is the shipped Area Buff - Haste (4644) level 2 the Swamp of
// Screams effect zones land on their monsters.
var areaHaste = modelskill.Definition{
	ID: 4644, Level: 2, Activation: modelskill.ActivationActive, Target: modelskill.TargetArea, SkillType: "BUFF",
	Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 5, StackType: "attack_time_down", StackOrder: 2}},
}

// effectZone builds an effect zone over form landing 4644-2 with every
// pulse, after a one second initial delay, on targetType.
func effectZone(t *testing.T, id int, form zone.Form, targetType string) *zone.Effect {
	t.Helper()
	set := commons.NewStatSet()
	set.Set("targetType", targetType)
	set.Set("skill", "4644-2")
	set.Set("initialDelay", "1000")
	set.Set("reuseDelay", "6000")
	z, err := zone.NewEffect(id, form, set)
	if err != nil {
		t.Fatal(err)
	}
	return z
}

// TestEffectZoneLandsItsSkillOnTheTargetScope pins EffectZone
// (EffectZone.java:207-283): an NPC standing in an Npc-scoped effect zone
// is its occupant, and the zone's pulse lands the zone's skill on it, the
// NPC being both caster and target; the same zone scoped to players never
// holds the NPC, so it never lands anything on it.
func TestEffectZoneLandsItsSkillOnTheTargetScope(t *testing.T) {
	t.Parallel()
	npcZone := effectZone(t, 1, zoneBox(t, 300, 1_000), "Npc")
	playerZone := effectZone(t, 2, zoneBox(t, 1_200, 2_000), "Player")
	zones := zone.NewIndex()
	zones.Add(npcZone)
	zones.Add(playerZone)
	db := sqltest.SharedDB(t)
	w := bootFolkWorld(t, nil, gameservertest.WithZones(zones), gameservertest.WithSkills(
		skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{areaHaste}), gamesql.NewCharacterSkillStore(db))))
	if !w.srv.DrivesClock() {
		t.Skip("pulse timing needs the driven clock")
	}

	inNPCZone := w.srv.SpawnHostileNPCKindAt(t, "Monster", location.Location{X: 600, Y: w.at.Y, Z: w.at.Z})
	inPlayerZone := w.srv.SpawnHostileNPCKindAt(t, "Monster", location.Location{X: 1_500, Y: w.at.Y, Z: w.at.Z})
	drainUntilQuiet(t, w.c)
	if got := len(npcZone.Occupants()); got != 1 {
		t.Fatalf("Npc-scoped zone occupants = %d, want the NPC inside it", got)
	}
	if got := len(playerZone.Occupants()); got != 0 {
		t.Fatalf("Player-scoped zone occupants = %d, want none: an NPC is outside its scope", got)
	}

	w.srv.AdvanceUntil(t, "the Npc-scoped zone's skill on its NPC", func() bool {
		_, ok := inNPCZone.EffectList().ActiveBySkillID(int(areaHaste.ID))
		return ok
	})
	w.srv.Advance(t, 6*time.Second)
	if _, ok := inPlayerZone.EffectList().ActiveBySkillID(int(areaHaste.ID)); ok {
		t.Fatal("the Player-scoped zone landed its skill on an NPC")
	}
}
