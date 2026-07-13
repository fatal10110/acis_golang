package skill

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// seedState is one seedable target's sow/harvest lifecycle: unseeded, sown
// (by a given player id, carrying which seed), then harvested at most once.
type seedState interface {
	Seeded() bool
	Sow(sowerID int32, seed manor.Seed)
	Harvested() bool
	MarkHarvested()
	// AllowedToHarvest reports whether playerID (the sower, or a member of
	// the sower's party) may harvest the crop.
	AllowedToHarvest(playerID int32) bool
	// HarvestedCrop returns the reward item id and quantity a successful
	// harvest grants; computing it (strong-type passive bonus, monster
	// level vs seed level, the manor drop-rate config) is the target's own
	// job, not this handler's.
	HarvestedCrop() (itemID int32, count int)
}

type seedableTarget interface {
	Dead() bool
	Level() int
	SeedState() seedState
}

// seedItem exposes the manor seed data an item carries when used to sow;
// resolving an item id to its Seed row (a manor.Table lookup) is the item's
// own job, not this handler's, since Cast carries no reference to global
// tables.
type seedItem interface {
	Seed() (manor.Seed, bool)
}

type sowCaster interface {
	ObjectID() int32
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
	target, ok := cast.Targets[0].(seedableTarget)
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
	ObjectID() int32
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
	target, ok := cast.Targets[0].(seedableTarget)
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
