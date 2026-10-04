package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/hero"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// socialActionHero is the social action a player claiming the hero status
// plays.
const socialActionHero = 16

// heroClaimLevel is the lowest level that may claim the hero status.
const heroClaimLevel = 76

// heroClaimReputation is the reputation a clan of level 5 or more gains
// when a member claims the hero status.
const heroClaimReputation = 1000

// heroClaimClanLevel is the lowest clan level rewarded for a member
// claiming the hero status.
const heroClaimClanLevel = 5

// A GameClientLink reaches the heroes online for an election.
var _ hero.Online = (*GameClientLink)(nil)

// restoreHeroStatus makes c, a character just selected, a hero when it
// claimed the hero status of the running era, with the hero skills on its
// base class; nothing is sent.
func (l *GameClientLink) restoreHeroStatus(c *player.Character) {
	if l.heroes == nil || !l.heroes.IsActive(c.ID) {
		return
	}
	c.SetHero(true)
	if c.SubclassActive() || l.skills == nil {
		return
	}
	if err := l.skills.GrantTransientSkills(c, modelskill.HeroSkills()); err != nil {
		l.log.Error().Err(err).Int32("object_id", c.ID).Msg("give hero skills")
	}
}

// setHero gives live hero status, or takes it, on live's queue: the hero
// skills come with the status only on the base class and go otherwise,
// then live gets its skill list.
func (l *GameClientLink) setHero(live *livePlayer, isHero bool) {
	if l.skills != nil {
		if isHero && !live.SubclassActive() {
			if err := l.skills.GrantTransientSkills(live.Character, modelskill.HeroSkills()); err != nil {
				l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("give hero skills")
			}
		} else {
			for _, ref := range modelskill.HeroSkills() {
				l.removeLiveSkill(live, int(ref.ID), false)
			}
		}
	}
	live.SetHero(isHero)
	live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
}

// Dethrone implements hero.Online: the player objectID, when online, loses
// its hero status on its own queue, takes off every hero item it wears and
// loses every hero item it carries, then shows the change around.
func (l *GameClientLink) Dethrone(objectID int32) {
	live, ok := l.livePlayerByID(objectID)
	if !ok {
		return
	}
	postLive(live, func() { l.dethrone(live) })
}

// dethrone runs Dethrone on live's queue.
func (l *GameClientLink) dethrone(live *livePlayer) {
	l.setHero(live, false)
	if inv := live.Inventory(); inv != nil && l.inventory != nil {
		for _, inst := range inv.PaperdollItems() {
			if tmpl, ok := inv.Templates().Get(inst.TemplateID); ok && tmpl.HeroItem() && inst.Equipped() {
				l.toggleEquipItem(live, inv, inst, tmpl, true)
			}
		}
		for _, inst := range inv.Items() {
			if tmpl, ok := inv.Templates().Get(inst.TemplateID); ok && tmpl.HeroItem() && !inst.Equipped() {
				destroyHeldItem(live, inst, inst.Snapshot().Count)
			}
		}
	}
	l.broadcastCharacterInfo(live)
}

// sendHeroList shows live the heroes of the running era.
func (l *GameClientLink) sendHeroList(live *livePlayer) {
	var heroes []hero.Hero
	if l.heroes != nil {
		heroes = l.heroes.Heroes()
	}
	entries := make([]serverpackets.HeroListEntry, len(heroes))
	for i, h := range heroes {
		entries[i] = serverpackets.HeroListEntry{
			Name: h.Name, ClassID: int32(h.ClassID), ClanName: h.ClanName, ClanCrest: h.ClanCrest,
			AllyName: h.AllyName, AllyCrest: h.AllyCrest, Count: int32(h.Count),
		}
	}
	live.SendFrame(serverpackets.FrameExHeroList(entries))
}

// claimHero has live, elected hero and not yet one, claim the hero status
// at a Monument of Heroes. Only the base class at level 76 or more may
// claim it; anyone else is told so. The claim plays the hero's social
// action, shows live around with the status, and gives a clan of level 5
// or more reputation, telling its members. Anyone not elected gets
// nothing.
func (l *GameClientLink) claimHero(live *livePlayer) {
	if l.heroes == nil || !l.heroes.IsInactive(live.ObjectID()) {
		return
	}
	if live.SubclassActive() || live.Level() < heroClaimLevel {
		sendText(live, "You may only become an hero on a main class whose level is 75 or more.")
		return
	}
	h, ok := l.heroes.Activate(live.ObjectID())
	if !ok {
		return
	}
	l.setHero(live, true)
	id := live.ObjectID()
	l.broadcastLiveFrame(live, func() wire.Frame { return serverpackets.FrameSocialAction(id, socialActionHero) })
	l.broadcastCharacterInfo(live)
	cl, ok := l.clanService().ClanOf(live.Character)
	if !ok || cl.Level() < heroClaimClanLevel {
		return
	}
	if change, changed := l.clanService().AddReputation(cl, heroClaimReputation); changed {
		l.sendReputationChange(cl, change, live)
	}
	name := h.Name
	l.broadcastToClanQueued(cl, live,
		func() wire.Frame { return framePledgeShowInfoUpdate(cl) },
		func() wire.Frame {
			return serverpackets.FrameSystemMessageStringNumber(serverpackets.SystemMessageClanMemberS1BecameHeroAndGainedS2ReputationPoints, name, heroClaimReputation)
		})
}
