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
)

// punishmentStore stores a character's chat ban or jail term.
type punishmentStore interface {
	SetPunishment(ctx context.Context, objectID int32, level int, timer int64) error
	SetPunishmentByName(ctx context.Context, name string, level int, timer int64) (bool, error)
	SetPunishmentAtByName(ctx context.Context, name string, level int, timer int64, at location.Location) (bool, error)
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
	switch live.SetPunishment(kind, minutes) {
	case player.ChatBanLifted:
		live.stopPunishTimer(true)
		live.SendFrame(serverpackets.FrameEtcStatusUpdate(etcStatus(live.Character)))
		sendText(live, "Chatting is now available.")
		live.SendFrame(serverpackets.FramePlaySound(chatBanEndSound))
	case player.JailLifted:
		l.sendPunishmentPage(live, jailOutPage)
		live.stopPunishTimer(true)
		l.teleportLivePlayer(live, jailReleaseLocation, jailReleaseOffset)
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
		l.teleportLivePlayer(live, jailLocation, jailEntryOffset)
	}
	l.storePunishment(live)
}

// enterWorldPunishment resumes, at login, the punishment live serves: a
// timed one restarts its timer, with a reminder of the minutes left, and a
// jailed player found outside the jail is taken back.
func (l *GameClientLink) enterWorldPunishment(live *livePlayer) {
	kind, timer := live.Punishment()
	if kind == player.PunishNone {
		return
	}
	if timer > 0 {
		l.armPunishTimer(live)
		sendText(live, "You are still "+kind.Description()+" for "+strconv.Itoa(int(player.PunishmentMinutesLeft(timer)))+" minutes.")
	}
	if kind == player.PunishJail && (live.zoneActor == nil || !live.zoneActor.ZoneFlags().Has(zone.FlagJail)) {
		l.teleportLivePlayer(live, jailLocation, jailReturnOffset)
	}
}

// armPunishTimer lifts live's punishment once its timer has run, counted
// from now.
func (l *GameClientLink) armPunishTimer(live *livePlayer) {
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
	sendValidatedHTML(live, 0, page, 0)
}

// storePunishment writes live's punishment on its persistence lane.
func (l *GameClientLink) storePunishment(live *livePlayer) {
	kind, timer := live.Punishment()
	l.storePunishmentOf(live.ObjectID(), kind, timer)
}

// storePunishmentOf writes kind and timer as objectID's punishment on its
// persistence lane, ahead of any save its logout queues after.
func (l *GameClientLink) storePunishmentOf(objectID int32, kind player.Punishment, timer int64) {
	if l.punishments == nil {
		return
	}
	store, log := l.punishments, l.log
	if !l.persist.Enqueue(objectID, func() {
		ctx, cancel := context.WithTimeout(context.Background(), punishmentWriteTimeout)
		defer cancel()
		if err := store.SetPunishment(ctx, objectID, int(kind), timer); err != nil {
			log.Error().Err(err).Int32("object_id", objectID).Msg("store punishment")
		}
	}) {
		log.Error().Int32("object_id", objectID).Msg("store punishment: persistence closed")
	}
}

// punishTarget applies kind to the online target for minutes on target's
// queue. A target already leaving the world gets the punishment stored
// instead, which its next login resumes.
func (l *GameClientLink) punishTarget(gm, target *livePlayer, kind player.Punishment, minutes int32) {
	if onPlayer(gm, target, func() { l.punish(target, kind, minutes) }) {
		return
	}
	l.storePunishmentOf(target.ObjectID(), kind, player.PunishmentMillis(minutes))
}

// storePunishmentByName writes level and timer as the punishment of the
// offline character named name, moving it to at when at is set, and reports
// whether one has that name. ok is false when the write failed; it is
// logged.
func (l *GameClientLink) storePunishmentByName(name string, level player.Punishment, timer int64, at *location.Location) (found, ok bool) {
	if l.punishments == nil {
		return false, true
	}
	ctx, cancel := context.WithTimeout(context.Background(), punishmentWriteTimeout)
	defer cancel()
	var err error
	if at != nil {
		found, err = l.punishments.SetPunishmentAtByName(ctx, name, int(level), timer, *at)
	} else {
		found, err = l.punishments.SetPunishmentByName(ctx, name, int(level), timer)
	}
	if err != nil {
		l.log.Error().Err(err).Str("name", name).Msg("store punishment")
		return false, false
	}
	return found, true
}
