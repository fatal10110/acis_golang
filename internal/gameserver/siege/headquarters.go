package siege

// HeadquartersRefusal is why a player may not put up a siege headquarters.
type HeadquartersRefusal uint8

const (
	// HeadquartersAllowed refuses nothing.
	HeadquartersAllowed HeadquartersRefusal = iota
	// HeadquartersNotAttacker: the player's clan does not attack the siege
	// fought where it stands, or no siege is.
	HeadquartersNotAttacker
	// HeadquartersNotLeader: the player does not lead its clan.
	HeadquartersNotLeader
	// HeadquartersAlreadyUp: the clan holds a headquarters already.
	HeadquartersAlreadyUp
	// HeadquartersNotHere: the player stands outside the ground
	// headquarters are built on.
	HeadquartersNotHere
	// HeadquartersTooClose: another headquarters stands too close.
	HeadquartersTooClose
)

// HeadquartersRange is how close, in 3D and widened by both bodies, no
// other headquarters may stand to a player putting one up.
const HeadquartersRange = 400

// HeadquartersSite is what a headquarters check reads of the player putting
// one up.
type HeadquartersSite struct {
	X, Y, Z int
	ClanID  int32
	// Leader reports the player leads its clan; HasHeadquarters that the
	// clan holds a headquarters.
	Leader, HasHeadquarters bool
	// OnHeadquartersGround reports the player stands where headquarters
	// are built.
	OnHeadquartersGround bool
	// NearHeadquarters reports another headquarters within
	// HeadquartersRange of the player.
	NearHeadquarters func() bool
	// HallAttacker reports whether the player's clan attacks the clan hall
	// siege whose battlefield is on where it stands; asked only when no
	// castle siege is fought there.
	HallAttacker func() bool
}

// CheckHeadquarters returns why the player at site may not put up a siege
// headquarters, checked in this order: its clan must attack the siege in
// progress whose battlefield holds it, or with none the clan hall siege
// whose battlefield is on there, it must lead the clan, the clan must hold
// no headquarters, it must stand on headquarters ground, and no other
// headquarters may stand within HeadquartersRange.
func (e *Engine) CheckHeadquarters(site HeadquartersSite) HeadquartersRefusal {
	var attacker bool
	if s, ok := e.ActiveAt(site.X, site.Y, site.Z); ok {
		attacker = s.CheckSides(site.ClanID, SideAttacker)
	} else if site.HallAttacker != nil {
		attacker = site.HallAttacker()
	}
	switch {
	case !attacker:
		return HeadquartersNotAttacker
	case !site.Leader:
		return HeadquartersNotLeader
	case site.HasHeadquarters:
		return HeadquartersAlreadyUp
	case !site.OnHeadquartersGround:
		return HeadquartersNotHere
	case site.NearHeadquarters != nil && site.NearHeadquarters():
		return HeadquartersTooClose
	}
	return HeadquartersAllowed
}
