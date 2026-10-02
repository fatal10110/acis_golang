package network

import (
	"context"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	tradebook "github.com/fatal10110/acis_golang/internal/gameserver/trade"
)

// partyView is a copy of one live players' party.
type partyView = party.View[*livePlayer]

// duelRegistry holds the duels live players fight.
type duelRegistry = duel.Manager[*livePlayer]

var _ duel.Player = (*livePlayer)(nil)

// dispatchDuel decodes and runs one extended duel packet. It reports false
// once the connection has been closed for a malformed packet.
func (l *GameClientLink) dispatchDuel(client *Client, live *livePlayer, second uint16, payload []byte) bool {
	switch second {
	case clientpackets.OpcodeRequestDuelStart:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestDuelStart, l.requestDuelStart)
	case clientpackets.OpcodeRequestDuelAnswerStart:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestDuelAnswerStart, l.requestDuelAnswerStart)
	case clientpackets.OpcodeRequestDuelSurrender:
		// The request carries no body. A player in no duel surrenders
		// nothing and hears nothing, as the menu action waits on no
		// answer; the duel's end answers a surrender.
		if live != nil && l.duels != nil {
			onLive(live, func() { l.duels.Surrender(live) })
		}
	}
	return true
}

// requestDuelStart challenges the named player to a duel, or its party to
// a party duel; a party duel's challenge goes to the party's leader.
func (l *GameClientLink) requestDuelStart(live *livePlayer, req clientpackets.RequestDuelStart) {
	target, ok := l.livePlayerByName(req.Target)
	if !ok || target.ObjectID() == live.ObjectID() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoOpponentForDuel))
		return
	}
	if _, ok := l.duelRefusal(live); !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageUnableToRequestDuel))
		return
	}
	if reason, ok := l.duelRefusal(target); !ok {
		live.SendFrame(serverpackets.FrameSystemMessageString(reason, target.Name))
		return
	}
	if !inDuelRange(live, target) {
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1CannotReceiveDuelTooFar, target.Name))
		return
	}
	if !req.Party {
		if l.tradeBook().ProcessingRequest(target.ObjectID()) {
			live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1IsBusyTryLater, target.Name))
			return
		}
		l.leavePartyMatch(live)
		l.leavePartyMatch(target)
		if !l.challengeToDuel(live, target, tradebook.KindDuel) {
			return
		}
		target.SendFrame(serverpackets.FrameExDuelAskStart(live.Name, false))
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1ChallengedToDuel, target.Name))
		target.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1ChallengedYouToDuel, live.Name))
		return
	}

	own, targetParty, msg, ok := l.partyDuelSides(live, target)
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(msg))
		return
	}
	leader := targetParty.Leader
	if l.tradeBook().ProcessingRequest(leader.ObjectID()) {
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1IsBusyTryLater, leader.Name))
		return
	}
	l.prepareDuelParties(own.Members, targetParty.Members)
	if !l.challengeToDuel(live, leader, tradebook.KindPartyDuel) {
		return
	}
	leader.SendFrame(serverpackets.FrameExDuelAskStart(live.Name, true))
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1PartyChallengedToDuel, leader.Name))
	target.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1PartyChallengedYourParty, live.Name))
}

// challengeToDuel records live's challenge of target, of kind. A target
// that became busy since it was checked is reported busy and challenged by
// no one.
func (l *GameClientLink) challengeToDuel(live, target *livePlayer, kind tradebook.RequestKind) bool {
	if l.tradeBook().Invite(kind, live.ObjectID(), target.ObjectID(), false).Status != tradebook.RequestStarted {
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1IsBusyTryLater, target.Name))
		return false
	}
	return true
}

