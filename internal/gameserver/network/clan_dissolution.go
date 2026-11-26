package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// dissolveClan runs live's request to dissolve its clan. Granted, the clan
// is either marked for dissolution, which its members see in a refreshed
// clan header, or, with no delay configured, destroyed at once; either way
// the leader then loses a full death's experience.
func (l *GameClientLink) dissolveClan(live *livePlayer) {
	// A clan destroyed at once destroys its warehouse's items, so a
	// warehouse no one has used yet is read off the queue first.
	if cl, ok := l.clanService().ClanOf(live.Character); ok && !l.clanWarehouseReady(live, cl.ID(), func() { l.dissolveClan(live) }) {
		return
	}
	cl, result := l.clanService().RequestDissolve(live.Character, time.Now(), l.sieges)
	var msg int
	switch result {
	case clan.DissolveNotLeader:
		msg = serverpackets.SystemMessageNotAuthorizedToDoThat
	case clan.DissolveInAlliance:
		msg = serverpackets.SystemMessageCannotDisperseClansInAlly
	case clan.DissolveAtWar:
		msg = serverpackets.SystemMessageCannotDissolveWhileInWar
	case clan.DissolveOwnsResidence:
		msg = serverpackets.SystemMessageCannotDissolveOwningResidence
	case clan.DissolveSiegeRegistered:
		msg = serverpackets.SystemMessageCannotDissolveSiegeRegistered
	case clan.DissolveInSiege:
		msg = serverpackets.SystemMessageCannotDissolveWhileInSiege
	case clan.DissolveInProgress:
		msg = serverpackets.SystemMessageDissolutionInProgress
	case clan.DissolveScheduled:
		l.broadcastToClan(cl, 0, func() wire.Frame { return framePledgeShowInfoUpdate(cl) })
	case clan.DissolveNow:
		l.destroyClan(cl, live, false)
	}
	if msg != 0 {
		live.SendFrame(serverpackets.FrameSystemMessage(msg))
		return
	}
	live.Character.ApplyDeathPenalty()
}

// recoverClan runs live's request to call off its clan's dissolution; the
// members see the clan header refreshed.
func (l *GameClientLink) recoverClan(live *livePlayer) {
	cl, result := l.clanService().RecoverClan(live.Character)
	switch result {
	case clan.RecoverNotLeader:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAuthorizedToDoThat))
	case clan.RecoverNothing:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoRequestsToDisperse))
	case clan.Recovered:
		l.broadcastToClan(cl, 0, func() wire.Frame { return framePledgeShowInfoUpdate(cl) })
	}
}

// DissolveDue destroys cl, whose scheduled dissolution came due, unless it
// was called off meanwhile. It runs on the clan dissolution queue.
func (l *GameClientLink) DissolveDue(cl *clan.Clan) {
	l.destroyClan(cl, nil, true)
}

// destroyClan destroys cl (clan.Service.Destroy; due as there): the clan
// leaves every siege it was registered on, a siegable hall it owned freed
// (clanhall.Halls.ForgetClan); every member in the world is
// told the clan dispersed, then leaves it as a member who withdrew does,
// its siege state cleared and its clan tab cleared, the leader losing its
// siege skills; the clan warehouse's items are destroyed. An offline
// member of a clan holding a castle has its circlets put back in its
// inventory (unequipLeaverCirclets). A member in the world keeps them on:
// its items would pass a check made while it still holds the clan, but
// here the clan has already left the table, so a check would wrongly
// take them off. actor is the player whose queue the caller
// runs on, nil for none; its own leave runs inline, every other member's
// on its own queue.
func (l *GameClientLink) destroyClan(cl *clan.Clan, actor *livePlayer, due bool) {
	now := time.Now()
	out, ok := l.clanService().Destroy(cl, due, l.onlineCharacter, now)
	if !ok {
		return
	}
	l.sieges.Forget(cl.ID())
	l.halls.ForgetClan(cl)
	l.destroyClanWarehouse(cl.ID())
	type leaver struct {
		live *livePlayer
		m    clan.Member
	}
	var leavers []leaver
	for _, m := range out.Members {
		live, ok := l.livePlayerByID(m.ObjectID)
		if !m.Online || !ok {
			// A member whose selection already loaded what it wears keeps
			// it, and takes the leave as it enters the world. So does one
			// listed online but out of the world on its way out, whose last
			// inventory save stored it worn: the leave was handed to it
			// (clan.Service.Leave).
			if !m.Online && !m.Entering {
				l.unequipLeaverCirclets(cl, m.ObjectID)
			}
			continue
		}
		leavers = append(leavers, leaver{live, m})
	}
	dispersed := func() wire.Frame {
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageClanHasDispersed)
	}
	broadcastFrame(dispersed, func(send func(frameReceiver)) {
		for _, lv := range leavers {
			send(lv.live)
		}
	})
	for _, lv := range leavers {
		onMemberQueue(actor, lv.live, func() {
			leader := lv.m.ObjectID == out.LeaderID
			l.clanService().ApplyDispersed(lv.live.Character, lv.m, leader, now)
			if leader {
				l.takeSiegeSkills(lv.live)
			}
			l.sendLeftClan(lv.live, cl)
		})
	}
}

// destroyClanWarehouse destroys every item in the warehouse of the
// dissolved clan clanID, restoring it first when no member opened it, and
// deletes their rows; the warehouse is forgotten inside the same item
// operation, so a member's deposit or withdrawal waiting on that operation
// finds it gone (storeStillOpen).
func (l *GameClientLink) destroyClanWarehouse(clanID int32) {
	defer l.clanWarehouses.Forget(clanID)
	wh, err := l.clanWarehouse(clanID)
	if err != nil {
		l.log.Error().Err(err).Int32("clan_id", clanID).Msg("warehouse: restore dissolved clan warehouse")
		return
	}
	end := l.belongings.itemInstances.BeginOperation(clanID)
	defer end()
	var persist []invops.Persist
	for _, inst := range wh.Items() {
		objectID := inst.ObjectID
		if wh.DestroyAll(inst) != nil {
			persist = append(persist, invops.Delete(clanID, objectID))
		}
	}
	l.applyPersistActions(persist)
	l.clanWarehouses.Forget(clanID)
	end()
	wh.ReleasePersistence()
}
