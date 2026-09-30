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
	inv := a.PetInventory()
	if inv == nil || inv.WeightLimit <= 0 {
		return
	}
	a.weightPenaltyMu.Lock()
	ratio := (float64(inv.TotalWeight()) - a.calcStat(stat.WeightPenalty, 0)) / float64(inv.WeightLimit)
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
	changed := a.weightPenalty.Swap(band) != band
	a.weightPenaltyMu.Unlock()
	if changed {
		a.refreshMoveSpeed()
		a.BroadcastStatus()
	}
}
