// Package secondclass is what the second class-change quests share: the
// Dimensional Diamonds each set of them pays once per character, on top of
// its own reward.
package secondclass

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// The player memos marking a set's diamonds as paid.
const (
	secondClassChange35 = "secondClassChange35"
	secondClassChange37 = "secondClassChange37"
	secondClassChange39 = "secondClassChange39"
)

// dimensionalDiamond is the reward.
const dimensionalDiamond = 7562

// dfReward35 returns the diamonds of the level 35 set, by class id.
func dfReward35() map[int32]int32 {
	return map[int32]int32{
		1: 61, 4: 45, 7: 128, 11: 168, 15: 49, 19: 61, 22: 128, 26: 168, 29: 49,
		32: 61, 35: 128, 39: 168, 42: 49, 45: 61, 47: 61, 50: 49, 54: 85, 56: 85,
	}
}

// dfReward37 returns the diamonds of the level 37 set, by race.
func dfReward37() map[int32]int32 {
	return map[int32]int32{0: 96, 1: 102, 2: 98, 3: 109, 4: 50}
}

// dfReward39 returns the diamonds of the level 39 set, by class id.
func dfReward39() map[int32]int32 {
	return map[int32]int32{
		1: 72, 4: 104, 7: 96, 11: 122, 15: 60, 19: 72, 22: 96, 26: 122, 29: 45,
		32: 104, 35: 96, 39: 122, 42: 60, 45: 64, 47: 72, 50: 92, 54: 82, 56: 23,
	}
}

// GiveDimensionalDiamonds35 gives p the diamonds of the level 35 set for
// its class, unless p was given them before, and reports whether it gave.
func GiveDimensionalDiamonds35(s *script.Script, p *script.Player) bool {
	return give(s, p, secondClassChange35, dfReward35(), p.ClassID())
}

// GiveDimensionalDiamonds37 gives p the diamonds of the level 37 set for
// its race, unless p was given them before, and reports whether it gave.
func GiveDimensionalDiamonds37(s *script.Script, p *script.Player) bool {
	return give(s, p, secondClassChange37, dfReward37(), int32(p.Race()))
}

// GiveDimensionalDiamonds39 gives p the diamonds of the level 39 set for
// its class, unless p was given them before, and reports whether it gave.
func GiveDimensionalDiamonds39(s *script.Script, p *script.Player) bool {
	return give(s, p, secondClassChange39, dfReward39(), p.ClassID())
}

// give pays p the diamonds rewards lists for key and sets memo, once per
// memo. A key the table does not list panics before anything is given or
// set.
func give(s *script.Script, p *script.Player, memo string, rewards map[int32]int32, key int32) bool {
	if _, done := p.Memo(memo); done {
		return false
	}
	count, ok := rewards[key]
	if !ok {
		// The memo names the set whose table misses key.
		panic(memo)
	}
	s.GiveItems(p, dimensionalDiamond, count)
	p.SetMemo(memo, strconv.FormatBool(true))
	return true
}
