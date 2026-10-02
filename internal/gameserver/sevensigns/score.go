package sevensigns

import "github.com/fatal10110/acis_golang/internal/commons"

// Seal-stone values: each turned-in stone is worth this many points to the
// player's cabal and to the ancient adena the player can collect.
const (
	blueStoneValue  = 3
	greenStoneValue = 5
	redStoneValue   = 10
)

// Seal ownership thresholds, in percent of a cabal's members who chose the
// seal: an owner keeps a seal at retainPercent, a winner claims one at
// claimPercent.
const (
	retainPercent = 10
	claimPercent  = 35
)

// maxStoneProportion is the score the whole stone pot is scaled to, the
// same as the maximum obtainable from festivals.
const maxStoneProportion = 500

// StoneScore returns the points the given stones are worth.
func StoneScore(blue, green, red int) int {
	return blue*blueStoneValue + green*greenStoneValue + red*redStoneValue
}

// roundFloat rounds f half up to an int, saturating at the int32 range, the
// way single-precision scores are rounded everywhere in this system.
func roundFloat(f float32) int {
	return int(commons.JavaInt(float64(commons.JavaRound(float64(f)))))
}

// share is part/whole scaled to scale, computed in single precision.
func share(part, whole int, scale float32) int {
	return roundFloat(float32(float32(part)/float32(whole)) * scale)
}

// cabalScore is a cabal's total score: its share of the stone pot scaled to
// 500, computed in double precision and then narrowed, plus its festival
// score. An empty pot counts as one point so the share is zero.
func cabalScore(stone, totalStone float64, festival int) int {
	divisor := totalStone
	if float32(totalStone) == 0 {
		divisor = 1
	}
	return roundFloat(float32(float32(stone/divisor)*maxStoneProportion)) + festival
}

// stoneProportion is a cabal's share of the stone pot scaled to 500, in
// single precision, or zero for an empty pot.
func stoneProportion(stone, totalStone float64) int {
	if totalStone == 0 {
		return 0
	}
	return roundFloat(float32(float32(stone)/float32(totalStone)) * maxStoneProportion)
}

// Prediction is the reason given for a seal's predicted owner.
type Prediction int

const (
	// PredictionTie: the competition is tied, or the owner keeps too few
	// votes after a tie; the seal is not awarded.
	PredictionTie Prediction = iota
	// PredictionOwnedRetained: the owner keeps the seal with 10% or more.
	PredictionOwnedRetained
	// PredictionOwnedLost: the owner falls under 10% and loses the seal.
	PredictionOwnedLost
	// PredictionClaimed: an unowned or rival seal is claimed with 35% or more.
	PredictionClaimed
	// PredictionNotClaimed: the winner gets under 35% for an unowned seal.
	PredictionNotClaimed
)

// sealOutcome decides who owns a seal after a competition won by winner,
// given its owner and the percent of each cabal's members who chose it.
// The owner keeps it with 10% of its members while not outvoted by a winning
// rival with 35%; a winner claims an unowned seal with 35%.
func sealOutcome(owner, winner Cabal, dawnPercent, duskPercent int) (Cabal, Prediction) {
	switch owner {
	case Dawn:
		return heldSealOutcome(Dawn, winner, dawnPercent, duskPercent)
	case Dusk:
		return heldSealOutcome(Dusk, winner, duskPercent, dawnPercent)
	}
	switch winner {
	case Dawn:
		if dawnPercent >= claimPercent {
			return Dawn, PredictionClaimed
		}
		return NoCabal, PredictionNotClaimed
	case Dusk:
		if duskPercent >= claimPercent {
			return Dusk, PredictionClaimed
		}
		return NoCabal, PredictionNotClaimed
	}
	return NoCabal, PredictionTie
}

// heldSealOutcome is sealOutcome for a seal owned by owner, whose members
// chose it at ownPercent and its rival's at rivalPercent.
func heldSealOutcome(owner, winner Cabal, ownPercent, rivalPercent int) (Cabal, Prediction) {
	switch winner {
	case NoCabal:
		if ownPercent >= retainPercent {
			return owner, PredictionOwnedRetained
		}
		return NoCabal, PredictionTie
	case owner:
	default:
		if rivalPercent >= claimPercent {
			return winner, PredictionClaimed
		}
	}
	if ownPercent >= retainPercent {
		return owner, PredictionOwnedRetained
	}
	return NoCabal, PredictionOwnedLost
}
