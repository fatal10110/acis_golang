package network

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// punishmentStore stores a character's chat ban or jail term.
type punishmentStore interface {
	SetPunishment(ctx context.Context, objectID int32, level int, timer int64) error
	SetPunishmentAt(ctx context.Context, objectID int32, level int, timer int64, at location.Location) error
	ReplacePunishment(ctx context.Context, objectID int32, expected, level int, timer int64) (int, error)
}

// punishmentWriteTimeout bounds one punishment write.
const punishmentWriteTimeout = 5 * time.Second

var (
	// jailLocation is where a jailed player is held.
	jailLocation = location.Location{X: -114356, Y: -249645, Z: -2984}
	// jailReleaseLocation is where a player leaving jail is sent: Floran
	// village.
	jailReleaseLocation = location.Location{X: 17836, Y: 170178, Z: -3507}
)

// Scatter radii of the jail teleports: a jail entry lands on the exact
// point, a release or a return to jail anywhere within 20.
const (
	jailEntryOffset   = 0
	jailReturnOffset  = 20
	jailReleaseOffset = 20
)

// Pages a player is shown entering and leaving jail.
const (
	jailInPage  = "data/html/jail_in.htm"
	jailOutPage = "data/html/jail_out.htm"
)

// Sounds of a chat ban starting and ending.
const (
	chatBanStartSound = "systemmsg_e.346"
	chatBanEndSound   = "systemmsg_e.345"
)

// punish applies kind to live for minutes, without end when minutes is not
// positive, on live's queue: the punishment and its timer change, live is
// told and moved as the change requires, and the result is stored.
// PunishNone lifts a chat ban or a jail term.
func (l *GameClientLink) punish(live *livePlayer, kind player.Punishment, minutes int32) {
	l.applyPunishmentChange(live, live.SetPunishment(kind, minutes), minutes)
	l.storePunishment(live)
}

// punishIf is punish applied only while live serves expected, checked and
// changed in one step on live's queue (#5129). It reports what changed;
// nothing is told, moved or stored when nothing did.
func (l *GameClientLink) punishIf(live *livePlayer, expected, kind player.Punishment, minutes int32) player.PunishmentChange {
	change := live.ReplacePunishment(expected, kind, minutes)
	if change != player.PunishmentKept {
		l.applyPunishmentChange(live, change, minutes)
		l.storePunishment(live)
	}
	return change
}

// applyPunishmentChange tells and moves live as change requires, on live's
// queue; minutes is the term of a punishment change started.
func (l *GameClientLink) applyPunishmentChange(live *livePlayer, change player.PunishmentChange, minutes int32) {
	switch change {
	case player.ChatBanLifted:
		live.stopPunishTimer(true)
		live.SendFrame(serverpackets.FrameEtcStatusUpdate(etcStatus(live.Character)))
		sendText(live, "Chatting is now available.")
		live.SendFrame(serverpackets.FramePlaySound(chatBanEndSound))
	case player.JailLifted:
		l.sendPunishmentPage(live, jailOutPage)
		live.stopPunishTimer(true)
		// A release refused mid-landing is made once live lands, unless
		// it is jailed again or out of the jail by then.
		l.punishTeleportLivePlayer(live, jailReleaseLocation, jailReleaseOffset, func() bool {
			return !jailed(live) && inJail(live)
		})
	case player.ChatBanStarted:
		live.SendFrame(serverpackets.FrameEtcStatusUpdate(etcStatus(live.Character)))
		live.stopPunishTimer(false)
		if minutes > 0 {
			l.armPunishTimer(live)
			sendText(live, "Chatting has been suspended for "+strconv.Itoa(int(minutes))+" minute(s).")
		} else {
			sendText(live, "Chatting has been suspended.")
		}
		live.SendFrame(serverpackets.FramePlaySound(chatBanStartSound))
	case player.JailStarted:
		live.stopPunishTimer(false)
		if minutes > 0 {
			l.armPunishTimer(live)
			sendText(live, "You are jailed for "+strconv.Itoa(int(minutes))+" minutes.")
		}
		if l.olympiadRegistered(live) {
			l.dropOlympiadCompetitor(live)
		}
		l.sendPunishmentPage(live, jailInPage)
		live.SetIn7sDungeon(false)
		// An observer stops watching where it stands first, so that
		// nothing of its watch can take it back out of the jail.
		l.endObservationInPlace(live)
		l.punishTeleportLivePlayer(live, jailLocation, jailEntryOffset, jailedOutside(live))
	}
}

// enterWorldPunishment resumes, at login, the punishment live serves: a
// timed one restarts its timer, with a reminder of the minutes left, and a
// jailed player found outside the jail is taken back. A player found in the
// jail without a jail term, as a jail cleared without moving the character
// leaves it, is released to Floran village instead (#5128).
func (l *GameClientLink) enterWorldPunishment(live *livePlayer) {
	kind, timer := live.Punishment()
	if kind != player.PunishNone && timer > 0 {
		l.armPunishTimer(live)
		sendText(live, "You are still "+kind.Description()+" for "+strconv.Itoa(int(player.PunishmentMinutesLeft(timer)))+" minutes.")
	}
	switch {
	case kind == player.PunishJail && !inJail(live):
		l.punishTeleportLivePlayer(live, jailLocation, jailReturnOffset, jailedOutside(live))
	case kind != player.PunishJail && inJail(live):
		l.punishTeleportLivePlayer(live, jailReleaseLocation, jailReleaseOffset, func() bool {
			return !jailed(live) && inJail(live)
		})
	}
}

// jailed reports whether live serves a jail term.
func jailed(live *livePlayer) bool {
	kind, _ := live.Punishment()
	return kind == player.PunishJail
}

