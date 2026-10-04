package siege

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
)

const (
	dayMs = int64(24 * time.Hour / time.Millisecond)
	// The steps towards a siege's start: the registrations close a day
	// before it, then the calendar wakes 13,600,000 ms, ten minutes, five
	// minutes and ten seconds before it.
	registrationEndMs = 13_600_000
	tenMinutesMs      = 600_000
	fiveMinutesMs     = 300_000
	tenSecondsMs      = 10_000
	// siegeHour is the hour of day a siege date is set to.
	siegeHour = 18
	// The reputation a siege's end moves.
	reputationWon  = 1000
	reputationHeld = 500
)

// NextDate is the siege date after prev, set when a siege ends or is
// called off: from prev, or from now when prev has passed, the castle's
// siege day of that week (Sunday for castles 3, 4, 6 and 7, Saturday for
// the others, weeks starting on Sunday), two weeks later, at 18:00 on
// now's clock.
func NextDate(castleID int, prev, now time.Time) time.Time {
	base := prev.In(now.Location())
	if prev.Before(now) {
		base = now
	}
	day := time.Saturday
	switch castleID {
	case 3, 4, 6, 7:
		day = time.Sunday
	}
	d := base.AddDate(0, 0, int(day)-int(base.Weekday())+14)
	return time.Date(d.Year(), d.Month(), d.Day(), siegeHour, 0, 0, 0, d.Location())
}

// startAutoTaskLocked sets the siege's calendar going at boot: a date
// already passed is moved to the next one and stored, the calendar left
// idle; any other date has its next step taken a second from now.
func (s *Siege) startAutoTaskLocked(fx *effects) {
	if s.castle.SiegeDate() < s.e.nowLocked().UnixMilli() {
		s.saveSiegeLocked(fx, false)
		return
	}
	s.armStartLocked(time.Second)
}

// saveSiegeLocked sets the next siege date, opens its registrations and
// its date to change, and stores it; when launch is set, the calendar then
// takes its next step a second from now.
func (s *Siege) saveSiegeLocked(fx *effects, launch bool) {
	now := s.e.nowLocked()
	next := NextDate(s.castle.ID, time.UnixMilli(s.castle.SiegeDate()), now)
	s.castle.SetSiegeDate(next.UnixMilli())
	s.announceLocked(fx, castleMsg(MsgAnnouncedSiegeTime, s.castle.ID))
	s.status = StatusRegistrationOpened
	s.castle.SetTimeRegistrationOver(false)
	if s.start != nil {
		s.armStartLocked(time.Second)
	}
	s.castle.SaveSiegeInfo()
	if launch {
		s.armStartLocked(time.Second)
	}
	s.e.log.Info().Str("castle", s.castle.Name).Time("date", next).Msg("siege: new date")
}

// armStartLocked arms the calendar's next step after d, replacing the step
// armed before.
func (s *Siege) armStartLocked(d time.Duration) {
	if s.start != nil {
		s.start.Stop()
	}
	s.startGen++
	if s.e.queue == nil {
		s.start = nil
		return
	}
	gen := s.startGen
	s.start = s.e.queue.After(max(d, 0), func() { s.step(gen) })
}

// step is the calendar's step towards the siege, gen the generation its
// timer was armed with. While the siege date is open to change, it waits
// for the registrations' end, a day before the siege, and closes the date
// then. A day or less before the siege it closes the registrations, telling
// everyone, and drops the clans still waiting to defend; it then wakes
// 13,600,000 ms, ten minutes, five minutes and ten seconds before the
// siege, and starts it once its date is reached.
func (s *Siege) step(gen uint64) {
	var fx effects
	s.e.mu.Lock()
	if s.startGen != gen {
		s.e.mu.Unlock()
		return
	}
	s.start = nil
	s.stepLocked(&fx)
	s.e.mu.Unlock()
	fx.run()
}

