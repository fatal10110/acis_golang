package castle

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// Doors is the world's doors as a castle's gate upgrades reach them.
type Doors interface {
	// DoorByName returns the door standing in the world named name.
	DoorByName(name string) (*door.Object, bool)
	// UpgradeDoor multiplies door id's template HP by ratio into its
	// maximum HP and fills its HP to it.
	UpgradeDoor(id, ratio int) bool
	// ResetDoorUpgrade takes door id's HP upgrade back to none.
	ResetDoorUpgrade(id int)
}

// DoorUpgrade is a castle_doorupgrade row: the HP ratio a castle upgraded
// one of its gates by.
type DoorUpgrade struct {
	CastleID, DoorID int32
	Ratio            int
}

// TrapUpgrade is a castle_trapupgrade row: the level a castle upgraded the
// trap of one of its control towers to, by the tower's castles.xml index.
type TrapUpgrade struct {
	CastleID   int32
	TowerIndex int
	Level      int
}

// SetDoors installs the world's doors the castles' gate upgrades reach,
// once at boot before RestoreUpgrades and before any player connects.
func (m *Manager) SetDoors(d Doors) { m.doors = d }

// RestoreUpgrades applies the stored upgrades, once at boot after SetDoors:
// each castle's control tower traps take their levels, then its gates their
// HP ratios. A trap row naming no control tower of its castle, and a gate
// row naming no gate of it, is left out, as is a row of a castle not
// loaded.
func (m *Manager) RestoreUpgrades(doors []DoorUpgrade, traps []TrapUpgrade) {
	for _, u := range traps {
		if c, ok := m.byID[int(u.CastleID)]; ok && !c.SetTrapUpgrade(u.TowerIndex, u.Level, false) {
			m.log.Warn().Int32("castle", u.CastleID).Int("tower", u.TowerIndex).Msg("castle: trap upgrade of no control tower left out")
		}
	}
	for _, u := range doors {
		if c, ok := m.byID[int(u.CastleID)]; ok {
			c.UpgradeDoor(int(u.DoorID), u.Ratio, false)
		}
	}
}

// gate returns the castle's gate with door id doorID standing in the
// world.
func (c *Castle) gate(doorID int) (*door.Object, bool) {
	if c.m.doors == nil {
		return nil, false
	}
	for _, name := range c.Gates {
		if d, ok := c.m.doors.DoorByName(name); ok && d.DoorID() == doorID {
			return d, true
		}
	}
	return nil, false
}

// GateUpgradeRatio returns the HP upgrade ratio of the castle's gate
// doorID, 1 for none; ok is false for a door that is not one of its gates.
func (c *Castle) GateUpgradeRatio(doorID int) (ratio int, ok bool) {
	d, ok := c.gate(doorID)
	if !ok {
		return 0, false
	}
	return d.HPUpgradeRatio(), true
}

// UpgradeDoor multiplies the template HP of the castle's gate doorID by
// ratio into its maximum HP and fills the gate's HP to it, storing the
// upgrade when save is set. A door that is not one of the castle's gates
// is left as it is, and it reports false.
func (c *Castle) UpgradeDoor(doorID, ratio int, save bool) bool {
	d, ok := c.gate(doorID)
	if !ok {
		return false
	}
	c.m.doors.UpgradeDoor(d.DoorID(), ratio)
	if save {
		u := DoorUpgrade{CastleID: c.id(), DoorID: int32(doorID), Ratio: ratio}
		c.write("save door upgrade", func(ctx context.Context, st Store) error {
			return st.SaveDoorUpgrade(ctx, u)
		})
	}
	return true
}

// RemoveDoorUpgrades takes every gate of the castle back to no HP upgrade,
// leaving their HP as it is, and deletes the stored upgrades.
func (c *Castle) RemoveDoorUpgrades() {
	if c.m.doors != nil {
		for _, name := range c.Gates {
			if d, ok := c.m.doors.DoorByName(name); ok {
				c.m.doors.ResetDoorUpgrade(d.DoorID())
			}
		}
	}
	id := c.id()
	c.write("delete door upgrades", func(ctx context.Context, st Store) error {
		return st.DeleteDoorUpgrades(ctx, id)
	})
}

