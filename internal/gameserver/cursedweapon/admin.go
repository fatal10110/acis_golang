package cursedweapon

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Info is one weapon as the game masters' panel lists it.
type Info struct {
	ItemID int32
	Name   string
	// Activated: the weapon is held, by HolderID, who had Karma and
	// PKKills before it.
	Activated bool
	HolderID  int32
	Karma     int32
	PKKills   int32
	// Dropped: the weapon lies on the ground, at GroundAt when
	// HasGroundAt.
	Dropped     bool
	GroundAt    location.Location
	HasGroundAt bool
	// Out: the weapon is held, on the ground, or being handed out.
	Out   bool
	Stage int32
	// Kills are the kills toward the next stage, which takes NextStageKills.
	Kills          int32
	NextStageKills int32
	// HungryMinutes are the minutes left before the weapon ends for want
	// of a kill.
	HungryMinutes int32
	// TimeLeft is how long the weapon has left before it ends.
	TimeLeft time.Duration
}

// Weapons lists every weapon, out or not, in the reference's order.
func (m *Manager) Weapons() []Info {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	nowMs := m.now().UnixMilli()
	out := make([]Info, 0, len(m.order))
	for _, w := range m.order {
		out = append(out, Info{
			ItemID:         w.def.ItemID,
			Name:           w.def.Name,
			Activated:      w.activated,
			HolderID:       w.playerID,
			Karma:          w.playerKarma,
			PKKills:        w.playerPK,
			Dropped:        w.dropped,
			GroundAt:       w.groundAt,
			HasGroundAt:    w.dropped && w.hasGroundAt,
			Out:            w.active(),
			Stage:          w.stage,
			Kills:          w.nbKills,
			NextStageKills: w.nextAt,
			HungryMinutes:  w.hungry,
			TimeLeft:       time.Duration(w.endTime-nowMs) * time.Millisecond,
		})
	}
	return out
}

// End ends itemID's weapon wherever it is, out or not: endOfLife, which a
// game master's removal runs. ok is false for an item that is no cursed
// weapon.
func (m *Manager) End(itemID int32) (EndOfLife, bool) {
	if m == nil {
		return EndOfLife{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	w := m.byID[itemID]
	if w == nil {
		return EndOfLife{}, false
	}
	return m.endOfLifeLocked(w), true
}

// Reserve claims itemID's weapon, not out yet, for a game master to hand
// out: it counts as out, so no drop brings it out meanwhile, until
// Activate makes the player given it its holder or Unreserve hands it back.
// It reports false when the weapon is out already or is no cursed weapon.
func (m *Manager) Reserve(itemID int32) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	w := m.byID[itemID]
	if w == nil || w.active() {
		return false
	}
	w.reserved = true
	return true
}

// Unreserve hands back itemID's weapon when it is still reserved: it was
// never given.
func (m *Manager) Unreserve(itemID int32) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if w := m.byID[itemID]; w != nil && w.reserved {
		w.reserved = false
	}
}

// StartLife starts the full life of itemID's weapon, which a game master
// just gave holderID: its hunger and its end time count from now, and its
// life check runs every minute (CursedWeapon.reActivate(true)). It reports
// false, changing nothing, when holderID does not hold it.
func (m *Manager) StartLife(itemID, holderID int32) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	w := m.byID[itemID]
	if w == nil || !w.activated || w.playerID != holderID {
		return false
	}
	now := m.now()
	w.hungry = int32(w.def.DurationLost * 60)
	w.endTime = now.Add(time.Duration(w.def.Duration) * time.Hour).UnixMilli()
	w.overallAt = now.Add(minute)
	return true
}

// Reload ends every weapon, out or not, in the reference's order, then
// takes table's weapons in place of the ones it had, none of them out
// (CursedWeaponManager.reload). It returns the ends for the caller to
// apply.
func (m *Manager) Reload(table *entity.CursedWeaponTable) []EndOfLife {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	ends := make([]EndOfLife, 0, len(m.order))
	for _, w := range m.order {
		ends = append(ends, m.endOfLifeLocked(w))
	}
	m.byID, m.order = weapons(table)
	return ends
}