func (s *Siege) stepLocked(fx *effects) {
	if s.status == StatusInProgress {
		return
	}
	now := s.e.nowLocked().UnixMilli()
	if !s.castle.IsTimeRegistrationOver() {
		if remaining := s.castle.SiegeDate() - dayMs - now; remaining > 0 {
			s.armStartLocked(time.Duration(remaining) * time.Millisecond)
			return
		}
		s.castle.SetTimeRegistrationOver(true)
	}
	remaining := s.castle.SiegeDate() - now
	ms := func(v int64) time.Duration { return time.Duration(v) * time.Millisecond }
	switch {
	case remaining > dayMs:
		s.armStartLocked(ms(remaining - dayMs))
	case remaining > registrationEndMs:
		s.announceLocked(fx, castleMsg(MsgRegistrationTermEnded, s.castle.ID))
		s.status = StatusRegistrationOver
		s.clearPendingLocked()
		s.armStartLocked(ms(remaining - registrationEndMs))
	case remaining > tenMinutesMs:
		s.armStartLocked(ms(remaining - tenMinutesMs))
	case remaining > fiveMinutesMs:
		s.armStartLocked(ms(remaining - fiveMinutesMs))
	case remaining > tenSecondsMs:
		s.armStartLocked(ms(remaining - tenSecondsMs))
	case remaining > 0:
		s.armStartLocked(ms(remaining))
	default:
		s.startLocked(fx)
	}
}

// Start starts the siege now, as its date coming does.
func (s *Siege) Start() {
	var fx effects
	s.e.mu.Lock()
	s.startLocked(&fx)
	s.e.mu.Unlock()
	fx.run()
}

// startLocked starts the siege, unless it is under way. With no clan
// attacking, it is called off instead: everyone is told and the next date
// is set. Otherwise the siege is under way: the castle owner's clan is
// remembered, the players of other clans are thrown off the battlefield,
// which turns on, every registered clan's members in the world take their
// siege state, everyone is told and hears it start, the attackers are told
// they are in a temporary alliance, and the siege clock runs.
//
// ponytail: after the siege states the reference also polymorphs the
// control towers (#465), closes the castle doors and spawns the siege
// guards (#3373).
func (s *Siege) startLocked(fx *effects) {
	if s.status == StatusInProgress {
		return
	}
	castleID := s.castle.ID
	owner := s.castle.OwnerID()
	if s.countLocked(SideAttacker) == 0 {
		id := MsgSiegeCanceledNoClans
		if owner <= 0 {
			id = MsgSiegeCanceledNoInterest
		}
		s.announceLocked(fx, castleMsg(id, castleID))
		s.saveSiegeLocked(fx, true)
		return
	}
	s.formerOwner = 0
	if _, ok := s.e.clans.Get(owner); ok {
		s.formerOwner = owner
	}
	s.status = StatusInProgress
	if f := s.field; f != nil {
		fx.add(func() {
			f.BanishForeigners(owner)
			f.SetActive(true)
		})
	}
	s.siegeStatesLocked(fx, false)
	s.announceLocked(fx, castleMsg(MsgSiegeStarted, castleID))
	if n := s.e.notify; n != nil {
		fx.add(func() { n.PlaySound(SoundSiegeStarted) })
	}
	s.tellLocked(fx, Message{ID: MsgTemporaryAlliance}, SideAttacker)
	s.endsAt = s.e.nowLocked().Add(s.e.cfg.Length).UnixMilli()
	s.clockGen++
	s.tickLocked(fx, s.clockGen)
}

// tickLocked is the siege clock's step, gen the generation it was armed
// with: past the last hour it tells both sides the time left, an hour,
// then 30, 10, 5 and 1 minutes, then each of the last ten seconds, and
// ends the siege when its time is up.
func (s *Siege) tickLocked(fx *effects, gen uint64) {
	if s.status != StatusInProgress || s.clockGen != gen {
		return
	}
	remaining := s.endsAt - s.e.nowLocked().UnixMilli()
	tell := func(m Message) { s.tellLocked(fx, m, SideAttacker, SideDefender) }
	var next int64
	switch {
	case remaining > 3_600_000:
		next = remaining - 3_600_000
	case remaining > 1_800_000:
		tell(numberMsg(MsgHoursUntilConclusion, 1))
		next = remaining - 1_800_000
	case remaining > tenMinutesMs:
		tell(numberMsg(MsgMinutesUntilConclusion, 30))
		next = remaining - tenMinutesMs
	case remaining > fiveMinutesMs:
		tell(numberMsg(MsgMinutesUntilConclusion, 10))
		next = remaining - fiveMinutesMs
	case remaining > 60_000:
		tell(numberMsg(MsgMinutesUntilConclusion, 5))
		next = remaining - 60_000
	case remaining > tenSecondsMs:
		tell(numberMsg(MsgMinutesUntilConclusion, 1))
		next = remaining - tenSecondsMs
	case remaining > 0:
		// Each of the last ten seconds: n seconds left while more than
		// n-1 remain.
		n := (remaining + 999) / 1000
		tell(numberMsg(MsgSecondsLeft, int32(n)))
		next = remaining - (n-1)*1000
	default:
		s.endLocked(fx)
		return
	}
	s.armClockLocked(time.Duration(next) * time.Millisecond)
}

