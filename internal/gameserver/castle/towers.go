package castle

import (
	"slices"

	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
)

// Tower is one of a castle's control towers as it stands in the world.
type Tower interface {
	// TowerLife reports a life control tower.
	TowerLife() bool
	// TowerWorking reports a tower in its working form.
	TowerWorking() bool
	// PolymorphTower puts the tower in its working form.
	PolymorphTower()
	// UnpolymorphTower stands the tower dead at full HP.
	UnpolymorphTower()
	// TowerMidVictory is the tower's turn when the castle changes hands
	// while its siege is fought.
	TowerMidVictory()
}

// SpawnTowers places every castle's control towers once, at boot, before
// any player connects, castle by castle in castles.xml order. spawn places
// one for the castle castleID and returns it, false when it placed none.
// Each castle keeps its placed towers, in castles.xml order, for its whole
// life. It returns how many were placed.
func (m *Manager) SpawnTowers(spawn func(castleID int, t castledata.ControlTower) (Tower, bool)) int {
	placed := 0
	for _, c := range m.order {
		for _, t := range c.ControlTowers {
			tower, ok := spawn(c.ID, t)
			if !ok {
				continue
			}
			c.mu.Lock()
			c.towers = append(c.towers, tower)
			c.mu.Unlock()
			placed++
		}
	}
	return placed
}

// Towers returns the castle's placed control towers, in castles.xml order.
func (c *Castle) Towers() []Tower {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.towers)
}

// AliveLifeTowers counts the castle's placed life control towers standing:
// every one outside a siege, the working ones while it is fought.
func (c *Castle) AliveLifeTowers(siegeInProgress bool) int {
	n := 0
	for _, t := range c.Towers() {
		if t.TowerLife() && (!siegeInProgress || t.TowerWorking()) {
			n++
		}
	}
	return n
}
