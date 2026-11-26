package party

// Invitee is a player as a party invitation judges it, on either side.
type Invitee interface {
	ObjectID() int32
	CursedWeaponEquipped() bool
	Invisible() bool
	// ClientDetached reports whether the player's connection is gone,
	// though it may still linger in the world.
	ClientDetached() bool
	Jailed() bool
	OlympiadMode() bool
}

// InviteRefusal is why a party invitation is refused before the inviter's
// party is asked.
type InviteRefusal uint8

// The party invitation refusals, in the order they are checked.
const (
	InviteAllowed InviteRefusal = iota
	// InviteWrongTarget refuses an invitation of oneself, from or to a
	// player carrying a cursed weapon, or of an invisible player.
	InviteWrongTarget
	// InviteTargetInParty refuses a target already in a party.
	InviteTargetInParty
	// InviteTargetDetached refuses a target whose connection is gone.
	InviteTargetDetached
	// InviteJailed refuses an invitation with either side in jail.
	InviteJailed
	// InviteOlympiad turns away an invitation with either side in an
	// Olympiad match; neither side is told.
	InviteOlympiad
)

// CheckInvite judges inviter's invitation of target, which their block
// lists let through, before the inviter's party is asked (BeginInvite).
func (r *Registry[M]) CheckInvite(inviter, target Invitee) InviteRefusal {
	switch {
	case target.ObjectID() == inviter.ObjectID(), target.CursedWeaponEquipped(), inviter.CursedWeaponEquipped(), target.Invisible():
		return InviteWrongTarget
	case r.InParty(target.ObjectID()):
		return InviteTargetInParty
	case target.ClientDetached():
		return InviteTargetDetached
	case target.Jailed(), inviter.Jailed():
		return InviteJailed
	case target.OlympiadMode(), inviter.OlympiadMode():
		return InviteOlympiad
	}
	return InviteAllowed
}
