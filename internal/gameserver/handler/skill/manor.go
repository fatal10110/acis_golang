package skill

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// ManorMessage is a sow or harvest outcome its player caster is told.
type ManorMessage uint8

const (
	// SeedAlreadySown: the target was sown before (THE_SEED_HAS_BEEN_SOWN).
	SeedAlreadySown ManorMessage = iota + 1
	// SeedNotSown: the sow roll failed (THE_SEED_WAS_NOT_SOWN).
	SeedNotSown
	// SeedSown: the seed took (THE_SEED_WAS_SUCCESSFULLY_SOWN).
	SeedSown
	// HarvestTargetNotSown: the target is no monster or was never sown
	// (THE_HARVEST_FAILED_BECAUSE_THE_SEED_WAS_NOT_SOWN).
	HarvestTargetNotSown
	// HarvestFailed: the crop was already taken, or the harvest roll failed
	// (THE_HARVEST_HAS_FAILED).
	HarvestFailed
	// HarvestNotAuthorized: the harvester is neither the sower nor in the
	// sower's party (YOU_ARE_NOT_AUTHORIZED_TO_HARVEST).
	HarvestNotAuthorized
)

// CropHarvested tells the rest of the harvester's party what it harvested:
// Count units of CropID.
type CropHarvested struct {
	CropID int32
	Count  int
}

// seedItem exposes the manor seed data an item carries when used to sow;
// resolving an item id to its Seed row (a manor.Table lookup) is the item's
// own job, not this handler's, since Cast carries no reference to global
// tables.
type seedItem interface {
	Seed() (manor.Seed, bool)
}

type sowHandler struct{}

func (sowHandler) Types() []string { return []string{"SOW"} }

// Use sows the used item's seed onto the first target. Only a player sows,
// only from a seed item, and only onto a living monster; an already sown
// target and a failed sow roll are answered, and a seed that takes marks
// the monster sown by the caster.
func (sowHandler) Use(cast Cast) {
	if cast.Item == nil || len(cast.Targets) == 0 {
		return
	}
	caster, ok := asPlayer(cast.Caster)
	if !ok {
		return
	}
	item, ok := cast.Item.(seedItem)
	if !ok {
		return
	}
	target, ok := asMonster(cast.Targets[0])
	if !ok || target.Dead() {
		return
	}

	state := target.SeedState()
	if state == nil {
		return
	}
	if state.Seeded() {
		cast.record(SeedAlreadySown)
		return
	}

	seed, ok := item.Seed()
	if !ok {
		return
	}

	rate := formulas.SowSuccessRate(seed.Level, target.Level(), caster.Level(), seed.Alternative)
	if rnd.Get(100) >= rate {
		cast.record(SeedNotSown)
		return
	}

	// A concurrent sower can take this life first; losing that race reads
	// as the target already sown.
	if !state.Sow(caster.ObjectID(), seed) {
		cast.record(SeedAlreadySown)
		return
	}
	cast.record(SeedSown)
}

// harvester is a player caster that can take a harvested crop: it earns
// items and knows its party.
type harvester interface {
	Player
	earner
	// InPartyWith reports whether playerID is in this player's party.
	InPartyWith(playerID int32) bool
}

// earner is a player caster that takes the items a harvest or a sweep
// pays out as earned.
type earner interface {
	AddEarnedItem(itemID int32, count int, nextID func() (int32, error)) bool
}

// harvestHandler harvests crops. Without ids it cannot create the crop and
// harvests nothing. cropRate multiplies every harvested crop count.
type harvestHandler struct {
	ids      objectIDAllocator
	cropRate int
}

func (harvestHandler) Types() []string { return []string{"HARVEST"} }

// Use harvests the first target's sown crop into a player caster's
// inventory. The crop is spent as soon as the caster is found allowed to
// harvest it, so a failed harvest roll loses it; a successful one earns it
// and tells the caster's party.
func (h harvestHandler) Use(cast Cast) {
	if h.ids == nil || len(cast.Targets) == 0 {
		return
	}
	if _, ok := asPlayer(cast.Caster); !ok {
		return
	}
	caster, ok := cast.Caster.(harvester)
	if !ok {
		return
	}
	target, ok := asMonster(cast.Targets[0])
	if !ok {
		cast.record(HarvestTargetNotSown)
		return
	}
	state := target.SeedState()
	if state == nil {
		cast.record(HarvestTargetNotSown)
		return
	}

	// The claim checks sown, unharvested and allowed-to-harvest and spends
	// the crop in one step, so two harvesters cannot both take one crop.
	switch state.ClaimHarvest(caster.ObjectID(), caster.InPartyWith) {
	case npc.HarvestNotSown:
		cast.record(HarvestTargetNotSown)
		return
	case npc.HarvestAlreadyHarvested:
		cast.record(HarvestFailed)
		return
	case npc.HarvestNotAuthorized:
		cast.record(HarvestNotAuthorized)
		return
	}

	diff := caster.Level() - target.Level()
	if rnd.Get(100) >= formulas.HarvestSuccessRate(diff) {
		cast.record(HarvestFailed)
		return
	}

	cropID, count := state.HarvestedCrop(h.cropRate)
	caster.AddEarnedItem(cropID, count, h.ids.NextID)
	cast.record(CropHarvested{CropID: cropID, Count: count})
}

// asMonster returns a as a Monster-family NPC, or false for any other
// participant.
func asMonster(a Actor) (NPC, bool) {
	n, ok := asNPC(a)
	if !ok || !n.MonsterKind() {
		return nil, false
	}
	return n, true
}
