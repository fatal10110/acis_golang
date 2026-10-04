package siege

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
)

// RegisterAttacker registers cl to attack the castle. It reports the
// message refusing it, when it is refused: the castle owner's alliance may
// not attack it, an ally of cl may not stand on the other side, and cl
// must pass the registration checks (see canRegisterLocked).
func (s *Siege) RegisterAttacker(cl *clan.Clan) (refusal Message, refused bool) {
	if cl == nil {
		return Message{}, false
	}
	e := s.e
	e.mu.Lock()
	defer e.mu.Unlock()
	if owner, ok := e.clans.Get(s.castle.OwnerID()); ok && s.castle.OwnerID() != 0 {
		if allyID := owner.AllyID(); allyID != 0 && cl.AllyID() == allyID {
			return Message{ID: MsgCannotAttackAllianceCastle}, true
		}
	}
	if s.allyOnOtherSideLocked(cl, true) {
		return Message{ID: MsgCantAcceptAllyEnemy}, true
	}
	if m, ok := s.canRegisterLocked(cl, SideAttacker); !ok {
		return m, true
	}
	s.registerLocked(cl, SideAttacker)
	return Message{}, false
}

// RegisterDefender asks for cl to defend the castle; the castle lord then
// approves or refuses the request (ConfirmWaiting). It reports the message
// refusing it, when it is refused: a castle no clan owns has no room for
// defenders, an ally of cl may not stand on the other side, and cl must
// pass the registration checks (see canRegisterLocked).
func (s *Siege) RegisterDefender(cl *clan.Clan) (refusal Message, refused bool) {
	if cl == nil {
		return Message{}, false
	}
	e := s.e
	e.mu.Lock()
	defer e.mu.Unlock()
	if s.castle.OwnerID() <= 0 {
		return Message{ID: MsgDefenderSideFull}, true
	}
	if s.allyOnOtherSideLocked(cl, false) {
		return Message{ID: MsgCantAcceptAllyEnemy}, true
	}
	if m, ok := s.canRegisterLocked(cl, SidePending); !ok {
		return m, true
	}
	s.registerLocked(cl, SidePending)
	return Message{}, false
}

// Unregister drops cl's registration, unless cl owns the castle, and stores
// it.
func (s *Siege) Unregister(cl *clan.Clan) {
	if cl == nil || cl.CastleID() == int32(s.castle.ID) {
		return
	}
	s.e.mu.Lock()
	defer s.e.mu.Unlock()
	s.unregisterLocked(cl.ID())
}

func (s *Siege) unregisterLocked(clanID int32) {
	if !s.removeLocked(clanID) {
		return
	}
	castleID := int32(s.castle.ID)
	s.write("delete siege clan", func(ctx context.Context, st Store) error {
		return st.DeleteClan(ctx, castleID, clanID)
	})
}

// ConfirmWaiting is the castle lord's answer to cl's request to defend,
// while the registrations are open: approved, a pending cl defends;
// refused, a pending or defending cl is dropped.
func (s *Siege) ConfirmWaiting(cl *clan.Clan, approved bool) {
	e := s.e
	e.mu.Lock()
	defer e.mu.Unlock()
	if s.status != StatusRegistrationOpened {
		return
	}
	side := s.sideLocked(cl.ID())
	switch {
	case approved && side == SidePending:
		s.registerLocked(cl, SideDefender)
	case !approved && (side == SidePending || side == SideDefender):
		if cl.CastleID() != int32(s.castle.ID) {
			s.unregisterLocked(cl.ID())
		}
	}
}

// DropOwner drops clanID's registration without storing it: the castle's
// owner, registered on its own, losing the castle.
func (s *Siege) DropOwner(clanID int32) {
	s.e.mu.Lock()
	defer s.e.mu.Unlock()
	s.removeLocked(clanID)
}

// canRegisterLocked runs the registration checks for cl on side, in order:
// the registrations must be open (a siege under way has them closed too),
// cl must reach the minimum clan level, own no castle and be registered on
// no siege, and side must have room. It returns the message refusing cl.
func (s *Siege) canRegisterLocked(cl *clan.Clan, side Side) (Message, bool) {
	e := s.e
	switch {
	case s.status != StatusRegistrationOpened:
		return castleMsg(MsgDeadlinePassed, s.castle.ID), false
	case cl.Level() < e.cfg.MinClanLevel:
		return Message{ID: MsgOnlyClanLevel4}, false
	case cl.CastleID() > 0:
		if cl.ID() == s.castle.OwnerID() {
			return Message{ID: MsgOwnerAutomaticallyDefends}, false
		}
		return Message{ID: MsgOwnerCannotJoinOther}, false
	case e.registeredLocked(cl.ID()):
		// This also covers the reference's next check, a registration on
		// another siege fought the same day of the week (639), which a clan
		// registered on no siege cannot have.
		return Message{ID: MsgAlreadyRequested}, false
	case side == SideAttacker && s.countLocked(SideAttacker) >= e.cfg.MaxAttackers:
		return Message{ID: MsgAttackerSideFull}, false
	case side != SideAttacker && s.countLocked(SideOwner, SideDefender, SidePending) >= e.cfg.MaxDefenders:
		return Message{ID: MsgDefenderSideFull}, false
	}
	return Message{}, true
}

// registerLocked registers cl with side and stores it, unless cl owns a
// castle or side has no room left.
func (s *Siege) registerLocked(cl *clan.Clan, side Side) {
	if cl.CastleID() > 0 {
		return
	}
	if side == SideAttacker {
		if s.countLocked(SideAttacker) >= s.e.cfg.MaxAttackers {
			return
		}
	} else if s.countLocked(SideOwner, SideDefender, SidePending) >= s.e.cfg.MaxDefenders {
		return
	}
	castleID, clanID := int32(s.castle.ID), cl.ID()
	s.write("save siege clan", func(ctx context.Context, st Store) error {
		return st.SaveClan(ctx, castleID, clanID, side)
	})
	s.setSideLocked(clanID, side)
}

// allyOnOtherSideLocked reports whether another clan of cl's alliance
// stands on the side opposite the one cl asks for: owning, defending or
// asking to defend when cl attacks, attacking when it defends.
func (s *Siege) allyOnOtherSideLocked(cl *clan.Clan, attacker bool) bool {
	allyID := cl.AllyID()
	if allyID == 0 {
		return false
	}
	for _, ally := range s.e.clans.Allies(allyID) {
		if ally.ID() == cl.ID() {
			continue
		}
		side := s.sideLocked(ally.ID())
		if attacker && (side == SideDefender || side == SideOwner || side == SidePending) {
			return true
		}
		if !attacker && side == SideAttacker {
			return true
		}
	}
	return false
}

// registeredLocked reports whether clanID is registered on any siege.
func (e *Engine) registeredLocked(clanID int32) bool {
	for _, s := range e.order {
		if s.sideLocked(clanID) != SideNone {
			return true
		}
	}
	return false
}
