package summon

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"

// Weight-penalty bands a pet enters as its carried weight nears its limit,
// from none to fully overloaded, with the speed and regeneration
// multiplier each band applies.
const (
	weightPenaltyNone = iota
	weightPenaltyLevel1
	weightPenaltyLevel2
	weightPenaltyLevel3
	weightPenaltyLevel4
)

var (
	weightPenaltySpeed = [...]float64{1, 1, .5, .5, 0}
	weightPenaltyRegen = [...]float64{1, .5, .5, .5, .1}
)

// InventoryLimit returns how many item stacks a pet may carry; a servitor
// carries none.
func (a *Actor) InventoryLimit() int {
	if a.petConfig == nil {
		return 0
	}
	return a.petConfig.InventorySlots
}

// WeightLimit returns how much weight a pet may carry right now: its CON-
// and config-derived base through its weightLimit stat, so a buff raising
// it takes effect at once. A servitor carries nothing and has no limit.
func (a *Actor) WeightLimit() int {
	if !a.isPet || a.petConfig == nil {
		return 0
	}
	return int(a.calcStat(stat.WeightLimit, a.petConfig.BaseWeightLimit(a.CON())))
}

// WeightPenalty returns this summon's current weight-penalty band, 0 when
// it carries under half its weight limit. A servitor stays at 0.
func (a *Actor) WeightPenalty() int {
	return int(a.weightPenalty.Load())
}

// refreshWeightPenalty recomputes a pet's weight-penalty band from its
// carried weight and weight limit. A band change re-times the movement and
// republishes the pet's status once. A pet with no weight limit keeps its
// band, and a servitor carries nothing.
func (a *Actor) refreshWeightPenalty() {
	if a.storeWeightPenalty() {
		a.refreshMoveSpeed()
		a.BroadcastStatus()
	}
}

// settleWeightPenalty gives a pet just built around a restored inventory
// the band its load puts it in. Nothing has seen the pet yet, so nothing
// is republished: its first PetInfo carries the band's speed.
func (a *Actor) settleWeightPenalty() {
	a.storeWeightPenalty()
}

// storeWeightPenalty stores the band the pet's current load gives and
// reports whether it changed.
func (a *Actor) storeWeightPenalty() bool {
	inv := a.PetInventory()
	if inv == nil {
		return false
	}
	a.weightPenaltyMu.Lock()
	defer a.weightPenaltyMu.Unlock()
	limit := a.WeightLimit()
	if limit <= 0 {
		return false
	}
	ratio := (float64(inv.TotalWeight()) - a.calcStat(stat.WeightPenalty, 0)) / float64(limit)
	band := int32(weightPenaltyLevel4)
	switch {
	case ratio < .5:
		band = weightPenaltyNone
	case ratio < .666:
		band = weightPenaltyLevel1
	case ratio < .8:
		band = weightPenaltyLevel2
	case ratio < 1:
		band = weightPenaltyLevel3
	}
	return a.weightPenalty.Swap(band) != band
}