// inJail reports whether live stands in a jail.
func inJail(live *livePlayer) bool {
	return live.zoneActor != nil && live.zoneActor.ZoneFlags().Has(zone.FlagJail)
}

// atLarge is the condition of a move made once live's own teleport lands
// that yields to a jail term or an Olympiad match begun meanwhile.
func atLarge(live *livePlayer) func() bool {
	return func() bool { return !jailed(live) && !live.OlympiadMode() }
}

// jailedOutside is the condition of taking live to the jail once its own
// teleport lands: it is still jailed and has not landed in a jail.
func jailedOutside(live *livePlayer) func() bool {
	return func() bool { return jailed(live) && !inJail(live) }
}

// armPunishTimer lifts live's punishment once its timer has run, counted
// from now.
func (l *GameClientLink) armPunishTimer(live *livePlayer) {
	sim.AssertOwner(live.Queue())
	_, timer := live.Punishment()
	delay := time.Duration(math.MaxInt64)
	if timer < int64(delay/time.Millisecond) {
		delay = time.Duration(timer) * time.Millisecond
	}
	live.punishDeadline = live.Queue().Now().Add(delay)
	// live's queue closes when it leaves the game, which drops the timer.
	live.punishTimer = live.after(delay, func() { l.punish(live, player.PunishNone, 0) })
}

// stopPunishTimer cancels p's punishment timer, if one runs; with save, the
// punishment timer becomes the time that was left.
func (p *livePlayer) stopPunishTimer(save bool) {
	sim.AssertOwner(p.Queue())
	if p.punishTimer == nil {
		return
	}
	if save {
		p.SetPunishmentTimer(max(0, p.punishDeadline.Sub(p.Queue().Now()).Milliseconds()))
	}
	p.punishTimer.Stop()
	p.punishTimer = nil
}

// sendPunishmentPage opens the page file for live.
func (l *GameClientLink) sendPunishmentPage(live *livePlayer, file string) {
	page, ok := l.html.Get(file)
	if !ok {
		page = fmt.Sprintf("<html><body>My html is missing:<br>%s</body></html>", file)
	}
	l.sendValidatedFile(live, 0, file, page, 0)
}

// storePunishment writes live's punishment on its persistence lane.
func (l *GameClientLink) storePunishment(live *livePlayer) {
	kind, timer := live.Punishment()
	l.storePunishmentOf(live.ObjectID(), kind, timer)
}

// storePunishmentOf writes kind and timer as objectID's punishment on its
// persistence lane, ahead of any save its logout queues after.
func (l *GameClientLink) storePunishmentOf(objectID int32, kind player.Punishment, timer int64) {
	l.storePunishmentAt(objectID, kind, timer, nil)
}

// storePunishmentAt is storePunishmentOf moving the character to at as
// well, when at is set.
func (l *GameClientLink) storePunishmentAt(objectID int32, kind player.Punishment, timer int64, at *location.Location) {
	if l.punishments == nil {
		return
	}
	store, log := l.punishments, l.log
	if !l.persist.Enqueue(objectID, func() {
		ctx, cancel := context.WithTimeout(context.Background(), punishmentWriteTimeout)
		defer cancel()
		var err error
		if at != nil {
			err = store.SetPunishmentAt(ctx, objectID, int(kind), timer, *at)
		} else {
			err = store.SetPunishment(ctx, objectID, int(kind), timer)
		}
		if err != nil {
			log.Error().Err(err).Int32("object_id", objectID).Msg("store punishment")
		}
	}) {
		log.Error().Int32("object_id", objectID).Msg("store punishment: persistence closed")
	}
}

// replaceStoredPunishment stores kind and timer as objectID's punishment
// on its persistence lane, after any save its logout queued, only while the
// stored one is expected: the check and the write are one step there, so a
// punishment stored meanwhile is never replaced or lifted by mistake
// (#5502). report is then given the punishment stored before, expected
// when it was replaced; a failed write is logged and reports nothing.
func (l *GameClientLink) replaceStoredPunishment(objectID int32, expected, kind player.Punishment, timer int64, report func(stored player.Punishment)) {
	if l.punishments == nil {
		return
	}
	store, log := l.punishments, l.log
	if !l.persist.Enqueue(objectID, func() {
		ctx, cancel := context.WithTimeout(context.Background(), punishmentWriteTimeout)
		defer cancel()
		stored, err := store.ReplacePunishment(ctx, objectID, int(expected), int(kind), timer)
		if err != nil {
			log.Error().Err(err).Int32("object_id", objectID).Msg("replace punishment")
			return
		}
		report(player.Punishment(stored))
	}) {
		log.Error().Int32("object_id", objectID).Msg("replace punishment: persistence closed")
	}
}

// punishTarget applies kind to the online target for minutes on target's
// queue. A target already leaving the world gets the punishment stored
// instead, after its logout saves, which its next login resumes: a jail
// stores it in the jail and a release from jail in Floran village, as for a
// character out of the world, since the move it would have made online
// can no longer reach its logout save.
func (l *GameClientLink) punishTarget(gm, target *livePlayer, kind player.Punishment, minutes int32) {
	onPlayerOr(gm, target, func() { l.punish(target, kind, minutes) }, func() {
		var at *location.Location
		switch current, _ := target.Punishment(); {
		case kind == player.PunishJail:
			at = &jailLocation
		case kind == player.PunishNone && current == player.PunishJail:
			at = &jailReleaseLocation
		}
		l.storePunishmentAt(target.ObjectID(), kind, player.PunishmentMillis(minutes), at)
	})
}
