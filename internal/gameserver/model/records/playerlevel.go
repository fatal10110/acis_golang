package records

import "github.com/fatal10110/acis_golang/internal/commons"

// PlayerLevel holds the experience and death-penalty parameters associated
// with reaching a specific character level (PlayerLevel.java).
type PlayerLevel struct {
	// RequiredExpToLevelUp is the total experience needed to advance from
	// the previous level into this one.
	RequiredExpToLevelUp int64
	// KarmaModifier scales karma gain/loss calculations at this level.
	KarmaModifier float64
	// ExpLossAtDeath is the percentage of the level's experience span lost
	// on death.
	ExpLossAtDeath float64
}

// NewPlayerLevel builds a PlayerLevel from set. requiredExpToLevelUp is
// required; karmaModifier and expLossAtDeath default to 0 when absent (the
// level-81 sentinel entry carries neither) but, like the Java StatSet
// default getters, still fail on a present-but-malformed value.
func NewPlayerLevel(set *commons.StatSet) (PlayerLevel, error) {
	exp, err := set.GetLong("requiredExpToLevelUp")
	if err != nil {
		return PlayerLevel{}, err
	}

	pl := PlayerLevel{RequiredExpToLevelUp: exp}
	if set.Has("karmaModifier") {
		if pl.KarmaModifier, err = set.GetDouble("karmaModifier"); err != nil {
			return PlayerLevel{}, err
		}
	}
	if set.Has("expLossAtDeath") {
		if pl.ExpLossAtDeath, err = set.GetDouble("expLossAtDeath"); err != nil {
			return PlayerLevel{}, err
		}
	}
	return pl, nil
}
