// Package fishing runs a player's fishing: which fish a lure draws, the
// wait for a bite, and the reeling and pumping fight that follows. It holds
// no packets and no timers; the caller arms the timers, feeds each tick to
// a Stance and turns the outcomes it returns into client messages.
package fishing

import (
	"math"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/fish"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// Roll returns a uniform int in [0, n).
type Roll func(n int) int

// Skill and item ids the fishing rules read.
const (
	// ExpertiseSkillID is Fishing Expertise, whose level sets the fish
	// level drawn and the pumping and reeling penalty.
	ExpertiseSkillID = 1315
	// PotionSkillID is Fisherman's Potion: while its effect holds, its
	// skill power replaces the expertise level as the fish level drawn.
	PotionSkillID = 2274
)

// Lure groups: the fish group a lure draws from.
const (
	GroupBeginner = 0
	GroupNormal   = 1
	GroupUpper    = 2
)

// FirstLookDelay is how long a cast line waits before its first look for a
// bite. A line that has had no bite FirstLookDelay plus the fish's wait time
// after the cast is reeled in empty.
const FirstLookDelay = 10 * time.Second

// CombatTick is the period of the fight with a hooked fish.
const CombatTick = time.Second

// expertisePenalty is the damage a pumping or reeling skill loses when its
// level is at least expertisePenaltyGap above the caster's Fishing
// Expertise.
const (
	expertisePenalty    = 50
	expertisePenaltyGap = 3
)

// penaltyMonsterChance is the percent chance a caught fish turns out to be a
// monster instead.
const penaltyMonsterChance = 5

// penaltyMonsterBaseID is the first of the eight monsters a catch can turn
// into, one per eleven player levels.
const (
	penaltyMonsterBaseID   = 18319
	penaltyMonsterMaxIndex = 7
)

// LureGroup returns the fish group lureID draws from.
func LureGroup(lureID int32) int {
	switch lureID {
	case 7807, 7808, 7809, 8486:
		return GroupBeginner
	case 8506, 8509, 8512, 8485:
		return GroupUpper
	}
	return GroupNormal
}

// NightLure reports whether lureID only gets bites at night.
func NightLure(lureID int32) bool {
	return (lureID >= 8505 && lureID <= 8513) || lureID == 8485
}

// fishType returns the fish type lureID of group draws, check being a roll
// in [0, 100). A lure each group's table does not name draws type 1.
func fishType(group int, lureID int32, check int) int {
	switch group {
	case GroupBeginner:
		switch lureID {
		case 7807:
			return pick3(check, 54, 77, 5, 4, 6)
		case 7808:
			return pick3(check, 54, 77, 4, 6, 5)
		case 7809:
			return pick3(check, 54, 77, 6, 5, 4)
		case 8486:
			return pick3(check, 33, 66, 4, 5, 6)
		}
	case GroupNormal:
		switch lureID {
		case 7610, 7611, 7612, 7613:
			return 3
		case 6519, 6520, 6521, 8505, 8507:
			return pick4(check, 54, 1, 0)
		case 6522, 6523, 6524, 8508, 8510:
			return pick4(check, 54, 0, 1)
		case 6525, 6526, 6527, 8511, 8513:
			return pick4(check, 55, 2, 1)
		case 8484:
			return pick3(check, 33, 66, 0, 1, 2)
		}
	case GroupUpper:
		switch lureID {
		case 8506:
			return pick3(check, 54, 77, 8, 7, 9)
		case 8509:
			return pick3(check, 54, 77, 7, 9, 8)
		case 8512:
			return pick3(check, 54, 77, 9, 8, 7)
		case 8485:
			return pick3(check, 33, 66, 7, 8, 9)
		}
	}
	return 1
}

func pick3(check, first, second, a, b, c int) int {
	switch {
	case check <= first:
		return a
	case check <= second:
		return b
	}
	return c
}

// pick4 is a normal-group colored lure: its preferred type up to first, its
// second type up to 74, the remaining one of types 0-2 up to 94, and type 3
// above.
func pick4(check, first, preferred, second int) int {
	switch {
	case check <= first:
		return preferred
	case check <= 74:
		return second
	case check <= 94:
		return 3 - preferred - second
	}
	return 3
}

// fishLevel returns the level of the fish drawn by a fisher of level base
// (its potion power or expertise level): 1 when base is not positive,
// otherwise base, one lower on 35% and one higher on 15% of rolls, kept
// within 1-27.
func fishLevel(base int, roll Roll) int {
	if base <= 0 {
		return 1
	}
	check := roll(100)
	switch {
	case check < 35:
		base--
	case check < 50:
		base++
	}
	return min(max(base, 1), 27)
}

// Choose draws the fish a line baited with lureID hooks for a fisher whose
// potion power or expertise level is base: its level, then its type, then
// one fish of the table's matching rows. ok is false when no row matches.
func Choose(table *fish.Table, lureID int32, base int, roll Roll) (fish.Fish, bool) {
	if table == nil {
		return fish.Fish{}, false
	}
	group := LureGroup(lureID)
	level := fishLevel(base, roll)
	typ := fishType(group, lureID, roll(100))
	return table.Pick(level, typ, group, roll)
}

// CheckDelay returns how often a line baited with lureID looks for a bite
// from f, scaled by the lure grade: low-grade lures look less often,
// high-grade ones more. A lure no grade names returns 0: its line never
// looks for a bite.
func CheckDelay(lureID int32, gutsCheckTime int) time.Duration {
	var scale float64
	switch {
	case lureID == 6519 || lureID == 6522 || lureID == 6525 || lureID == 8505 || lureID == 8508 || lureID == 8511:
		scale = 1.33
	case lureID == 6520 || lureID == 6523 || lureID == 6526 || (lureID >= 8505 && lureID <= 8513) ||
		(lureID >= 7610 && lureID <= 7613) || (lureID >= 7807 && lureID <= 7809) || (lureID >= 8484 && lureID <= 8486):
		scale = 1.00
	case lureID == 6521 || lureID == 6524 || lureID == 6527:
		scale = 0.66
	default:
		return 0
	}
	scaled := float32(float64(gutsCheckTime) * scale)
	return time.Duration(math.Floor(float64(scaled)+0.5)) * time.Millisecond
}

// Damage returns what one pumping or reeling skill of power and skillLevel
// takes off a fish, fought with a rod of grade rod, doubled by a charged
// fishing shot. A skill at least three levels above the fisher's Fishing
// Expertise loses penalty points of it.
func Damage(power float32, rod item.CrystalType, fishShot bool, skillLevel, expertise int) (damage, penalty int) {
	shot := 1.0
	if fishShot {
		shot = 2
	}
	gradeBonus := 1 + float64(rod)*0.1
	damage = int(float64(power) * gradeBonus * shot)
	if skillLevel-expertise >= expertisePenaltyGap {
		penalty = expertisePenalty
		damage -= penalty
	}
	return damage, penalty
}

// CaughtMonster reports, from a roll in [0, 100), whether a caught fish
// turns out to be a monster.
func CaughtMonster(check int) bool {
	return check < penaltyMonsterChance
}

// PenaltyMonsterID is the monster a catch of a fisher of playerLevel turns
// into.
func PenaltyMonsterID(playerLevel int) int {
	return penaltyMonsterBaseID + min(playerLevel/11, penaltyMonsterMaxIndex)
}

// Fisher is the caster state a Fishing cast is refused on.
type Fisher interface {
	// FishingRod returns the grade of the rod in hand; ok is false
	// without one.
	FishingRod() (grade item.CrystalType, ok bool)
	Operating() bool
	InWater() bool
}

// Refusal is why a Fishing cast casts no line.
type Refusal uint8

const (
	// RefuseNone means the line may be cast.
	RefuseNone Refusal = iota
	// RefuseNoRod means no fishing rod is in hand.
	RefuseNoRod
	// RefuseOperating means the caster runs a private store or workshop.
	RefuseOperating
	// RefuseInWater means the caster stands in water.
	RefuseInWater
	// RefuseNoLure means no lure is on the hook.
	RefuseNoLure
)

// CastRefusal returns why f may not cast a line, in the order the checks
// run; baited reports a lure worn in the off hand. Standing on a boat also
// refuses a cast, which needs boat passengers to be modeled first.
func CastRefusal(f Fisher, baited bool) Refusal {
	if _, ok := f.FishingRod(); !ok {
		return RefuseNoRod
	}
	if f.Operating() {
		return RefuseOperating
	}
	if f.InWater() {
		return RefuseInWater
	}
	if !baited {
		return RefuseNoLure
	}
	return RefuseNone
}
