package npc

import (
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
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
	// SpawnHeading is the heading restored when a non-follow arrival lands
	// exactly on Home.
	SpawnHeading int
	// DriftRange is how far around Home the NPC's territory reaches and how
	// far it strays from Home before it heads back: the configured drift
	// range the spawn sets, the shipped 200 when not positive.
	DriftRange int
	// WalkMode forces walk stance (WalkSpeed, not RunSpeed) instead of the
	// default run stance every other NPC spawns in (the walker ids that
	// spawn walking).
	WalkMode bool
	// Maker is the spawning npc-maker group. InTerritory and idle wander
	// use its polygon when this is set. A nil Maker keeps the home-offset
	// walk unless the NPC is a private, which offsets from current position.
	Maker *spawn.Maker
	// Summoner is the creature the NPC was spawned for, a caught monster's
	// fisher; nil for none. Set before the NPC is placed, read-only after.
	Summoner attackable.Combatant
	// CastleID is the castle the NPC was given as it spawned, a mercenary
	// a siege hired or a castle's control tower; 0 for none, leaving it to
	// its maker and template. Set before the NPC is placed, read-only
	// after.
	CastleID int

	// name and title, once set, replace the template's; see Name and Title.
	name, title atomic.Pointer[string]
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

// MaxDriftRange is the NPC's drift range: DriftRange when positive, else the
// shipped 200.
func (inst *Instance) MaxDriftRange() int {
	if inst.DriftRange > 0 {
		return inst.DriftRange
	}
	return defaultDriftRange
}
