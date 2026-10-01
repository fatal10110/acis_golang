package player

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// partyBonus is the party exp/sp multiplier by the number of members that
// qualify for a share, up to a full party of nine.
var partyBonus = [...]float64{1, 1, 1.30, 1.39, 1.50, 1.54, 1.58, 1.63, 1.67, 1.71}

// PartyKillPool returns a party's pooled exp and sp for a kill. The pool is
// the party's damage share of the victim's reward at the party's level
// difference, as one attacker's would be, scaled once more by that share
// when other attackers dealt part of the damage.
func PartyKillPool(expReward, spReward, partyDamage, totalDamage float64, levelDiff int) (exp int64, sp int) {
	exp, sp = KillRewardExpAndSp(expReward, spReward, partyDamage, totalDamage, levelDiff)
	share := 1.0
	if partyDamage < totalDamage {
		share = partyDamage / totalDamage
	}
	return commons.JavaLong(float64(exp) * share), int(commons.JavaInt(float64(sp) * share))
}

// PartyXPCutoff is how a party decides which of its rewarded members are
// close enough to its top level to share a kill's exp and sp.
type PartyXPCutoff uint8

// Party exp cutoff methods.
const (
	// PartyXPCutoffLevel keeps members at most CutoffLevel levels under the
	// party's level.
	PartyXPCutoffLevel PartyXPCutoff = iota
	// PartyXPCutoffPercentage keeps members whose squared level is at least
	// CutoffPercent percent of the members' summed squared levels.
	PartyXPCutoffPercentage
	// PartyXPCutoffAuto keeps members whose squared level outweighs the
	// bonus the last member to join would add.
	PartyXPCutoffAuto
	// PartyXPCutoffNone keeps every rewarded member.
	PartyXPCutoffNone
	// PartyXPCutoffUnknown is a configured method none of the others name:
	// no member qualifies.
	PartyXPCutoffUnknown
)

// ParsePartyXPCutoff reads a configured cutoff method, ignoring case.
func ParsePartyXPCutoff(s string) PartyXPCutoff {
	switch strings.ToLower(s) {
	case "level":
		return PartyXPCutoffLevel
	case "percentage":
		return PartyXPCutoffPercentage
	case "auto":
		return PartyXPCutoffAuto
	case "none":
		return PartyXPCutoffNone
	}
	return PartyXPCutoffUnknown
}

// PartyXPRules are the configured rules for sharing a kill inside a party.
type PartyXPRules struct {
	Cutoff        PartyXPCutoff
	CutoffLevel   int
	CutoffPercent float64
	RateXP        float64
	RateSP        float64
}

// PartyRewardMember is one rewarded party member as the split reads it: its
// level, and its servitor's exp penalty (zero without a servitor).
type PartyRewardMember struct {
	Level           int
	ServitorPenalty float32
}

// PartyShare is one rewarded member's part of a party kill. A member that
// does not qualify under the cutoff is still paid, with zero exp and sp.
type PartyShare struct {
	Exp   int64
	Sp    int
	Valid bool
}

// Shares splits a party's pooled exp and sp among its rewarded members, in
// their order. topLevel is the party level the cutoff measures against. The
// pool grows by the party bonus for the number of qualifying members and by
// the party rates, then each qualifying member takes its squared level's
// part of the qualifying members' summed squared levels, less its
// servitor's penalty.
func (r PartyXPRules) Shares(xpReward int64, spReward int, topLevel int, members []PartyRewardMember) []PartyShare {
	shares := make([]PartyShare, len(members))
	valid := 0
	sqLevelSum := 0
	for _, m := range members {
		sqLevelSum += m.Level * m.Level
	}
	for i, m := range members {
		sqLevel := m.Level * m.Level
		switch r.Cutoff {
		case PartyXPCutoffLevel:
			shares[i].Valid = topLevel-m.Level <= r.CutoffLevel
		case PartyXPCutoffPercentage:
			shares[i].Valid = float64(sqLevel*100) >= float64(sqLevelSum)*r.CutoffPercent
		case PartyXPCutoffAuto:
			size := min(max(len(members), 1), 9)
			shares[i].Valid = float64(sqLevel) >= float64(sqLevelSum)*(1-1/(1+partyBonus[size]-partyBonus[size-1]))
		case PartyXPCutoffNone:
			shares[i].Valid = true
		}
		if shares[i].Valid {
			valid++
		}
	}

	// A command channel can reward more than nine members; the bonus stops
	// at a full party's.
	bonus := partyBonus[min(valid, 9)]
	xpReward = commons.JavaLong(float64(xpReward) * (bonus * r.RateXP))
	spReward = int(commons.JavaInt(float64(spReward) * (bonus * r.RateSP)))

	validSqLevelSum := 0
	for i, m := range members {
		if shares[i].Valid {
			validSqLevelSum += m.Level * m.Level
		}
	}
	for i, m := range members {
		if !shares[i].Valid {
			continue
		}
		// The penalty is taken from one in single precision, as the
		// servitor stores it.
		part := float64(m.Level*m.Level) / float64(validSqLevelSum) * float64(1-m.ServitorPenalty)
		shares[i].Exp = commons.JavaRound(float64(xpReward) * part)
		shares[i].Sp = int(commons.JavaInt(float64(spReward) * part))
	}
	return shares
}