func (s *Siege) armClockLocked(d time.Duration) {
	if s.e.queue == nil {
		return
	}
	gen := s.clockGen
	s.clock = s.e.queue.After(max(d, 0), func() {
		var fx effects
		s.e.mu.Lock()
		s.tickLocked(&fx, gen)
		s.e.mu.Unlock()
		fx.run()
	})
}

// End ends the siege now, as its time running out does.
func (s *Siege) End() {
	var fx effects
	s.e.mu.Lock()
	s.endLocked(&fx)
	s.e.mu.Unlock()
	fx.run()
}

// endLocked ends the siege under way. Everyone is told and hears it end,
// then who holds the castle: a clan that took it from another has the
// former owner lose the items the castle's owners wear and its nobles
// record the castle in their diaries; with no owner the siege is a draw.
// Every registered clan's siege kills and deaths are cleared, the
// reputation moves (see reputationLocked), the players of other clans
// than the owner's are thrown off the battlefield, the siege states are
// cleared, the next date is set and every registration dropped, the owner
// registered again, and the battlefield turns off.
//
// ponytail: the reference also drops each registered clan's headquarters
// flag with its counters (#3374), unpolymorphs the control towers (#465),
// despawns the siege guards and revives the doors (#3373) before the
// battlefield turns off, and resets the artifacts after (#2220).
func (s *Siege) endLocked(fx *effects) {
	if s.status != StatusInProgress {
		return
	}
	e := s.e
	castleID := s.castle.ID
	s.clockGen++
	if s.clock != nil {
		s.clock.Stop()
		s.clock = nil
	}
	s.announceLocked(fx, castleMsg(MsgSiegeEnded, castleID))
	if n := e.notify; n != nil {
		fx.add(func() { n.PlaySound(SoundSiegeEnded) })
	}
	ownerID := s.castle.OwnerID()
	owner, owned := e.clans.Get(ownerID)
	if ownerID > 0 && owned {
		s.announceLocked(fx, Message{ID: MsgClanVictoriousOverSiege, Text: owner.Name(), CastleID: castleID})
		if former, ok := e.clans.Get(s.formerOwner); ok && s.formerOwner != ownerID {
			if n := e.notify; n != nil {
				c := s.castle
				fx.add(func() { n.CastleTaken(c, owner, former) })
			}
		}
	} else {
		s.announceLocked(fx, castleMsg(MsgSiegeDraw, castleID))
	}
	for _, r := range s.clans {
		delete(e.counters, r.clanID)
	}
	s.reputationLocked(fx, ownerID)
	if f := s.field; f != nil {
		fx.add(func() { f.BanishForeigners(ownerID) })
	}
	s.siegeStatesLocked(fx, true)
	s.saveSiegeLocked(fx, true)
	s.clearAllLocked()
	if f := s.field; f != nil {
		fx.add(func() { f.SetActive(false) })
	}
}

// reputationLocked moves the reputation a siege's end decides: a former
// owner that lost the castle loses 1000 points and the new owner, if any,
// gains 1000; a former owner that held it gains 500; with no former owner,
// the clan that took the castle from the NPCs gains 1000.
func (s *Siege) reputationLocked(fx *effects, ownerID int32) {
	n := s.e.notify
	if n == nil {
		return
	}
	owner, owned := s.e.clans.Get(ownerID)
	former, hadFormer := s.e.clans.Get(s.formerOwner)
	switch {
	case hadFormer && s.formerOwner != ownerID:
		fx.add(func() { n.Reputation(former, -reputationWon, numberMsg(MsgClanDefeatedLostReputation, reputationWon)) })
		if owned {
			fx.add(func() { n.Reputation(owner, reputationWon, numberMsg(MsgClanVictoriousGainReputation, reputationWon)) })
		}
	case hadFormer:
		fx.add(func() {
			n.Reputation(former, reputationHeld, numberMsg(MsgClanVictoriousGainReputation, reputationHeld))
		})
	case owned:
		fx.add(func() { n.Reputation(owner, reputationWon, numberMsg(MsgClanVictoriousGainReputation, reputationWon)) })
	}
}

