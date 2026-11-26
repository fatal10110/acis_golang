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
	// Limits holds the base buff-slot count and the drift range the NPC
	// reads at each use, shared with the other NPCs of its population; nil
	// reads DefaultLimits. Set before the NPC is placed, read-only after.
	Limits *LimitBook
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
	// ClanID is the clan the NPC was given as it spawned, a flag war's
	// warrior; 0 for none, leaving it to the owner of the residence it
	// serves. Set before the NPC is placed, read-only after.
	ClanID int32
	// ResidenceOwner returns the clan owning the residence the NPC serves,
	// read at each use, 0 for none; nil serves none. Set before the NPC is
	// placed, read-only after.
	ResidenceOwner func(*Instance) int32
	// Slot is the spawn slot placing the NPC; nil leaves it with no spawn
	// parameters and a script memory of its own. Set before the NPC is
	// built, read-only after.
	Slot SpawnSlot

	// name and title, once set, replace the template's; see Name and Title.
	name, title atomic.Pointer[string]
	// form is the NPC the NPC is shown as, nil while it is shown as itself;
	// see Polymorph.
	form atomic.Pointer[Template]
}

var supportedInstanceKinds = map[InstanceKind]struct{}{
	KindAdventurer:            {},
	KindAuctioneer:            {},
	KindBabyPet:               {},
	KindCastleBlacksmith:      {},
	KindCastleChamberlain:     {},
	KindCastleDoorman:         {},
	KindCastleGatekeeper:      {},
	KindCastleMagician:        {},
	KindCastleWarehouseKeeper: {},
	KindChest:                 {},
	KindChristmasTree:         {},
	KindClanHallDoorman:       {},
	KindClanHallManagerNpc:    {},
	KindClassMaster:           {},
	KindCubic:                 {},
	KindDawnPriest:            {},
	KindDerbyTrackManagerNpc:  {},
	KindDoor:                  {},
	KindDoorman:               {},
	KindDungeonGatekeeper:     {},
	KindDuskPriest:            {},
	KindEffectPoint:           {},
	KindFeedableBeast:         {},
	KindFence:                 {},
	KindFestivalGuide:         {},
	KindFestivalMonster:       {},
	KindFisherman:             {},
	KindFlameTower:            {},
	KindFolk:                  {},
	KindFriendlyMonster:       {},
	KindGatekeeper:            {},
	KindGrandBoss:             {},
	KindGuard:                 {},
	KindHalishaChest:          {},
	KindHolyThing:             {},
	KindLifeTower:             {},
	KindManorManagerNpc:       {},
	KindMercenaryManagerNpc:   {},
	KindMerchant:              {},
	KindMonster:               {},
	KindMutedFolk:             {},
	KindOlympiadManagerNpc:    {},
	KindPet:                   {},
	KindRaidBoss:              {},
	KindSchemeBuffer:          {},
	KindServitor:              {},
	KindSiegeFlag:             {},
	KindSiegeGuard:            {},
	KindSiegeNpc:              {},
	KindSiegeSummon:           {},
	KindSignsPriest:           {},
	KindStaticObject:          {},
	KindSymbolMaker:           {},
	KindTamedBeast:            {},
	KindTrainer:               {},
	KindVillageMaster:         {},
	KindVillageMasterDElf:     {},
	KindVillageMasterDwarf:    {},
	KindVillageMasterFighter:  {},
	KindVillageMasterMystic:   {},
	KindVillageMasterOrc:      {},
	KindVillageMasterPriest:   {},
	KindWarehouseKeeper:       {},
	KindWeddingManagerNpc:     {},
	KindWyvernManagerNpc:      {},
}

// Kind returns the instance kind t's type names, known or not.
func (t *Template) Kind() InstanceKind { return InstanceKind(t.Type) }

// NewInstance creates an NPC instance for a supported template type.
func NewInstance(objectID int32, template *Template) (*Instance, error) {
	if template == nil {
		return nil, errors.New("npc: nil template")
	}

	kind := template.Kind()
	if _, ok := supportedInstanceKinds[kind]; !ok {
		return nil, fmt.Errorf("npc %d: unsupported instance type %q", template.ID, template.Type)
	}

	return &Instance{ObjectID: objectID, Template: template, Kind: kind}, nil
}

// MaxDriftRange is the NPC's drift range in force: the Limits one when
// positive, else the shipped 200.
func (inst *Instance) MaxDriftRange() int {
	if r := inst.Limits.Limits().DriftRange; r > 0 {
		return r
	}
	return defaultDriftRange
}

// NPCClanID is the clan the NPC stands for: the one its spawn gave it,
// else the owner of the residence it serves; 0 for none.
func (i *Instance) NPCClanID() int32 {
	if i.ClanID != 0 {
		return i.ClanID
	}
	if i.ResidenceOwner != nil {
		return i.ResidenceOwner(i)
	}
	return 0
}
