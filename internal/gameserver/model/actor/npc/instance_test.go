package npc

import (
	"slices"
	"testing"
)

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

func TestSupportedInstanceKinds(t *testing.T) {
	got := SupportedInstanceKinds()
	if !slices.IsSorted(got) {
		t.Fatalf("SupportedInstanceKinds() = %v, want sorted kinds", got)
	}
	if !slices.Equal(got, supportedKindsOracle) {
		t.Fatalf("SupportedInstanceKinds() = %v, want %v", got, supportedKindsOracle)
	}

	got[0] = "mutated"
	if SupportedInstanceKinds()[0] != supportedKindsOracle[0] {
		t.Fatal("SupportedInstanceKinds() returned a mutable registry slice")
	}
}
