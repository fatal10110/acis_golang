package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// dissolveClan runs live's request to dissolve its clan. Granted, the clan
// is either marked for dissolution, which its members see in a refreshed
// clan header, or, with no delay configured, destroyed at once; either way
// the leader then loses a full death's experience.
func (l *GameClientLink) dissolveClan(live *livePlayer) {
	cl, result := l.clanService().RequestDissolve(live.Character, time.Now())
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

// destroyClan destroys cl (clan.Service.Destroy; due as there): every
// member in the world is told the clan dispersed, then leaves it as a
// member who withdrew does, its clan tab cleared; the clan warehouse's
// items are destroyed. actor is the player whose queue the caller runs on,
// nil for none; its own leave runs inline, every other member's on its own
// queue.
func (l *GameClientLink) destroyClan(cl *clan.Clan, actor *livePlayer, due bool) {
	now := time.Now()
	online := func(id int32) *player.Character {
		if live, ok := l.livePlayerByID(id); ok {
			return live.Character
		}
		return nil
	}
	out, ok := l.clanService().Destroy(cl, due, online, now)
	if !ok {
		return
	}
	l.destroyClanWarehouse(cl.ID())
	type leaver struct {
		live *livePlayer
		m    clan.Member
	}
	var leavers []leaver
	for _, m := range out.Members {
		if !m.Online {
			continue
		}
		if live, ok := l.livePlayerByID(m.ObjectID); ok {
			leavers = append(leavers, leaver{live, m})
		}
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
			l.clanService().ApplyDispersed(lv.live.Character, lv.m, lv.m.ObjectID == out.LeaderID, now)
			l.sendLeftClan(lv.live, cl)
		})
	}
}

// destroyClanWarehouse destroys every item in the warehouse of the
// dissolved clan clanID, restoring it first when no member opened it, and
// deletes their rows; the warehouse is then forgotten.
func (l *GameClientLink) destroyClanWarehouse(clanID int32) {
	defer l.clanWarehouses.forget(clanID)
	wh, err := l.clanWarehouse(clanID)
	if err != nil {
		l.log.Error().Err(err).Int32("clan_id", clanID).Msg("warehouse: restore dissolved clan warehouse")
		return
	}
	end := l.itemInstances.BeginOperation(clanID)
	defer end()
	var persist []invops.Persist
	for _, inst := range wh.Items() {
		objectID := inst.ObjectID
		if wh.DestroyAll(inst) != nil {
			persist = append(persist, invops.Delete(clanID, objectID))
		}
	}
	l.applyPersistActions(persist)
	end()
	wh.ReleasePersistence()
}
