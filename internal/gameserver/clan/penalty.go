package clan

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"

// Alliance penalty types, as a clan's ally_penalty_type column stores them.
const (
	allyPenaltyClanLeft      = 1
	allyPenaltyClanDismissed = 2
	allyPenaltyDismissClan   = 3
	allyPenaltyDissolveAlly  = 4
)

// PenaltyKind names one line of a character's clan penalty report.
type PenaltyKind uint8

// Penalty report lines, in the order the report lists them.
const (
	// PenaltyJoinClan: the character may not join a clan.
	PenaltyJoinClan PenaltyKind = iota
	// PenaltyCreateClan: the character may not found a clan.
	PenaltyCreateClan
	// PenaltyInviteMember: the clan may not recruit.
	PenaltyInviteMember
	// PenaltyJoinAlliance: the clan, having left or been dismissed from an
	// alliance, may not join one.
	PenaltyJoinAlliance
	// PenaltyInviteAllyMember: the alliance leader's clan, having dismissed
	// a clan, may not invite one.
	PenaltyInviteAllyMember
	// PenaltyCreateAlliance: the clan, having dissolved its alliance, may
	// not found one.
	PenaltyCreateAlliance
	// PenaltyDissolving: the clan's dissolution is pending.
	PenaltyDissolving
	// PenaltyNoDissolve: the clan may not be dissolved now; it has no
	// expiry.
	PenaltyNoDissolve
)

// Penalty is one line of a penalty report: what is refused, and until when
// in epoch milliseconds.
type Penalty struct {
	Kind   PenaltyKind
	Expiry int64
}

// PenaltyReport lists, in display order, the penalties c and its clan are
// under at nowMs.
//
// A clan registered on a castle siege may not be dissolved either; no siege
// takes registrations yet, so no clan is registered on one (#3211).
func (s *Service) PenaltyReport(c *player.Character, nowMs int64) []Penalty {
	var out []Penalty
	if t := c.ClanJoinExpiryTime(); t > nowMs {
		out = append(out, Penalty{PenaltyJoinClan, t})
	}
	if t := c.ClanCreateExpiryTime(); t > nowMs {
		out = append(out, Penalty{PenaltyCreateClan, t})
	}
	cl, ok := s.ClanOf(c)
	if !ok {
		return out
	}
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	if cl.charPenaltyExpiry > nowMs {
		out = append(out, Penalty{PenaltyInviteMember, cl.charPenaltyExpiry})
	}
	if cl.allyPenaltyType != 0 && cl.allyPenaltyExpiry > nowMs {
		switch cl.allyPenaltyType {
		case allyPenaltyClanLeft, allyPenaltyClanDismissed:
			out = append(out, Penalty{PenaltyJoinAlliance, cl.allyPenaltyExpiry})
		case allyPenaltyDismissClan:
			out = append(out, Penalty{PenaltyInviteAllyMember, cl.allyPenaltyExpiry})
		case allyPenaltyDissolveAlly:
			out = append(out, Penalty{PenaltyCreateAlliance, cl.allyPenaltyExpiry})
		}
	}
	if cl.dissolvingExpiry > nowMs {
		out = append(out, Penalty{PenaltyDissolving, cl.dissolvingExpiry})
	}
	if cl.allyID != 0 || len(cl.wars) > 0 || cl.castleID > 0 || cl.hallID > 0 {
		out = append(out, Penalty{Kind: PenaltyNoDissolve})
	}
	return out
}
