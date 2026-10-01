package npc

import (
	"sort"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// ---- from instance_test.go ----
var supportedKindsOracle = []InstanceKind{
	"Adventurer", "Auctioneer", "BabyPet", "CastleBlacksmith", "CastleChamberlain",
	"CastleDoorman", "CastleGatekeeper", "CastleMagician", "CastleWarehouseKeeper", "Chest",
	"ChristmasTree", "ClanHallDoorman", "ClanHallManagerNpc", "ClassMaster", "Cubic",
	"DawnPriest", "DerbyTrackManagerNpc", "Door", "Doorman", "DungeonGatekeeper",
	"DuskPriest", "EffectPoint", "FeedableBeast", "Fence", "FestivalGuide", "FestivalMonster",
	"Fisherman", "FlameTower", "Folk", "FriendlyMonster", "Gatekeeper", "GrandBoss", "Guard",
	"HalishaChest", "HolyThing", "LifeTower", "ManorManagerNpc", "MercenaryManagerNpc", "Merchant",
	"Monster", "MutedFolk", "OlympiadManagerNpc", "Pet", "RaidBoss", "SchemeBuffer", "Servitor",
	"SiegeFlag", "SiegeGuard", "SiegeNpc", "SiegeSummon", "SignsPriest", "StaticObject", "SymbolMaker",
	"TamedBeast", "Trainer", "VillageMaster", "VillageMasterDElf", "VillageMasterDwarf", "VillageMasterFighter",
	"VillageMasterMystic", "VillageMasterOrc", "VillageMasterPriest", "WarehouseKeeper", "WeddingManagerNpc", "WyvernManagerNpc",
}

func TestNewInstance_AllSupportedKinds(t *testing.T) {
	if len(supportedKindsOracle) != 65 {
		t.Fatalf("oracle has %d kinds, want 65", len(supportedKindsOracle))
	}

	for _, kind := range supportedKindsOracle {
		t.Run(string(kind), func(t *testing.T) {
			got, err := NewInstance(101, &Template{ID: 9001, Type: string(kind)})
			if err != nil {
				t.Fatalf("NewInstance() error: %v", err)
			}
			if got.ObjectID != 101 || got.Template.ID != 9001 || got.Kind != kind {
				t.Fatalf("instance = %+v", got)
			}
		})
	}
}

func TestNewInstance_RejectsInvalidTemplate(t *testing.T) {
	for _, tpl := range []*Template{nil, {Type: ""}, {Type: "NotAType"}} {
		if _, err := NewInstance(1, tpl); err == nil {
			t.Fatalf("NewInstance(%+v) error = nil", tpl)
		}
	}
}

// ---- from race_test.go ----
func TestRaceBySecondarySkillID(t *testing.T) {
	for skillID, want := range map[int]Race{
		4295: RaceHumanoid,
		4296: RaceSpirit,
		4297: RaceAngel,
		4298: RaceDemon,
	} {
		if got := RaceBySecondarySkillID(skillID); got != want {
			t.Fatalf("RaceBySecondarySkillID(%d) = %v, want %v", skillID, got, want)
		}
	}
	if got := RaceBySecondarySkillID(4290); got != RaceUndead {
		t.Fatalf("RaceBySecondarySkillID(4290) = %v, want RaceUndead", got)
	}
	if got := RaceBySecondarySkillID(4302); got != RaceFairy {
		t.Fatalf("RaceBySecondarySkillID(4302) = %v, want RaceFairy", got)
	}
	if got := RaceBySecondarySkillID(4416); got != RaceDummy {
		t.Fatalf("RaceBySecondarySkillID(4416) = %v, want RaceDummy (not a secondary marker)", got)
	}
	if got := RaceBySecondarySkillID(1); got != RaceDummy {
		t.Fatalf("RaceBySecondarySkillID(1) = %v, want RaceDummy", got)
	}
}

func TestRaceByOrdinal(t *testing.T) {
	got, ok := RaceByOrdinal(13)
	if !ok || got != RaceFairy {
		t.Fatalf("RaceByOrdinal(13) = %v, %v, want RaceFairy, true", got, ok)
	}
	if _, ok := RaceByOrdinal(-1); ok {
		t.Fatal("RaceByOrdinal(-1) ok = true, want false")
	}
	if _, ok := RaceByOrdinal(len(raceNames)); ok {
		t.Fatal("RaceByOrdinal(len) ok = true, want false")
	}
}

// ---- from template_test.go ----
func TestNewPrivateEntryKeepsIDInFieldError(t *testing.T) {
	for _, missing := range []string{"weight", "respawn"} {
		t.Run(missing, func(t *testing.T) {
			set := commons.NewStatSet()
			set.Set("id", 123)
			set.Set("weight", 1)
			set.Set("respawn", "1sec")
			set.Unset(missing)

			_, err := NewPrivateEntry(set)
			if err == nil || !strings.Contains(err.Error(), "npc: private entry 123") {
				t.Fatalf("NewPrivateEntry() error = %v, want entry id", err)
			}
		})
	}
}

func TestTable_All(t *testing.T) {
	table := NewTable([]*Template{
		{ID: 30, Name: "c"},
		{ID: 10, Name: "a"},
		{ID: 20, Name: "b"},
	})

	all := table.All()
	if len(all) != table.Len() {
		t.Fatalf("All() returned %d templates, Len() = %d", len(all), table.Len())
	}

	var ids []int
	for _, tpl := range all {
		ids = append(ids, tpl.ID)
	}
	if !sort.IntsAreSorted(ids) {
		t.Fatalf("All() not sorted ascending by ID: %v", ids)
	}
	if ids[0] != 10 || ids[len(ids)-1] != 30 {
		t.Fatalf("All() ids = %v, want [10 20 30]", ids)
	}
}
