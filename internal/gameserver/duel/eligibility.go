package duel

import "github.com/fatal10110/acis_golang/internal/gameserver/model/location"

// Candidate is what deciding whether a player may join a duel reads.
type Candidate struct {
	InCombat bool
	Jailed   bool
	// AlikeDead covers a dead player and one feigning death.
	AlikeDead bool
	// CurrentHP, MaxHP, CurrentMP and MaxMP are the player's resources.
	CurrentHP, MaxHP float64
	CurrentMP, MaxMP float64
	InDuel           bool
	InOlympiad       bool
	// Chaotic covers a cursed weapon, karma and a PvP flag.
	Chaotic bool
	// Operating covers a private store or workshop being set up or open.
	Operating bool
	// Riding covers a mount and a boat.
	Riding  bool
	Fishing bool
	// ProhibitedArea covers a peace, siege, PvP, water or no-restart zone.
	ProhibitedArea bool
}

// Refusal is why a player may not join a duel.
type Refusal uint8

// Refusals, in the order they are checked.
const (
	// Eligible lets the player duel.
	Eligible Refusal = iota
	RefusedInBattle
	RefusedHPOrMPBelowHalf
	RefusedAlreadyDuelling
	RefusedOlympiad
	RefusedChaotic
	RefusedPrivateStore
	RefusedRiding
	RefusedFishing
	RefusedProhibitedArea
)

// CanDuel decides whether a player in c may join a duel. The first refusal
// that applies wins.
func CanDuel(c Candidate) Refusal {
	switch {
	case c.InCombat || c.Jailed:
		return RefusedInBattle
	case c.AlikeDead || c.CurrentHP < c.MaxHP*0.5 || c.CurrentMP < c.MaxMP*0.5:
		return RefusedHPOrMPBelowHalf
	case c.InDuel:
		return RefusedAlreadyDuelling
	case c.InOlympiad:
		return RefusedOlympiad
	case c.Chaotic:
		return RefusedChaotic
	case c.Operating:
		return RefusedPrivateStore
	case c.Riding:
		return RefusedRiding
	case c.Fishing:
		return RefusedFishing
	case c.ProhibitedArea:
		return RefusedProhibitedArea
	}
	return Eligible
}

// InRange reports whether a and b stand within Range of each other.
func InRange(a, b Player) bool {
	ax, ay, az := a.Position()
	bx, by, bz := b.Position()
	return location.In3DRadius(ax, ay, az, bx, by, bz, Range)
}