// partyDuelSides checks the two parties of a party duel live, the
// challenger's side, fights against target's: live leads a party target is
// not in, target is in a party, and every other member of both may duel.
// It returns both parties, or the message refusing the duel to whoever
// asked for it.
func (l *GameClientLink) partyDuelSides(live, target *livePlayer) (own, other partyView, msg int, ok bool) {
	own, inParty := l.parties.View(live.ObjectID())
	if !inParty || own.Leader.ObjectID() != live.ObjectID() || l.parties.SameParty(live.ObjectID(), target.ObjectID()) {
		return own, other, serverpackets.SystemMessageUnableToRequestDuel, false
	}
	other, inParty = l.parties.View(target.ObjectID())
	if !inParty {
		return own, other, serverpackets.SystemMessageChallengedNotInParty, false
	}
	for _, m := range own.Members {
		if m.ObjectID() != live.ObjectID() {
			if _, ok := l.duelRefusal(m); !ok {
				return own, other, serverpackets.SystemMessageUnableToRequestDuel, false
			}
		}
	}
	for _, m := range other.Members {
		if m.ObjectID() != target.ObjectID() {
			if _, ok := l.duelRefusal(m); !ok {
				return own, other, serverpackets.SystemMessageOpposingPartyUnableToDuel, false
			}
		}
	}
	return own, other, 0, true
}

// prepareDuelParties takes both parties of a party duel out of their
// command channels, then every member out of party matching.
func (l *GameClientLink) prepareDuelParties(a, b []*livePlayer) {
	for _, side := range [][]*livePlayer{a, b} {
		if len(side) > 0 {
			l.applyPartyNotices(l.parties.LeaveChannelSilently(side[0].ObjectID()))
		}
	}
	for _, side := range [][]*livePlayer{a, b} {
		for _, m := range side {
			l.leavePartyMatch(m)
		}
	}
}

// requestDuelAnswerStart answers live's pending duel challenge. With no
// challenge pending, or its challenger gone, nothing answers: the dialog
// closed on the client when it answered. The duel is the kind the challenge
// asked for, whatever kind the answer names.
func (l *GameClientLink) requestDuelAnswerStart(live *livePlayer, req clientpackets.RequestDuelAnswerStart) {
	party := false
	requesterID, ok := l.tradeBook().TakeInvite(tradebook.KindDuel, live.ObjectID())
	if !ok {
		party = true
		if requesterID, ok = l.tradeBook().TakeInvite(tradebook.KindPartyDuel, live.ObjectID()); !ok {
			return
		}
	}
	requester, ok := l.livePlayerByID(requesterID)
	if !ok {
		return
	}
	if !req.Accepted {
		if party {
			requester.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOpposingPartyDeclinedDuel))
		} else {
			requester.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1DeclinedYourDuel, live.Name))
		}
		return
	}
	if reason, ok := l.duelRefusal(requester); !ok {
		live.SendFrame(serverpackets.FrameSystemMessageString(reason, requester.Name))
		return
	}
	if _, ok := l.duelRefusal(live); !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageUnableToRequestDuel))
		return
	}
	if !inDuelRange(requester, live) {
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1CannotReceiveDuelTooFar, requester.Name))
		return
	}
	if !party {
		l.leavePartyMatch(live)
		l.leavePartyMatch(requester)
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageYouAcceptedS1Duel, requester.Name))
		requester.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1AcceptedYourDuel, live.Name))
		l.beginDuel(live, requester, nil, nil, false)
		return
	}
	requesterParty, ownParty, msg, ok := l.partyDuelSides(requester, live)
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(msg))
		return
	}
	l.prepareDuelParties(requesterParty.Members, ownParty.Members)
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageYouAcceptedS1PartyDuel, requester.Name))
	requester.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1AcceptedYourPartyDuel, live.Name))
	l.beginDuel(live, requester, requesterParty.Members, ownParty.Members, true)
}

