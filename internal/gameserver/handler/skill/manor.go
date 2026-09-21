package skill

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// Inert until manor lands (#240): no item implements seedItem, so SOW never
// sows and HARVEST therefore never finds a sown target. That issue also owns
// the parity these handlers still lack — the Player-only caster gate, the
// Monster-only target gate, the sow/harvest system messages, the party
// harvest broadcast and the manor production rate — plus party-shared
// harvesting (#863).
//
// The caster gate is what makes HARVEST's ordering safe: it marks the crop
// consumed before it checks that the caster can be paid, so a caster that
// clears every gate without being an earner would eat the crop and receive
// nothing. The reference rejects a non-player caster before it touches the
// seed state at all, which is why its own reward call needs no such check.
//
// seedItem exposes the manor seed data an item carries when used to sow;
// resolving an item id to its Seed row (a manor.Table lookup) is the item's
// own job, not this handler's, since Cast carries no reference to global
// tables.
type seedItem interface {
	Seed() (manor.Seed, bool)
}

type sowCaster interface {
	Actor
	Level() int
}

type sowHandler struct{}

func (sowHandler) Types() []string { return []string{"SOW"} }

// Use sows the used item's seed onto the first target, when neither is
// already seeded and the sow roll succeeds.
func (sowHandler) Use(cast Cast) {
	if cast.Item == nil || len(cast.Targets) == 0 {
		return
	}
	caster, ok := cast.Caster.(sowCaster)
	if !ok {
		return
	}
	item, ok := cast.Item.(seedItem)
	if !ok {
		return
	}
	target, ok := asNPC(cast.Targets[0])
	if !ok || target.Dead() {
		return
	}

	state := target.SeedState()
	if state == nil || state.Seeded() {
		return
	}

	seed, ok := item.Seed()
	if !ok {
		return
	}

	rate := formulas.SowSuccessRate(seed.Level, target.Level(), caster.Level(), seed.Alternative)
	if rnd.Get(100) >= rate {
		return
	}

	state.Sow(caster.ObjectID(), seed)
}

type harvestCaster interface {
	Actor
	Level() int
}

// earner receives an item a harvest, sweep or extraction rewards directly
// to its owner (as opposed to a party distribution, which an optional
// interface layers on top).
type earner interface {
	AddEarnedItem(itemID int32, count int)
}

type harvestHandler struct{}

func (harvestHandler) Types() []string { return []string{"HARVEST"} }

// Use harvests the first target's sown crop into the caster's inventory,
// when the target is seeded, unharvested, the caster is allowed to harvest
// it, and the harvest roll succeeds.
func (harvestHandler) Use(cast Cast) {
	caster, ok := cast.Caster.(harvestCaster)
	if !ok {
		return
	}
	if len(cast.Targets) == 0 {
		return
	}
	target, ok := asNPC(cast.Targets[0])
	if !ok {
		return
	}

	state := target.SeedState()
	if state == nil || !state.Seeded() || state.Harvested() {
		return
	}
	if !state.AllowedToHarvest(caster.ObjectID()) {
		return
	}

	state.MarkHarvested()

	diff := caster.Level() - target.Level()
	if rnd.Get(100) >= formulas.HarvestSuccessRate(diff) {
		return
	}

	itemID, count := state.HarvestedCrop()
	if e, ok := cast.Caster.(earner); ok {
		e.AddEarnedItem(itemID, count)
	}
}
