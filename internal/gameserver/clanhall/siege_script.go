package clanhall

import (
	"slices"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// SiegeBase is a hall siege as a step override of its script runs it: the
// siege and the base steps, which the override runs as part of its own. It
// is valid only while the override runs.
type SiegeBase struct {
	s *HallSiege
}

// Siege is the siege the step runs on.
func (b SiegeBase) Siege() *HallSiege { return b.s }

// Prepare runs the base close of the registrations: an owner that is a
// clan joins the attackers and the hall is remembered as owned, the hall is
// freed, everyone is thrown out of its grounds, everyone is told and the
// siege starts after delay.
func (b SiegeBase) Prepare(delay time.Duration) {
	b.s.hs.decide(func(fx *siegeEffects) { b.s.hs.prepareLocked(b.s.h, fx, delay) })
}

// Start runs the base start of the siege (see startLocked).
func (b SiegeBase) Start() {
	b.s.hs.decide(func(fx *siegeEffects) { b.s.hs.startLocked(b.s.h, fx) })
}

// End runs the base end of the siege, the script naming the winner first
// (see endLocked).
func (b SiegeBase) End() {
	var winner int32
	if sc := b.s.script(); sc != nil && sc.Winner != nil {
		winner = sc.Winner()
	}
	b.s.hs.decide(func(fx *siegeEffects) { b.s.hs.endLocked(b.s.h, fx, winner) })
}

// The operations below are the parts a script's own steps are made of. None
// takes a step: an override and a script's other hooks may run them.

// Name is the hall's name.
func (s *HallSiege) Name() string { return s.h.data.Name }

// OwnerID is the clan owning the hall, 0 for none.
func (s *HallSiege) OwnerID() int32 {
	s.hs.mu.Lock()
	defer s.hs.mu.Unlock()
	return s.h.ownerID
}

// Spawns are the hall's spawn points of kind.
func (s *HallSiege) Spawns(kind residence.SpawnType) []location.Location {
	return slices.Clone(s.h.data.Spawns[kind])
}

// SetOwner gives the hall to clanID, reporting false for no clan (see
// Halls.SetOwner).
func (s *HallSiege) SetOwner(clanID int32) bool {
	s.hs.mu.Lock()
	defer s.hs.mu.Unlock()
	cl, ok := s.hs.clans.Get(clanID)
	if !ok {
		return false
	}
	s.hs.setOwnerLocked(s.h, cl)
	return true
}

// Free takes the hall from its owner (see Halls.Free).
func (s *HallSiege) Free() {
	s.hs.mu.Lock()
	defer s.hs.mu.Unlock()
	s.hs.freeLocked(s.h)
}

// Registering reports whether the siege takes registrations.
func (s *HallSiege) Registering() bool { return s.Status() == siege.StatusRegistrationOpened }

// SiegeLength is how long the siege is fought.
func (s *HallSiege) SiegeLength() time.Duration {
	return time.Duration(s.h.data.SiegeLength) * time.Millisecond
}

// SetStatus sets where the siege stands in its cycle.
func (s *HallSiege) SetStatus(st siege.Status) {
	s.hs.mu.Lock()
	defer s.hs.mu.Unlock()
	s.hs.setSiege(func() { s.h.siege.status = st })
}

// UpdateNextSiege sets the next siege's date from now and stores it.
func (s *HallSiege) UpdateNextSiege() {
	s.hs.mu.Lock()
	defer s.hs.mu.Unlock()
	s.hs.updateNextSiegeLocked(s.h)
}

// AddAttacker makes clanID an attacker of the siege, and stores it,
// reporting false when no script runs the siege or the clan attacks it
// already.
func (s *HallSiege) AddAttacker(clanID int32) bool {
	hs := s.hs
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hs.siegeMu.Lock()
	defer hs.siegeMu.Unlock()
	st := s.h.siege
	if st.script == nil || clanID == 0 || slices.Contains(st.attackers, clanID) {
		return false
	}
	st.attackers = append(st.attackers, clanID)
	hs.saveAttackersLocked(s.h)
	return true
}

// RemoveAttacker drops clanID from the attackers, and stores it, whatever
// the siege's status.
func (s *HallSiege) RemoveAttacker(clanID int32) {
	hs := s.hs
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hs.setSiege(func() { hs.removeAttackerLocked(s.h, clanID) })
}

// ClearAttackers drops every attacker, and stores it.
func (s *HallSiege) ClearAttackers() {
	hs := s.hs
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hs.setSiege(func() {
		if len(s.h.siege.attackers) == 0 {
			return
		}
		s.h.siege.attackers = nil
		hs.saveAttackersLocked(s.h)
	})
}

// ReleaseAttackers lets the attackers go: each loses its headquarters and
// its members in the world their siege state, the players still on the
// battlefield are flagged and the attackers are dropped, and stored.
func (s *HallSiege) ReleaseAttackers() {
	s.hs.decide(func(fx *siegeEffects) { s.hs.releaseAttackersLocked(s.h, fx) })
}

// RespawnDoors stands the hall's doors again at full HP, closed.
func (s *HallSiege) RespawnDoors() {
	s.hs.decide(func(fx *siegeEffects) { s.hs.respawnDoorsLocked(s.h, fx) })
}

// BanishForeigners throws the players of other clans than the owner's out
// of the hall's grounds.
func (s *HallSiege) BanishForeigners() {
	s.hs.decide(func(fx *siegeEffects) { s.hs.banishLocked(s.h, fx, s.h.ownerID) })
}

// SetBattlefieldActive turns the siege's battlefield on or off.
func (s *HallSiege) SetBattlefieldActive(on bool) {
	s.withBattlefield(func(f *zone.Siege) { f.SetActive(on) })
}

// Announce tells every player in the world m.
func (s *HallSiege) Announce(m siege.Message) {
	s.hs.decide(func(fx *siegeEffects) { s.hs.announceLocked(fx, m) })
}

// ArmPrepare arms the registrations' close an hour before the siege's date,
// as the step armed last.
func (s *HallSiege) ArmPrepare() {
	s.hs.mu.Lock()
	defer s.hs.mu.Unlock()
	s.hs.armPrepareLocked(s.h)
}

// ArmStart arms the siege's start after d, as the step armed last.
func (s *HallSiege) ArmStart(d time.Duration) {
	s.hs.mu.Lock()
	defer s.hs.mu.Unlock()
	s.hs.armStartLocked(s.h, d)
}

// ArmEnd arms the siege's end after d, as the step armed last.
func (s *HallSiege) ArmEnd(d time.Duration) {
	s.hs.mu.Lock()
	defer s.hs.mu.Unlock()
	s.hs.armEndLocked(s.h, d)
}

// SetClanRestartPoint makes at where clanID's players restart to town
// while the siege's battlefield is on and holds them.
func (s *HallSiege) SetClanRestartPoint(clanID int32, at location.Location) {
	s.hs.siegeMu.Lock()
	defer s.hs.siegeMu.Unlock()
	st := s.h.siege
	if st.restartPoints == nil {
		st.restartPoints = map[int32]location.Location{}
	}
	st.restartPoints[clanID] = at
}

// DropClanRestartPoint drops clanID's restart point.
func (s *HallSiege) DropClanRestartPoint(clanID int32) {
	s.hs.siegeMu.Lock()
	defer s.hs.siegeMu.Unlock()
	delete(s.h.siege.restartPoints, clanID)
}

// ClanRestartPointAt returns where a player of clanID standing at (x, y, z)
// restarts to town: the restart point the hall siege whose battlefield is
// on there keeps for the clan. A clanID of 0 has none.
func (hs *Halls) ClanRestartPointAt(x, y, z int, clanID int32) (location.Location, bool) {
	if clanID == 0 {
		return location.Location{}, false
	}
	s, ok := hs.ActiveSiegeAt(x, y, z)
	if !ok {
		return location.Location{}, false
	}
	hs.siegeMu.Lock()
	defer hs.siegeMu.Unlock()
	at, ok := s.h.siege.restartPoints[clanID]
	return at, ok
}

// SiegableHallOfNPC returns the siegable hall whose NPCs list the template
// npcID.
func (hs *Halls) SiegableHallOfNPC(npcID int) (int32, bool) {
	if hs == nil {
		return 0, false
	}
	for _, h := range hs.sieges() {
		if slices.Contains(h.data.NPCs, npcID) {
			return h.id(), true
		}
	}
	return 0, false
}