// duelRefusal reports whether p may join a duel, and otherwise the message,
// naming p, that tells why.
//
// A player on a boat is refused like a rider once boats exist (#229).
func (l *GameClientLink) duelRefusal(p *livePlayer) (msg int, ok bool) {
	res := p.ResourceValues()
	switch {
	case p.InCombat() || p.Jailed():
		return serverpackets.SystemMessageS1CannotDuelInBattle, false
	case p.AlikeDead() || res.CurrentHP < res.MaxHP*0.5 || res.CurrentMP < res.MaxMP*0.5:
		return serverpackets.SystemMessageS1CannotDuelHPOrMPBelowHalf, false
	case p.InDuel():
		return serverpackets.SystemMessageS1CannotDuelAlreadyDuelling, false
	case p.OlympiadMode():
		return serverpackets.SystemMessageS1CannotDuelOlympiad, false
	case p.CursedWeaponEquipped() || p.Karma() != 0 || p.PvPFlagged():
		return serverpackets.SystemMessageS1CannotDuelChaotic, false
	case p.Operating():
		return serverpackets.SystemMessageS1CannotDuelPrivateStore, false
	case p.Mounted():
		return serverpackets.SystemMessageS1CannotDuelRiding, false
	case p.Fishing():
		return serverpackets.SystemMessageS1CannotDuelFishing, false
	case duelProhibitedArea(p):
		return serverpackets.SystemMessageS1CannotDuelProhibitedArea, false
	}
	return 0, true
}

// duelProhibitedArea reports whether p stands where no duel may be fought:
// a PvP, peace, siege, water or no-restart zone.
func duelProhibitedArea(p *livePlayer) bool {
	if p.InDuelBlockedZone() || p.InWater() {
		return true
	}
	return p.zoneActor != nil && p.zoneActor.ZoneFlags().Has(zone.FlagNoRestart)
}

// inDuelRange reports whether a and b stand within duel.Range of each
// other.
func inDuelRange(a, b *livePlayer) bool {
	ax, ay, az := a.Position()
	bx, by, bz := b.Position()
	return location.In3DRadius(ax, ay, az, bx, by, bz, duel.Range)
}

// duelCondition is what a duel saved of one of its players when it began,
// to put back when it ends. It is owned by the player's queue.
type duelCondition struct {
	cp, hp, mp float64
	// at is where a party duel's player stood: the end brings it back.
	at     *location.Location
	skills skillstate.SaveState
}

// beginDuel starts the duel challenger fought against answerer, who
// accepted it on its own queue: all players of both sides join the duel,
// which ticks on a queue of its own, and each saves its condition. teamA
// and teamB are both parties of a party duel. A player another duel took
// meanwhile leaves the answerer unable to duel.
func (l *GameClientLink) beginDuel(answerer, challenger *livePlayer, teamA, teamB []*livePlayer, party bool) {
	if l.queues == nil || l.duels == nil {
		return
	}
	q := l.queues.NewQueue(fmt.Sprintf("duel-%d", challenger.ObjectID()))
	v, ok := l.duels.Begin(challenger, answerer, teamA, teamB, party, q.Now())
	if !ok {
		q.Close()
		answerer.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageUnableToRequestDuel))
		return
	}
	for _, m := range v.All() {
		if m == answerer {
			l.saveDuelCondition(m, party)
		} else {
			postLive(m, func() { l.saveDuelCondition(m, party) })
		}
	}
	if party {
		l.sendToDuel(v, func() wire.Frame {
			return serverpackets.FrameSystemMessage(serverpackets.SystemMessageTransportedToDuelSite)
		})
	}
	q.Every(duel.TickPeriod, func() {
		if !l.applyDuelStep(l.duels.Tick(v.ID, q.Now())) {
			q.Close()
		}
	})
}

// saveDuelCondition saves live's CP, HP, MP, effects and reuse timers, and,
// for a party duel, where it stands. The effects and timers are also
// written to the character's saved skill state, which a logout during the
// duel leaves as it is.
func (l *GameClientLink) saveDuelCondition(live *livePlayer, party bool) {
	res := live.ResourceValues()
	cond := &duelCondition{cp: res.CurrentCP, hp: res.CurrentHP, mp: res.CurrentMP}
	if party {
		at := live.CurrentLocation()
		cond.at = &at
	}
	if l.skills != nil {
		cond.skills = l.skills.DuelState(live.Character)
		st, skills, id := cond.skills, l.skills, live.ObjectID()
		l.persist.Enqueue(id, func() {
			ctx, cancel := context.WithTimeout(context.Background(), livePlayerDetachSaveTimeout)
			defer cancel()
			if err := skills.Save(ctx, st); err != nil {
				l.log.Error().Err(err).Int32("object_id", id).Msg("save duel skill state")
			}
		})
	}
	live.duelCondition = cond
}

