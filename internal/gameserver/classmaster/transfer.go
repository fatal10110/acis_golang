package classmaster

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"

// Refusal is why an occupation change request was refused.
type Refusal int

const (
	// Accepted lets the change go ahead.
	Accepted Refusal = iota
	// Refused is a talker below the level its next change needs, or a class
	// it may not change to; nothing is said.
	Refused
	// Overweight is a talker in the third weight band or above asking for
	// a change that hands out items.
	Overweight
	// NotEnoughItems is a talker short of an item the change takes.
	NotEnoughItems
	// Unconfigured is a change whose tier has no configured items at all:
	// a change of more than one tier at once, under AllowEntireTree, past
	// the tiers ConfigClassMaster lists. The request stops there, and
	// nothing more is sent, not even the closing ActionFailed.
	Unconfigured
)

// Talker is what an occupation change reads of the player asking for it.
type Talker struct {
	ClassID       int
	Level         int
	WeightPenalty int
	// ItemCount is how many units of an item the player holds, worn ones
	// included.
	ItemCount func(itemID int32) int
}

// CheckTransfer decides whether t may change to class next, and returns
// the configured change it then pays for: the one of the tier after t's
// current class, whatever tier next is.
func (c Config) CheckTransfer(t Talker, next int) (Job, Refusal) {
	tier, _ := player.ClassLevel(t.ClassID)
	if changeLevel(tier) > t.Level && !c.AllowEntireTree {
		return Job{}, Refused
	}
	if !c.canTransfer(t.ClassID, next) {
		return Job{}, Refused
	}
	job, ok := c.Job(tier + 1)
	if t.WeightPenalty > 2 {
		if !ok {
			return Job{}, Unconfigured
		}
		if len(job.Reward) > 0 {
			return Job{}, Overweight
		}
	}
	if !ok {
		return Job{}, Unconfigured
	}
	for _, it := range job.Required {
		if t.ItemCount(it.ID) < it.Count {
			return Job{}, NotEnoughItems
		}
	}
	return job, Accepted
}
