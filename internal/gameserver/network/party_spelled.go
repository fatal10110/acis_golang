package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// updateEffectIcons runs live's effect icon pass: live's own icon list,
// then, in a party, the icon list every member sees of live. The
// ExOlympiadSpelledInfo an Olympiad match's observers get from the same
// pass waits for Olympiad matches and observers (#1268, #219).
func (l *GameClientLink) updateEffectIcons(live *livePlayer) {
	l.updateLiveAbnormalEffect(live)
	if l.parties == nil {
		return
	}
	if view, ok := l.parties.View(live.ObjectID()); ok {
		sendPartySpelled(live, view.Members)
	}
}

// sendPartySpelled shows to, live's party, the icon list of live's
// effects.
func sendPartySpelled(live *livePlayer, to []*livePlayer) {
	effects := partyIconEffects(live.EffectList(), live.Queue().Now())
	sendToMembers(to, func() wire.Frame {
		return serverpackets.FramePartySpelled(serverpackets.PartySpelledPlayer, live.ObjectID(), effects)
	})
}

// sendSummonPartySpelled shows the icon list of a's effects to its owner's
// party, or to its owner alone outside one.
func sendSummonPartySpelled(a *summon.Actor) {
	owner, ok := liveSummonOwner(a)
	if !ok {
		return
	}
	to := []*livePlayer{owner}
	if l := owner.link; l != nil && l.parties != nil {
		if view, ok := l.parties.View(owner.ObjectID()); ok {
			to = view.Members
		}
	}
	kind := serverpackets.PartySpelledServitor
	if a.IsPet() {
		kind = serverpackets.PartySpelledPet
	}
	effects := partyIconEffects(a.EffectList(), a.Queue().Now())
	sendToMembers(to, func() wire.Frame {
		return serverpackets.FramePartySpelled(kind, a.ObjectID(), effects)
	})
}

// refreshSummonPartySpelled shows a's icon list again after a PetInfo,
// which clears it on the client. A summon that never held an effect has
// none to show.
func refreshSummonPartySpelled(a *summon.Actor) {
	if a.EffectList().HasHeld() {
		sendSummonPartySpelled(a)
	}
}

func partyIconEffects(list *effect.List, now time.Time) []serverpackets.AbnormalStatusEffect {
	entries := list.PartyIconEntries(now)
	effects := make([]serverpackets.AbnormalStatusEffect, len(entries))
	for i, e := range entries {
		effects[i] = serverpackets.AbnormalStatusEffect{SkillID: e.ID, Level: int32(e.Level), DurationMillis: int(e.Duration)}
	}
	return effects
}