// applyDuelStep shows one second of a duel and reports whether the duel
// still ticks.
func (l *GameClientLink) applyDuelStep(step duel.Step[*livePlayer]) bool {
	v := step.Duel
	switch step.Kind {
	case duel.StepGone:
		return false
	case duel.StepTeleport:
		l.teleportDuelParties(v)
	case duel.StepCountdown:
		l.sendToDuel(v, func() wire.Frame {
			return serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageDuelBeginsInS1Seconds, int32(step.Seconds))
		})
	case duel.StepStart:
		l.sendToDuel(v, func() wire.Frame { return serverpackets.FrameSystemMessage(serverpackets.SystemMessageLetTheDuelBegin) })
		l.startDuel(v)
	case duel.StepEnd:
		l.endDuel(v, step.Result, false)
		return false
	}
	return true
}

// sendToDuel sends each player of both sides of v, on its own queue, its
// own copy of one frame.
func (l *GameClientLink) sendToDuel(v duel.View[*livePlayer], build func() wire.Frame) {
	for _, m := range v.All() {
		postLive(m, func() {
			frame := build()
			m.SendFrame(frame)
		})
	}
}

// teleportDuelParties moves both parties of v to the arena, side by side.
func (l *GameClientLink) teleportDuelParties(v duel.View[*livePlayer]) {
	for side, team := range [][]*livePlayer{v.TeamA, v.TeamB} {
		dy := -150
		if side == 1 {
			dy = 150
		}
		for i, m := range team {
			at := location.Location{X: duel.Arena.X + 40*i - 180, Y: duel.Arena.Y + dy, Z: duel.Arena.Z}
			postLive(m, func() { l.teleportLivePlayer(m, at, 0) })
		}
	}
}

// startDuel starts v's fight: both sides get the duel window and its
// music, then each player takes its side's colour and shows itself in the
// other side's window.
func (l *GameClientLink) startDuel(v duel.View[*livePlayer]) {
	for _, m := range v.All() {
		postLive(m, func() {
			m.SendFrame(serverpackets.FrameExDuelReady(v.Party))
			m.SendFrame(serverpackets.FrameExDuelStart(v.Party))
			m.SendFrame(serverpackets.FramePlaySoundAt(serverpackets.Sound{Type: 1, File: duelStartMusic}))
		})
	}
	for side, team := range [][]*livePlayer{v.TeamA, v.TeamB} {
		colour, opponents := duel.TeamBlue, v.TeamB
		if side == 1 {
			colour, opponents = duel.TeamRed, v.TeamA
		}
		for _, m := range team {
			postLive(m, func() {
				l.prepareToDuel(m, colour)
				info := duelUserInfo(m)
				l.broadcastToMembers(opponents, func() wire.Frame { return serverpackets.FrameExDuelUpdateUserInfo(info) })
			})
		}
	}
}

// duelStartMusic is the music a duel's start plays.
const duelStartMusic = "B04_S01"

// prepareToDuel readies live to fight: its enchant and trade are cancelled,
// it fights in colour and shows it.
func (l *GameClientLink) prepareToDuel(live *livePlayer, colour duel.Team) {
	l.cancelActiveEnchant(live)
	l.cancelActiveTrade(live)
	live.SetDuelState(duel.Duelling)
	live.SetDuelTeam(colour)
	l.refreshDuelLook(live)
}

// refreshDuelLook shows live's duel colour, on itself and on its summon.
func (l *GameClientLink) refreshDuelLook(live *livePlayer) {
	l.broadcastCharacterInfo(live)
	if pet, ok := l.summonOf(live).(*summon.Actor); ok {
		l.refreshSummonAbnormalEffect(pet)
	}
}

