package manager

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
)

// RewardParty is the group a party member shares a kill with: its party's
// members, or its command channel's while the party is in one, with the
// channel's level.
type RewardParty struct {
	Members      []*player.Character
	InChannel    bool
	ChannelLevel int
}

// RewardParties resolves the group a rewarded player shares a kill with.
type RewardParties interface {
	// RewardParty returns the player's group; ok is false outside a party.
	RewardParty(playerID int32) (group RewardParty, ok bool)
}

func (d *deathRewards) rewardGroup(p *player.Character) (RewardParty, bool) {
	if d.config.Parties == nil {
		return RewardParty{}, false
	}
	return d.config.Parties.RewardParty(p.ObjectID())
}

// grantParty shares the kill with attacker's group. Every living member in
// party range is rewarded, attacker or not; the party's damage is what its
// rewarded members dealt, and its level the highest rewarded member's (the
// channel's, in a command channel). Every living member's own entry is
// spent, in range or not. The pooled exp takes the attacker's overhit
// bonus, and the members split it by the party rules; a member the cutoff
// leaves out is still paid nothing.
func (d *deathRewards) grantParty(attacker *player.Character, group RewardParty, entries []playerRewardEntry, spent map[int32]bool, summonDamage map[int32]float64, totalDamage float64) {
	var (
		partyDamage float64
		partyLevel  int
		rewarded    []*player.Character
	)
	ownDamage := map[int32]float64{}
	for _, m := range group.Members {
		if m.Dead() {
			continue
		}
		inRange := d.inPartyRange(m)
		if inRange {
			rewarded = append(rewarded, m)
			if m.Level() > partyLevel {
				partyLevel = m.Level()
				if group.InChannel {
					partyLevel = group.ChannelLevel
				}
			}
		}
		id := m.ObjectID()
		i := slices.IndexFunc(entries, func(e playerRewardEntry) bool { return e.actor.ObjectID() == id })
		if i < 0 || spent[id] {
			continue
		}
		if inRange {
			partyDamage += entries[i].damage
		}
		spent[id] = true
		ownDamage[id] = entries[i].damage
	}

	rewardExp, rewardSp := d.ratedReward()
	exp, sp := player.PartyKillPool(rewardExp, rewardSp, partyDamage, totalDamage, partyLevel-d.tmpl.Level)
	if d.hostile.OverhitValid(attacker) {
		attacker.NotifyOverHit()
		exp += d.hostile.OverhitBonus(exp)
	}
	if partyDamage <= 0 {
		return
	}

	members := make([]player.PartyRewardMember, len(rewarded))
	owns := make([]*summon.Actor, len(rewarded))
	for i, m := range rewarded {
		owns[i] = d.summonOf(m)
		members[i].Level = m.Level()
		if owns[i] != nil && !owns[i].IsPet() {
			members[i].ServitorPenalty = owns[i].ExpPenalty()
		}
	}
	shares := d.config.PartyXP.Shares(exp, sp, partyLevel, members)
	for i, m := range rewarded {
		if m.Dead() {
			continue
		}
		if !shares[i].Valid {
			// Still a reward: the member sees its status and a zero gain.
			m.RewardExpAndSp(d.config.PlayerLevels, 0, 0)
			continue
		}
		m.UpdateKarmaLoss(d.config.PlayerLevels, shares[i].Exp)
		d.pay(m, owns[i], shares[i].Exp, shares[i].Sp, ownDamage[m.ObjectID()], summonDamage)
	}
}
