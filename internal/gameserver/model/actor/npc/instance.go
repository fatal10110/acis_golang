package npc

import (
	"errors"
	"fmt"
)

// InstanceKind identifies the behavior category selected by an NPC template.
type InstanceKind string

// Instance is an NPC spawned from a template.
type Instance struct {
	ObjectID int32
	Template *Template
	Kind     InstanceKind
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