// duelUserInfo is live as its opponents' duel window shows it.
func duelUserInfo(live *livePlayer) serverpackets.DuelUserInfo {
	res := live.ResourceValues()
	return serverpackets.DuelUserInfo{
		Name: live.Name, ObjectID: live.ObjectID(), ClassID: int32(live.ClassID()), Level: int32(live.Level()),
		HP: int32(res.CurrentHP), MaxHP: int32(res.MaxHP),
		MP: int32(res.CurrentMP), MaxMP: int32(res.MaxMP),
		CP: int32(res.CurrentCP), MaxCP: int32(res.MaxCP),
	}
}

// endDuel ends v with res. Unless a party edit cancelled it, its players
// stop fighting first, and the losing side bows when its leader is still
// online and defeated. Both sides are then told the result, and every
// player gets its condition back and leaves the duel. A cancelled
// one-on-one duel puts nothing back.
func (l *GameClientLink) endDuel(v duel.View[*livePlayer], res duel.Result, partyEdit bool) {
	all := v.All()
	if !partyEdit {
		for _, m := range all {
			postLive(m, func() { l.stopToFight(m) })
		}
		if leader, losers := duelLosers(v, res); leader != nil && !leader.Departed() && leader.DuelState() == duel.Dead {
			for _, m := range losers {
				postLive(m, func() {
					l.broadcastLiveFrame(m, func() wire.Frame { return serverpackets.FrameSocialAction(m.ObjectID(), duelBowAction) })
				})
			}
		}
	}
	msgs := duelEndMessages(v, res)
	for _, m := range all {
		postLive(m, func() {
			for _, msg := range msgs {
				m.SendFrame(msg())
			}
			m.SendFrame(serverpackets.FrameExDuelEnd(v.Party))
		})
	}
	abnormal := !v.Party && res == duel.Canceled
	for _, m := range all {
		if !postLive(m, func() {
			l.restoreDuelCondition(m, abnormal)
			l.leaveDuel(m)
			l.duels.Leave(v.ID)
		}) {
			l.duels.Leave(v.ID)
		}
	}
}

// duelBowAction is the social action a duel's losers play.
const duelBowAction = 7

// duelLosers returns the leader and the players of the side res defeated,
// or nil for a tie.
func duelLosers(v duel.View[*livePlayer], res duel.Result) (*livePlayer, []*livePlayer) {
	switch res {
	case duel.Team1Win, duel.Team2Surrender:
		return v.B, v.TeamB
	case duel.Team2Win, duel.Team1Surrender:
		return v.A, v.TeamA
	}
	return nil, nil
}

// duelEndMessages are the messages a duel ending with res sends both
// sides: who withdrew, if anyone did, then who won, or the tie.
func duelEndMessages(v duel.View[*livePlayer], res duel.Result) []func() wire.Frame {
	won, withdrew := serverpackets.SystemMessageS1WonTheDuel, serverpackets.SystemMessageS1WithdrewS2Won
	if v.Party {
		won, withdrew = serverpackets.SystemMessageS1PartyWonTheDuel, serverpackets.SystemMessageS1PartyWithdrewS2PartyWon
	}
	withdrawal := func(loser, winner string) func() wire.Frame {
		return func() wire.Frame {
			return serverpackets.FrameSystemMessageParams(withdrew, serverpackets.TextParam(loser), serverpackets.TextParam(winner))
		}
	}
	victory := func(winner string) func() wire.Frame {
		return func() wire.Frame { return serverpackets.FrameSystemMessageString(won, winner) }
	}
	switch res {
	case duel.Team2Surrender:
		return []func() wire.Frame{withdrawal(v.B.Name, v.A.Name), victory(v.A.Name)}
	case duel.Team1Win:
		return []func() wire.Frame{victory(v.A.Name)}
	case duel.Team1Surrender:
		return []func() wire.Frame{withdrawal(v.A.Name, v.B.Name), victory(v.B.Name)}
	case duel.Team2Win:
		return []func() wire.Frame{victory(v.B.Name)}
	}
	return []func() wire.Frame{func() wire.Frame { return serverpackets.FrameSystemMessage(serverpackets.SystemMessageDuelEndedInTie) }}
}

