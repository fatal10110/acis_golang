package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/observer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
)

// showObserverGroups opens broadcasting tower f's list of viewpoint
// groups, in place of its chat window. It reports false for an NPC that
// is no tower.
func showObserverGroups(live *livePlayer, f *npc.Folk) bool {
	groups := f.ObserverGroups()
	if groups == nil {
		return false
	}
	sendValidatedHTML(live, f.ObjectID(), observer.GroupsWindow(f.ObjectID(), groups), 0)
	return true
}

// observeGroup opens the viewpoints of group id, linked through f. An
// unknown group answers nothing of its own.
func (l *GameClientLink) observeGroup(live *livePlayer, f *npc.Folk, id int) {
	locs, ok := l.observers.Group(id)
	if !ok {
		return
	}
	sendValidatedHTML(live, f.ObjectID(), observer.LocationsWindow(f.ObjectID(), locs), 0)
}

// observe sends live to watch from viewpoint id. A castle viewpoint
// refuses a player with a summon out and is closed outside its castle's
// siege; any other viewpoint turns a player with a summon away without a
// word. A player in combat is refused. An unknown viewpoint answers
// nothing.
func (l *GameClientLink) observe(live *livePlayer, id int) {
	loc, ok := l.observers.Location(id)
	if !ok {
		return
	}
	hasSummon := l.liveSummon(live) != nil
	if loc.CastleID > 0 {
		if hasSummon {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoObserveWithPet))
			return
		}
		// ponytail: a castle viewpoint opens while that castle's siege
		// is in progress (#3375); it is always closed until then.
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOnlyViewSiege))
		return
	}
	if hasSummon {
		return
	}
	if live.InCombat() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotObserveInCombat))
		return
	}
	// A player waiting for an Olympiad match is turned away silently.
	if l.olympiadRegistered(live) {
		return
	}
	l.enterObserverMode(live, loc)
}

// enterObserverMode charges live the viewpoint's fee, then sends its summon
// away, takes it out of its party, stands it up and hides it, unable to
// act or be hurt, at the viewpoint. The position it leaves is kept for its
// return. A player short of the fee is told so and stays.
func (l *GameClientLink) enterObserverMode(live *livePlayer, loc observer.Location) {
	if loc.Cost > 0 && !reduceAdena(live, loc.Cost) {
		return
	}
	l.dropAllSummons(live)
	if l.parties != nil {
		l.applyPartyNotices(l.parties.Leave(live, party.Expelled))
	}
	live.Character.StandUp()
	live.SetSavedLocation(live.CurrentLocation())
	live.SetInvul(true)
	live.SetInvisible(true)
	live.SetParalyzed(true)
	// The player cannot act any more: the abort is answered as a
	// teleport's is, then the teleport aborts once more.
	live.abortAll(true)
	l.teleportLivePlayer(live, loc.Location, 0)
	live.SendFrame(serverpackets.FrameObserverStart(loc.Location, loc.Yaw, loc.Pitch))
}

// observerReturn answers ObserverReturn: an observer goes back to where it
// left; anyone else is ignored. So is an observer whose jump to the
// viewpoint its client has not yet reported landed: the jump back would be
// refused while that one is in flight, leaving it at the viewpoint as a
// plain player.
func (l *GameClientLink) observerReturn(live *livePlayer) {
	if live.ObserverMode() && !live.Teleporting() {
		l.leaveObserverMode(live)
	}
}

// leaveObserverMode brings observer live back to the position it left to
// watch, visible, vulnerable and free to act again.
func (l *GameClientLink) leaveObserverMode(live *livePlayer) {
	live.tryToIdle(live.DenyAIAction())
	l.clearLiveTarget(live)
	live.SetInvisible(false)
	live.SetInvul(false)
	live.SetParalyzed(false)
	saved, _ := live.SavedLocation()
	live.SendFrame(serverpackets.FrameObserverEnd(saved))
	l.teleportLivePlayer(live, saved, 0)
	live.ClearSavedLocation()
}

// dropAllSummons sends live's summon away and ends its cubics, telling
// its observers once.
func (l *GameClientLink) dropAllSummons(live *livePlayer) {
	if s := l.liveSummon(live); s != nil {
		s.Unsummon()
	}
	l.stopAllCubics(live)
}

// refuseObserverAction answers an Action request of an observer: observers
// cannot take part in anything.
func refuseObserverAction(live *livePlayer) bool {
	if !live.ObserverMode() {
		return false
	}
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageObserversCannotParticipate))
	live.SendFrame(serverpackets.FrameActionFailed())
	return true
}

// refuseObserverAttack answers an AttackRequest of an observer. One unable
// to act, as a viewpoint's observer is, is only released; any other is
// told observers cannot take part.
func refuseObserverAttack(live *livePlayer) bool {
	if !live.ObserverMode() {
		return false
	}
	if !liveOutOfControl(live) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageObserversCannotParticipate))
	}
	live.SendFrame(serverpackets.FrameActionFailed())
	return true
}

// refuseObserverActionUse answers an action-bar command of an observer.
// One unable to act, as a viewpoint's observer is, is only released; any
// other is told observers cannot take part, with no release.
func refuseObserverActionUse(live *livePlayer) bool {
	if !live.ObserverMode() {
		return false
	}
	if liveOutOfControl(live) {
		live.SendFrame(serverpackets.FrameActionFailed())
	} else {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageObserversCannotParticipate))
	}
	return true
}