// MidVictory is the siege's turn when the castle changes hands while it is
// under way. With no owner left nothing else changes. Otherwise the
// attackers are told their temporary alliance is over, every owner and
// defender attacks, the new owner owns, the attackers of its alliance
// defend, the players of other clans are thrown off the battlefield, and
// every registered clan's members in the world take their new siege state.
//
// ponytail: the reference first respawns the siege guards (#3373), and
// before the new siege states drops the former defenders' headquarters
// flags (#3374), removes the door and trap upgrades (#236), revives the
// doors at half HP (#3373) and resets the control towers (#465).
func (s *Siege) MidVictory() {
	var fx effects
	e := s.e
	e.mu.Lock()
	s.midVictoryLocked(&fx)
	e.mu.Unlock()
	fx.run()
}

func (s *Siege) midVictoryLocked(fx *effects) {
	if s.status != StatusInProgress {
		return
	}
	ownerID := s.castle.OwnerID()
	if ownerID <= 0 {
		return
	}
	owner, ok := s.e.clans.Get(ownerID)
	if !ok {
		return
	}
	attackers := s.idsLocked(SideAttacker)
	s.tellLocked(fx, Message{ID: MsgTemporaryAllianceDissolved}, SideAttacker)
	for i := range s.clans {
		if s.clans[i].side == SideDefender || s.clans[i].side == SideOwner {
			s.clans[i].side = SideAttacker
		}
	}
	s.setSideLocked(ownerID, SideOwner)
	if allyID := owner.AllyID(); allyID != 0 {
		for _, id := range attackers {
			if cl, ok := s.e.clans.Get(id); ok && cl.AllyID() == allyID {
				s.setSideLocked(id, SideDefender)
			}
		}
	}
	if f := s.field; f != nil {
		fx.add(func() { f.BanishForeigners(ownerID) })
	}
	s.siegeStatesLocked(fx, false)
}

// siegeStatesLocked gives the members in the world of the attacking clans
// the attacker state and those of the owning and defending clans the
// defender state, or clears both when clear is set.
func (s *Siege) siegeStatesLocked(fx *effects, clear bool) {
	n := s.e.notify
	if n == nil {
		return
	}
	attackers := s.e.resolve(s.idsLocked(SideAttacker))
	defenders := s.e.resolve(s.idsLocked(SideOwner, SideDefender))
	attack, defend := StateAttacker, StateDefender
	if clear {
		attack, defend = StateNone, StateNone
	}
	fx.add(func() {
		n.SetSiegeState(attackers, attack)
		n.SetSiegeState(defenders, defend)
	})
}

// announceLocked tells every player in the world m.
func (s *Siege) announceLocked(fx *effects, m Message) {
	if n := s.e.notify; n != nil {
		fx.add(func() { n.Announce(m) })
	}
}

// tellLocked tells m to the members of the clans on each of sides in
// turn: SideAttacker for the attackers, SideDefender for the owner and the
// defenders.
func (s *Siege) tellLocked(fx *effects, m Message, sides ...Side) {
	n := s.e.notify
	if n == nil {
		return
	}
	for _, side := range sides {
		var clans []*clan.Clan
		switch side {
		case SideAttacker:
			clans = s.e.resolve(s.idsLocked(SideAttacker))
		case SideDefender:
			clans = s.e.resolve(s.idsLocked(SideOwner, SideDefender))
		default:
			continue
		}
		fx.add(func() { n.TellClans(clans, m) })
	}
}

// clearPendingLocked drops the clans still waiting to defend, and stores
// it.
func (s *Siege) clearPendingLocked() {
	castleID := int32(s.castle.ID)
	s.write("delete pending siege clans", func(ctx context.Context, st Store) error {
		return st.DeletePending(ctx, castleID)
	})
	kept := s.clans[:0]
	for _, r := range s.clans {
		if r.side != SidePending {
			kept = append(kept, r)
		}
	}
	s.clans = kept
}

// clearAllLocked drops every registration and stores it, then registers
// the castle's owner again.
func (s *Siege) clearAllLocked() {
	castleID := int32(s.castle.ID)
	s.write("delete siege clans", func(ctx context.Context, st Store) error {
		return st.DeleteClans(ctx, castleID)
	})
	s.clans = nil
	if owner := s.castle.OwnerID(); owner > 0 {
		if _, ok := s.e.clans.Get(owner); ok {
			s.setSideLocked(owner, SideOwner)
		}
	}
}