// TrapUpgradeLevel returns the level the trap of the castle's control tower
// at towerIndex, in castles.xml order, is upgraded to; 0 for none or for an
// index naming no tower.
func (c *Castle) TrapUpgradeLevel(towerIndex int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if towerIndex < 0 || towerIndex >= len(c.trapLevels) {
		return 0
	}
	return c.trapLevels[towerIndex]
}

// TrapUpgrade returns the level the trap of the castle's control tower at
// towerIndex is upgraded to; ok is false for an index naming no tower.
func (c *Castle) TrapUpgrade(towerIndex int) (level int, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if towerIndex < 0 || towerIndex >= len(c.trapLevels) {
		return 0, false
	}
	return c.trapLevels[towerIndex], true
}

// SetTrapUpgrade upgrades the trap of the castle's control tower at
// towerIndex to level, storing it when save is set. An index naming no
// control tower changes and stores nothing, and it reports false.
func (c *Castle) SetTrapUpgrade(towerIndex, level int, save bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if towerIndex < 0 || towerIndex >= len(c.trapLevels) {
		return false
	}
	c.trapLevels[towerIndex] = level
	if save {
		u := TrapUpgrade{CastleID: c.id(), TowerIndex: towerIndex, Level: level}
		c.write("save trap upgrade", func(ctx context.Context, st Store) error {
			return st.SaveTrapUpgrade(ctx, u)
		})
	}
	return true
}

// RemoveTrapUpgrades takes every control tower trap of the castle back to
// level 0 and deletes the stored upgrades.
func (c *Castle) RemoveTrapUpgrades() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.trapLevels)
	id := c.id()
	c.write("delete trap upgrades", func(ctx context.Context, st Store) error {
		return st.DeleteTrapUpgrades(ctx, id)
	})
}

// Purchase runs fn alone among the castle's purchases: its gate and trap
// upgrades, its certificate sales and its vault moves, each a check of the
// castle's state, a payment and the change it buys. fn must not start
// another purchase.
func (c *Castle) Purchase(fn func()) {
	c.purchase.Lock()
	defer c.purchase.Unlock()
	fn()
}

// DoorUpgradeCost is the adena a castle pays to raise a gate's HP ratio to
// ratio: gateType 1 is a plain gate, 2 a metal gate, 3 a wall, each priced
// for ratios 2, 3 and 5. The price is tripled while the dusk owns the Seal
// of Strife and cut by a fifth while the dawn does. An unpriced type or
// ratio costs 0.
func DoorUpgradeCost(gateType, ratio int, strifeOwner sevensigns.Cabal) int {
	prices := map[int][3]int{
		1: {300_000, 400_000, 500_000},
		2: {750_000, 900_000, 1_000_000},
		3: {1_600_000, 1_800_000, 2_000_000},
	}
	price := 0
	if row, ok := prices[gateType]; ok {
		switch ratio {
		case 2:
			price = row[0]
		case 3:
			price = row[1]
		case 5:
			price = row[2]
		}
	}
	return strifePrice(price, strifeOwner)
}

// TrapUpgradeCost is the adena a castle pays to raise a control tower's
// trap to level 1 to 4, priced as DoorUpgradeCost prices under the Seal of
// Strife. Any other level costs 0.
func TrapUpgradeCost(level int, strifeOwner sevensigns.Cabal) int {
	price := 0
	if level >= 1 && level <= 4 {
		price = 2_000_000 + level*1_000_000
	}
	return strifePrice(price, strifeOwner)
}

// strifePrice is price tripled under a dusk Seal of Strife and cut by a
// fifth, truncated, under a dawn one.
func strifePrice(price int, strifeOwner sevensigns.Cabal) int {
	switch strifeOwner {
	case sevensigns.Dusk:
		return price * 3
	case sevensigns.Dawn:
		return int(float64(price) * 0.8)
	case sevensigns.NoCabal:
	}
	return price
}
