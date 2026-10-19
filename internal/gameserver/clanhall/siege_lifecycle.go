package clanhall

import (
	"slices"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// registrationCloseMs is how long before its siege date a hall's
// registrations close.
const registrationCloseMs = 3_600_000

// NextSiegeDate is a hall's siege date set from now: schedule's days,
// months and years ahead, in that order, at schedule's hour and minute on
// now's clock, second 0, now's millisecond kept. schedule is the hall's
// scheduleConfig (days;months;years;hour;minute); a missing value counts as
// 0.
func NextSiegeDate(now time.Time, schedule []int) time.Time {
	at := func(i int) int {
		if i < len(schedule) {
			return schedule[i]
		}
		return 0
	}
	t := now.AddDate(0, 0, at(0))
	t = addMonths(t, at(1))
	t = addMonths(t, 12*at(2))
	ms := t.Nanosecond() / int(time.Millisecond) * int(time.Millisecond)
	return time.Date(t.Year(), t.Month(), t.Day(), at(3), at(4), 0, ms, t.Location())
}

// addMonths moves t n months, the day pinned to the last of the month it
// lands in when that month is shorter.
func addMonths(t time.Time, n int) time.Time {
	if n == 0 {
		return t
	}
	first := time.Date(t.Year(), t.Month(), 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location()).AddDate(0, n, 0)
	last := first.AddDate(0, 1, -1).Day()
	return first.AddDate(0, 0, min(t.Day(), last)-1)
}

// siegeEffects are a step's world effects, run in order once the halls'
// locks are released.
type siegeEffects []func()

func (fx *siegeEffects) add(f func()) { *fx = append(*fx, f) }

func (fx siegeEffects) run() {
	for _, f := range fx {
		f()
	}
}

// siegeStep runs step: serialized with every other step, deciding under the
// halls' mu, then running the world effects it decided with no lock held.
func (hs *Halls) siegeStep(step func(fx *siegeEffects)) {
	hs.stepMu.Lock()
	defer hs.stepMu.Unlock()
	var fx siegeEffects
	hs.mu.Lock()
	step(&fx)
	hs.mu.Unlock()
	fx.run()
}

// setSiege runs f, a change of the sieges' state, under siegeMu. The halls'
// mu is held too: every change of a hall siege holds both, so a read holds
// either.
func (hs *Halls) setSiege(f func()) {
	hs.siegeMu.Lock()
	defer hs.siegeMu.Unlock()
	f()
}

// armLocked arms fire after d. The step armed before it is no longer the
// one CancelTask stops, but still runs when due.
func (hs *Halls) armLocked(h *hall, d time.Duration, fire func()) {
	var t *sim.Timer
	if hs.queue != nil {
		t = hs.queue.After(max(d, 0), fire)
	}
	hs.setSiege(func() { h.siege.task, h.siege.armed = t, true })
}

// armPrepareLocked arms h's registrations' close an hour before its siege
// date, the siege then starting at that date.
func (hs *Halls) armPrepareLocked(h *hall) {
	next := h.siege.next
	hs.armLocked(h, hs.siegeDelay(next-registrationCloseMs), func() {
		hs.siegeStep(func(fx *siegeEffects) { hs.prepareLocked(h, fx, hs.siegeDelay(h.siege.next)) })
	})
	hs.log.Info().Int32("hall_id", h.id()).Time("date", time.UnixMilli(next)).Msg("clanhall: siege scheduled")
}

// armEndLocked arms the end of h's siege after d.
func (hs *Halls) armEndLocked(h *hall, d time.Duration) {
	hs.armLocked(h, d, func() { (&HallSiege{hs: hs, h: h}).End() })
}

// Instant closes the registrations now and starts the siege at once,
// reporting false when no script runs it.
func (s *HallSiege) Instant() bool {
	if s.script() == nil {
		return false
	}
	s.hs.siegeStep(func(fx *siegeEffects) { s.hs.prepareLocked(s.h, fx, 0) })
	return true
}

// End ends the siege now, as its time running out does, reporting false
// when no script runs it. The script names the winner first.
func (s *HallSiege) End() bool {
	sc := s.script()
	if sc == nil {
		return false
	}
	var winner int32
	if sc.Winner != nil {
		winner = sc.Winner()
	}
	s.hs.siegeStep(func(fx *siegeEffects) { s.hs.endLocked(s.h, fx, winner) })
	return true
}

// CancelTask stops the step armed last.
func (s *HallSiege) CancelTask() {
	s.hs.siegeMu.Lock()
	t := s.h.siege.task
	s.hs.siegeMu.Unlock()
	if t != nil {
		t.Stop()
	}
}

func (s *HallSiege) script() *SiegeScript {
	s.hs.siegeMu.Lock()
	defer s.hs.siegeMu.Unlock()
	return s.h.siege.script
}

// prepareLocked closes h's registrations: an owner that is a clan joins the
// attackers and the hall is remembered as owned, the hall is freed, the
// players of every clan are thrown out of its grounds, everyone is told and
// the siege starts after delay.
func (hs *Halls) prepareLocked(h *hall, fx *siegeEffects, delay time.Duration) {
	st := h.siege
	owned := h.ownerID > 0
	_, ownerIsClan := hs.clans.Get(h.ownerID)
	hs.setSiege(func() {
		st.previouslyOwned = owned
		if owned && ownerIsClan && !slices.Contains(st.attackers, h.ownerID) {
			st.attackers = append(st.attackers, h.ownerID)
			hs.saveAttackersLocked(h)
		}
	})
	hs.freeLocked(h)
	hs.banishLocked(h, fx, 0)
	hs.setSiege(func() { st.status = siege.StatusRegistrationOver })
	hs.armLocked(h, delay, func() { hs.siegeStep(func(fx *siegeEffects) { hs.startLocked(h, fx) }) })
	hs.announceLocked(fx, siege.Message{ID: siege.MsgRegistrationTermEnded, CastleID: int(h.id())})
}

// startLocked starts h's siege. With no attacker, and the hall not the
// Fortress of Resistance, it is called off instead: the next date is set,
// the registrations open again and everyone is told. Otherwise the hall's
// doors stand again, everyone is thrown out of its grounds, its battlefield
// turns on, the attackers' members in the world take the attacker state,
// the siege is under way, the script spawns its NPCs and the siege ends
// once its length has passed.
func (hs *Halls) startLocked(h *hall, fx *siegeEffects) {
	st := h.siege
	id := h.id()
	if len(st.attackers) == 0 && id != FortressOfResistance {
		hs.updateNextSiegeLocked(h)
		hs.armPrepareLocked(h)
		hs.setSiege(func() { st.status = siege.StatusRegistrationOpened })
		hs.announceLocked(fx, siege.Message{ID: siege.MsgSiegeCanceledNoInterest, CastleID: int(id)})
		return
	}
	s := &HallSiege{hs: hs, h: h}
	hs.respawnDoorsLocked(h, fx)
	hs.banishLocked(h, fx, -1)
	fx.add(func() { s.withBattlefield(func(f *zone.Siege) { f.SetActive(true) }) })
	if n := hs.siegeNotify; n != nil {
		attackers := hs.resolveClans(st.attackers)
		fx.add(func() { n.SetSiegeState(attackers, siege.StateAttacker) })
	}
	hs.setSiege(func() { st.status = siege.StatusInProgress })
	if sc := st.script; sc.SpawnNPCs != nil {
		fx.add(sc.SpawnNPCs)
	}
	hs.armEndLocked(h, time.Duration(h.data.SiegeLength)*time.Millisecond)
}

// endLocked ends h's siege, winnerID being the clan the script names. A won
// siege gives the hall to the winner, if still a clan, and tells everyone;
// otherwise everyone is told of a draw. Everyone is told the siege is over,
// the battlefield turns off, the next date is set, the hall's doors stand
// again, the players of other clans than the owner's are thrown out of its
// grounds, each attacker loses its headquarters and its members in the
// world their siege state, the players still on the battlefield are
// flagged, the attackers are dropped, the registrations open again for the
// next siege and the script takes its NPCs away.
func (hs *Halls) endLocked(h *hall, fx *siegeEffects, winnerID int32) {
	st := h.siege
	id := h.id()
	s := &HallSiege{hs: hs, h: h}
	winner, isClan := hs.clans.Get(winnerID)
	if st.accomplished && winnerID != 0 && isClan {
		hs.setOwnerLocked(h, winner)
		hs.announceLocked(fx, siege.Message{ID: MsgClanDefeated, Text: winner.Name(), CastleID: int(id)})
	} else {
		hs.announceLocked(fx, siege.Message{ID: siege.MsgSiegeDraw, CastleID: int(id)})
	}
	hs.announceLocked(fx, siege.Message{ID: MsgSiegeFinished, CastleID: int(id)})
	hs.setSiege(func() { st.accomplished = false })
	fx.add(func() { s.withBattlefield(func(f *zone.Siege) { f.SetActive(false) }) })
	hs.updateNextSiegeLocked(h)
	hs.respawnDoorsLocked(h, fx)
	hs.banishLocked(h, fx, h.ownerID)
	n := hs.siegeNotify
	for _, cl := range hs.resolveClans(st.attackers) {
		fx.add(func() {
			cl.DropHeadquarters()
			if n != nil {
				n.SetSiegeState([]*clan.Clan{cl}, siege.StateNone)
			}
		})
	}
	if n != nil {
		owner := h.ownerID
		fx.add(func() { s.withBattlefield(func(f *zone.Siege) { n.FlagPvP(f, id, owner) }) })
	}
	hs.setSiege(func() {
		st.attackers = nil
		hs.saveAttackersLocked(h)
	})
	hs.armPrepareLocked(h)
	hs.setSiege(func() { st.status = siege.StatusRegistrationOpened })
	if sc := st.script; sc.UnspawnNPCs != nil {
		fx.add(sc.UnspawnNPCs)
	}
}

// updateNextSiegeLocked sets h's next siege date from now and stores the
// hall's row.
func (hs *Halls) updateNextSiegeLocked(h *hall) {
	next := NextSiegeDate(time.UnixMilli(hs.nowLocked()), h.data.ScheduleConfig).UnixMilli()
	hs.setSiege(func() { h.siege.next = next })
	hs.updateLocked(h)
}

// banishLocked throws every player not of clan clanID out of h's grounds,
// everyone for -1.
func (hs *Halls) banishLocked(h *hall, fx *siegeEffects, clanID int32) {
	if g := hs.grounds; g != nil {
		id := h.id()
		fx.add(func() { g.BanishForeigners(id, clanID) })
	}
}

// respawnDoorsLocked stands h's doors again at full HP, closed.
func (hs *Halls) respawnDoorsLocked(h *hall, fx *siegeEffects) {
	if n := hs.siegeNotify; n != nil && len(h.data.Gates) > 0 {
		gates := h.data.Gates
		fx.add(func() { n.RespawnDoors(gates, false) })
	}
}

// announceLocked tells every player in the world m.
func (hs *Halls) announceLocked(fx *siegeEffects, m siege.Message) {
	if n := hs.siegeNotify; n != nil {
		fx.add(func() { n.Announce(m) })
	}
}

// resolveClans resolves ids to the clans still existing, in order.
func (hs *Halls) resolveClans(ids []int32) []*clan.Clan {
	out := make([]*clan.Clan, 0, len(ids))
	for _, id := range ids {
		if cl, ok := hs.clans.Get(id); ok {
			out = append(out, cl)
		}
	}
	return out
}

// restoreSiegeDateLocked sets h's next siege date from next, its stored
// row's, at boot: a date already passed is set anew from now and the row
// stored.
func (hs *Halls) restoreSiegeDateLocked(h *hall, next int64) {
	if next-hs.nowLocked() < 0 {
		hs.updateNextSiegeLocked(h)
		return
	}
	hs.setSiege(func() { h.siege.next = next })
}
