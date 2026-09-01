package npc

import (
	"errors"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

// InstanceKind identifies the behavior category selected by an NPC template.
type InstanceKind string

// Instance is an NPC spawned from a template.
type Instance struct {
	ObjectID int32
	Template *Template
	Kind     InstanceKind

	// Home is the spawn point this NPC returns to when it leashes.
	Home location.Location
	// HasHome reports whether Home was populated by the spawn runtime.
	HasHome bool
	// SpawnHeading is the heading this NPC restores whenever movement
	// arrives back exactly at Home (aCis NpcAI.onEvtArrived).
	SpawnHeading int
	// DriftRange overrides the default home radius when positive.
	DriftRange int
	// WalkMode forces walk stance (WalkSpeed, not RunSpeed) instead of the
	// default run stance every other NPC spawns in (aCis Walkers.java
	// onCreated's setWalkOrRun(false) for its WALKING_NPCS id subset).
	WalkMode bool
	// Maker is the spawning npc-maker group. Idle wander uses its
	// territory when this is set; a nil Maker (single spawns, minions,
	// test fixtures) keeps the home-offset walk.
	Maker *spawn.Maker
}

var supportedInstanceKinds = map[InstanceKind]struct{}{
	"Adventurer":            {},
	"Auctioneer":            {},
	"BabyPet":               {},
	"CastleBlacksmith":      {},
	"CastleChamberlain":     {},
	"CastleDoorman":         {},
	"CastleGatekeeper":      {},
	"CastleMagician":        {},
	"CastleWarehouseKeeper": {},
	"Chest":                 {},
	"ChristmasTree":         {},
	"ClanHallDoorman":       {},
	"ClanHallManagerNpc":    {},
	"ClassMaster":           {},
	"Cubic":                 {},
	"DawnPriest":            {},
	"DerbyTrackManagerNpc":  {},
	"Door":                  {},
	"Doorman":               {},
	"DungeonGatekeeper":     {},
	"DuskPriest":            {},
	"EffectPoint":           {},
	"FeedableBeast":         {},
	"Fence":                 {},
	"FestivalGuide":         {},
	"FestivalMonster":       {},
	"Fisherman":             {},
	"FlameTower":            {},
	"Folk":                  {},
	"FriendlyMonster":       {},
	"Gatekeeper":            {},
	"GrandBoss":             {},
	"Guard":                 {},
	"HalishaChest":          {},
	"HolyThing":             {},
	"LifeTower":             {},
	"ManorManagerNpc":       {},
	"MercenaryManagerNpc":   {},
	"Merchant":              {},
	"Monster":               {},
	"MutedFolk":             {},
	"OlympiadManagerNpc":    {},
	"Pet":                   {},
	"RaidBoss":              {},
	"SchemeBuffer":          {},
	"Servitor":              {},
	"SiegeFlag":             {},
	"SiegeGuard":            {},
	"SiegeNpc":              {},
	"SiegeSummon":           {},
	"SignsPriest":           {},
	"StaticObject":          {},
	"SymbolMaker":           {},
	"TamedBeast":            {},
	"Trainer":               {},
	"VillageMaster":         {},
	"VillageMasterDElf":     {},
	"VillageMasterDwarf":    {},
	"VillageMasterFighter":  {},
	"VillageMasterMystic":   {},
	"VillageMasterOrc":      {},
	"VillageMasterPriest":   {},
	"WarehouseKeeper":       {},
	"WeddingManagerNpc":     {},
	"WyvernManagerNpc":      {},
}

// NewInstance creates an NPC instance for a supported template type.
func NewInstance(objectID int32, template *Template) (*Instance, error) {
	if template == nil {
		return nil, errors.New("npc: nil template")
	}

	kind := InstanceKind(template.Type)
	if _, ok := supportedInstanceKinds[kind]; !ok {
		return nil, fmt.Errorf("npc %d: unsupported instance type %q", template.ID, template.Type)
	}

	return &Instance{ObjectID: objectID, Template: template, Kind: kind}, nil
}