// stopToFight stops live's cast and every action it takes, and clears its
// target.
func (l *GameClientLink) stopToFight(live *livePlayer) {
	live.Character.StopCast()
	live.goIdle()
	l.clearLiveTarget(live)
	live.SendFrame(serverpackets.FrameActionFailed())
}

// restoreDuelCondition puts back what live's duel saved when it began: a
// party duel's player returns where it stood, and, unless the duel ended
// abnormally, gets its CP, HP and MP back and its effects as they were.
func (l *GameClientLink) restoreDuelCondition(live *livePlayer, abnormal bool) {
	cond := live.duelCondition
	if cond == nil {
		return
	}
	if cond.at != nil {
		l.teleportLivePlayer(live, *cond.at, 0)
	}
	if abnormal {
		return
	}
	live.RestoreDuelVitals(cond.cp, cond.hp, cond.mp)
	live.StopAllEffects()
	if l.skills == nil {
		return
	}
	l.skills.RestoreDuelState(live.Character, cond.skills)
	st, skills, id := cond.skills, l.skills, live.ObjectID()
	l.persist.Enqueue(id, func() {
		ctx, cancel := context.WithTimeout(context.Background(), livePlayerDetachSaveTimeout)
		defer cancel()
		if err := skills.ClearSaved(ctx, st); err != nil {
			l.log.Error().Err(err).Int32("object_id", id).Msg("clear duel skill state")
		}
	})
}

// leaveDuel takes live out of its duel and back to no colour.
func (l *GameClientLink) leaveDuel(live *livePlayer) {
	live.duelCondition = nil
	live.LeaveDuel()
	live.SetDuelTeam(duel.TeamNone)
	l.refreshDuelLook(live)
}

// cancelPartyDuel cancels the party duel leader's party fights, as any
// change to its members does: every player returns where it stood and
// leaves the duel, then the duel ends in a tie.
func (l *GameClientLink) cancelPartyDuel(leader *livePlayer) {
	if l.duels == nil {
		return
	}
	v, ok := l.duels.PartyEdit(leader)
	if !ok {
		return
	}
	for _, m := range v.All() {
		postLive(m, func() {
			if cond := m.duelCondition; cond != nil && cond.at != nil {
				l.teleportLivePlayer(m, *cond.at, 0)
			}
			m.LeaveDuel()
		})
	}
	l.endDuel(v, duel.Canceled, true)
}

// sendDuelVitals shows live's new CP and HP in its opponents' duel window
// when a gauge left its segment.
func (l *GameClientLink) sendDuelVitals(live *livePlayer, cpOrHP bool) {
	if !cpOrHP || !live.InDuel() || l.duels == nil {
		return
	}
	opponents := l.duels.OppositeTeam(live)
	if len(opponents) == 0 {
		return
	}
	info := duelUserInfo(live)
	l.broadcastToMembers(opponents, func() wire.Frame { return serverpackets.FrameExDuelUpdateUserInfo(info) })
}

// refuseFrozenDuellist answers an action on a player, or a player's summon,
// that lost its duel: the other side is frozen. It reports whether it did.
func (l *GameClientLink) refuseFrozenDuellist(live *livePlayer, objectID int32) bool {
	target := l.resolveTarget(objectID)
	if target == nil {
		return false
	}
	var p *livePlayer
	switch t := target.(type) {
	case *livePlayer:
		p = t
	case *summon.Actor:
		if owner, ok := liveSummonOwner(t); ok {
			p = owner
		}
	}
	if p == nil || p.DuelState() != duel.Dead {
		return false
	}
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOtherPartyIsFrozen))
	live.SendFrame(serverpackets.FrameActionFailed())
	return true
}
