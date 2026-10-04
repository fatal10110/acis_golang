package network

import (
	"context"
	"strconv"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
)

// classRankingTimeout bounds the read of a class's ranking.
const classRankingTimeout = 5 * time.Second

// passesMultisell is the multisell list the Olympiad manager sells for
// Noblesse Gate Passes.
const passesMultisell = "102"

// olympiadNobleBypass runs an Olympiad manager's "OlympiadNoble <choice>"
// service for live; see npc.BypassOlympiadNoble.
func (l *GameClientLink) olympiadNobleBypass(live *livePlayer, f *npc.Folk, choice int) {
	o := l.olympiad
	if o == nil {
		l.log.Debug().Int("choice", choice).Msg("bypass: olympiad manager without an Olympiad")
		return
	}
	pages := setPages{l.html}
	id := live.ObjectID()
	switch choice {
	case 1:
		l.sendUnregisterResult(live, o.Unregister(id, live.BaseClassID(), live.IsNoble()))
	case 2:
		classed, nonClassed := o.WaitingList()
		sendFilledHTML(live, f.ObjectID(), f.OlympiadPage(pages, "noble_registered.htm",
			"%listClassed%", strconv.Itoa(classed), "%listNonClassed%", strconv.Itoa(nonClassed)), 0)
	case 3:
		sendFilledHTML(live, f.ObjectID(), f.OlympiadPage(pages, "noble_points1.htm", "%points%", strconv.Itoa(o.Points(id))), 0)
	case 4, 5:
		kind := olympiad.NonClassed
		if choice == 5 {
			kind = olympiad.Classed
		}
		l.registerOlympiad(live, f, kind)
	case 6:
		page := "noble_nopoints2.htm"
		if o.Points(id) >= 50 || l.olympiadHero(live) {
			page = "noble_settle.htm"
		}
		sendFilledHTML(live, f.ObjectID(), f.OlympiadPage(pages, page), 0)
	case 7:
		l.openMultisell(live, f, passesMultisell, false)
	case 10:
		if passes := o.Passes(id, l.olympiadHero(live)); passes > 0 {
			live.AddCreatedItem(olympiad.NoblesseGatePass, passes, l.nextObjectID)
		}
	}
}

// olympiadHero reports whether live counts as a hero at the Olympiad
// manager: a hero, or one elected who has not claimed the status yet.
func (l *GameClientLink) olympiadHero(live *livePlayer) bool {
	return live.IsHero() || (l.heroes != nil && l.heroes.IsInactive(live.ObjectID()))
}

// registerOlympiad asks for live's registration for matches of kind at
// Olympiad manager f and tells live the answer: a record without points
// gets f's page saying so, everything else a system message.
func (l *GameClientLink) registerOlympiad(live *livePlayer, f *npc.Folk, kind olympiad.GameType) {
	cursedWeapon := live.CursedWeaponID()
	result := l.olympiad.Register(olympiad.Applicant{
		ObjectID: live.ObjectID(), Name: live.Name, BaseClass: live.BaseClassID(), Noble: live.IsNoble(),
		SubclassActive: live.ClassIndex() != 0, CursedWeaponID: cursedWeapon, Overweight: live.Overweight(),
	}, kind)
	var msg int
	switch result {
	case olympiad.Registered:
		msg = serverpackets.SystemMessageRegisteredInNoClassGamesWaitingList
		if kind == olympiad.Classed {
			msg = serverpackets.SystemMessageRegisteredInClassifiedGamesWaitingList
		}
	case olympiad.RegisterNotInProgress:
		msg = serverpackets.SystemMessageOlympiadGameNotInProgress
	case olympiad.RegisterClosing:
		msg = serverpackets.SystemMessageGameRequestCannotBeMade
	case olympiad.RegisterNotNoble:
		msg = serverpackets.SystemMessageOnlyNoblessCanParticipateInOlympiad
	case olympiad.RegisterSubclass:
		msg = serverpackets.SystemMessageCantJoinOlympiadWithSubJob
	case olympiad.RegisterCursedWeapon:
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageCannotJoinOlympiadPossessingS1, cursedWeapon))
		return
	case olympiad.RegisterOverweight:
		msg = serverpackets.SystemMessageInventoryTooFullForOlympiad
	case olympiad.RegisterAlreadyNonClassed:
		msg = serverpackets.SystemMessageAlreadyOnAllClassesWaitingList
	case olympiad.RegisterAlreadyClassed:
		msg = serverpackets.SystemMessageAlreadyOnClassWaitingList
	case olympiad.RegisterInMatch:
		msg = serverpackets.SystemMessageAlreadyRegisteredInEventWaitingList
	case olympiad.RegisterNoPoints:
		sendFilledHTML(live, f.ObjectID(), f.OlympiadPage(setPages{l.html}, "noble_nopoints1.htm"), 0)
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessage(msg))
}

// sendUnregisterResult tells live the answer to its request to leave the
// Olympiad's waiting list; a player in a running match is told nothing.
func (l *GameClientLink) sendUnregisterResult(live *livePlayer, result olympiad.UnregisterResult) {
	var msg int
	switch result {
	case olympiad.Unregistered:
		msg = serverpackets.SystemMessageDeletedFromGameWaitingList
	case olympiad.UnregisterNotInProgress:
		msg = serverpackets.SystemMessageOlympiadGameNotInProgress
	case olympiad.UnregisterNotNoble:
		msg = serverpackets.SystemMessageNoblesseOnly
	case olympiad.UnregisterNotRegistered:
		msg = serverpackets.SystemMessageNotRegisteredInGameWaitingList
	case olympiad.UnregisterInMatch:
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessage(msg))
}

// olympiadRegistered reports whether live waits for an Olympiad match or
// competes in one.
func (l *GameClientLink) olympiadRegistered(live *livePlayer) bool {
	return l.olympiad != nil && l.olympiad.IsRegisteredInComp(live.ObjectID(), live.BaseClassID())
}

// leaveOlympiadOnSubclass drops live's Olympiad registration as a village
// master's subclass command starts, with the answer an unregistration gets.
func (l *GameClientLink) leaveOlympiadOnSubclass(live *livePlayer) {
	if l.olympiadRegistered(live) {
		l.sendUnregisterResult(live, l.olympiad.Unregister(live.ObjectID(), live.BaseClassID(), live.IsNoble()))
	}
}

// dropOlympiadCompetitor takes live off the Olympiad's waiting list as it
// leaves the world or is jailed.
func (l *GameClientLink) dropOlympiadCompetitor(live *livePlayer) {
	if l.olympiad != nil {
		l.olympiad.RemoveDisconnectedCompetitor(live.ObjectID(), live.BaseClassID())
	}
}

// showClassRanking opens Olympiad manager f's ranking of classID for live:
// the month's ten best of the class, read from the database. A failed read
// is logged and shows the ranking empty.
func (l *GameClientLink) showClassRanking(live *livePlayer, f *npc.Folk, classID int) {
	var names []string
	if l.olympiad != nil {
		ctx, cancel := context.WithTimeout(context.Background(), classRankingTimeout)
		defer cancel()
		var err error
		if names, err = l.olympiad.ClassLeaders(ctx, classID); err != nil {
			l.log.Error().Err(err).Int("class_id", classID).Msg("olympiad class ranking")
			names = nil
		}
	}
	sendFilledHTML(live, f.ObjectID(), f.OlympiadRankingPage(setPages{l.html}, names), 0)
}
