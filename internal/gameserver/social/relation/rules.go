package relation

// InviteRefusal is why a friend invitation is refused before it is sent, in
// the order the refusals are checked.
type InviteRefusal int

const (
	InviteAllowed InviteRefusal = iota
	// InviteTargetOffline: no player of that name is in the world.
	InviteTargetOffline
	// InviteSelf: a player cannot invite itself.
	InviteSelf
	// InviteTargetBlockingAll: the target blocks everything.
	InviteTargetBlockingAll
	// InviteTargetGM: a non-GM cannot invite a GM.
	InviteTargetGM
	// InviteBlocked: the requester is on the target's block list.
	InviteBlocked
	// InviteAlreadyFriends: the two are friends already.
	InviteAlreadyFriends
)

// InviteParties describes the two sides of a friend invitation as the
// network sees them.
type InviteParties struct {
	RequesterID   int32
	RequesterGM   bool
	TargetOnline  bool
	TargetID      int32
	TargetGM      bool
	TargetBlocked bool // the target blocks everything
}

// CheckInvite returns the first refusal p meets, or InviteAllowed. Whether
// the target is busy with another request is decided when the invitation is
// recorded (Invites.Offer), after every check here passes.
func (m *Manager) CheckInvite(p InviteParties) InviteRefusal {
	switch {
	case !p.TargetOnline:
		return InviteTargetOffline
	case p.TargetID == p.RequesterID:
		return InviteSelf
	case p.TargetBlocked:
		return InviteTargetBlockingAll
	case !p.RequesterGM && p.TargetGM:
		return InviteTargetGM
	case m.IsBlocked(p.TargetID, p.RequesterID):
		return InviteBlocked
	case m.AreFriends(p.RequesterID, p.TargetID):
		return InviteAlreadyFriends
	default:
		return InviteAllowed
	}
}

// BlockRefusal is why a block or unblock by name is refused.
type BlockRefusal int

const (
	BlockAllowed BlockRefusal = iota
	// BlockInvalidTarget: no character has that name, or it is the
	// player's own.
	BlockInvalidTarget
	// BlockTargetGM: the named character has an access level above 0.
	BlockTargetGM
)

// Named is a character a name lookup found: its id, its name as stored and
// its stored access level.
type Named struct {
	ID          int32
	Name        string
	AccessLevel int
}

// CheckBlockTarget returns why ownerID may not block or unblock target, the
// character a name lookup found; found reports whether it found one.
func CheckBlockTarget(ownerID int32, target Named, found bool) BlockRefusal {
	switch {
	case !found || target.ID <= 0 || target.ID == ownerID:
		return BlockInvalidTarget
	case target.AccessLevel > 0:
		return BlockTargetGM
	default:
		return BlockAllowed
	}
}
