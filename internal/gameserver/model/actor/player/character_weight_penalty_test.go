package player

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

func TestRefreshWeightPenalty(t *testing.T) {
	tests := []struct {
		name  string
		ratio float64
		want  int
	}{
		{"below half", .499, 0},
		{"half", .5, 1},
		{"two thirds", .666, 2},
		{"four fifths", .8, 3},
		{"full", 1, 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inv := itemcontainer.NewPlayerInventory(1, item.NewTable([]*item.Template{{ID: 1, Kind: item.KindEtcItem, Weight: 1, Stackable: true, EtcItem: &item.EtcItemDetail{}}}))
			c := &Character{}
			c.AttachRuntime(&Template{CON: 20}, inv)
			c.SetWeightLimitMultiplier(1)
			inv.AddNew(1, int(math.Ceil(float64(c.WeightLimit())*tc.ratio)), 1)
			inv.UpdateWeight()
			c.RefreshWeightPenalty()
			if got := c.WeightPenalty(); got != tc.want {
				t.Fatalf("WeightPenalty() = %d, want %d (limit %d weight %d)", got, tc.want, c.WeightLimit(), c.CurrentWeight())
			}
		})
	}
}

func TestRefreshWeightPenaltyChangesOnlyOnBandChange(t *testing.T) {
	inv := itemcontainer.NewPlayerInventory(1, item.NewTable([]*item.Template{{ID: 1, Kind: item.KindEtcItem, Weight: 1, Stackable: true, EtcItem: &item.EtcItemDetail{}}}))
	c := &Character{}
	c.AttachRuntime(&Template{CON: 20}, inv)
	c.SetWeightLimitMultiplier(1)
	inv.AddNew(1, c.WeightLimit()/2, 1)
	inv.UpdateWeight()
	updates := 0
	c.SetWeightPenaltyUpdater(func() { updates++ })

	c.RefreshWeightPenalty()
	c.RefreshWeightPenalty()
	if updates != 1 {
		t.Fatalf("updates after unchanged refresh = %d, want 1", updates)
	}

	inv.AddNew(1, c.WeightLimit()/6, 2)
	inv.UpdateWeight()
	c.RefreshWeightPenalty()
	if got, want := c.WeightPenalty(), 2; got != want {
		t.Fatalf("WeightPenalty() = %d, want %d", got, want)
	}
	if updates != 2 {
		t.Fatalf("updates after band change = %d, want 2", updates)
	}
}

// TestWeightPenaltySpeedMultiplier pins the per-band speed multiplier to
// WeightPenalty.java:5-9 (NONE 1, LEVEL_1 1, LEVEL_2 0.5, LEVEL_3 0.5,
// LEVEL_4 0). LEVEL_4 must be 0, not 0.5 — a fully overloaded reference
// player cannot move (PlayerStatus.getMoveSpeed, PlayerStatus.java:944-947).
func TestWeightPenaltySpeedMultiplier(t *testing.T) {
	tests := []struct {
		band int
		want float64
	}{
		{0, 1}, {1, 1}, {2, .5}, {3, .5}, {4, 0},
	}
	for _, tc := range tests {
		c := &Character{}
		c.stateMu.Lock()
		c.weightPenalty = tc.band
		c.stateMu.Unlock()
		if got := c.weightPenaltySpeedMultiplier(); got != tc.want {
			t.Errorf("band %d: weightPenaltySpeedMultiplier() = %v, want %v", tc.band, got, tc.want)
		}
	}
}

// TestAddLevelRefreshesWeightPenalty pins PlayerStatus.addLevel's direct
// call to _actor.refreshWeightPenalty() on every level change
// (PlayerStatus.java:644, before the UserInfo send at :648). The band is
// forced stale beforehand so a passing test proves AddLevel actually
// recomputed it rather than leaving it untouched.
func TestAddLevelRefreshesWeightPenalty(t *testing.T) {
	inv := itemcontainer.NewPlayerInventory(1, item.NewTable([]*item.Template{{ID: 1, Kind: item.KindEtcItem, Weight: 1, Stackable: true, EtcItem: &item.EtcItemDetail{}}}))
	c := &Character{CharLevel: 1}
	c.AttachRuntime(&Template{CON: 20}, inv)
	c.SetWeightLimitMultiplier(1)
	inv.AddNew(1, c.WeightLimit(), 1) // full overload -> band 4
	inv.UpdateWeight()

	c.stateMu.Lock()
	c.weightPenalty = 0 // stale: as if never refreshed
	c.stateMu.Unlock()

	table, err := NewLevelTable(map[int]Level{1: {RequiredExpToLevelUp: 0}, 2: {RequiredExpToLevelUp: 100}, 3: {RequiredExpToLevelUp: 200}})
	if err != nil {
		t.Fatalf("NewLevelTable: %v", err)
	}
	c.AddLevel(table, nil, 1)

	if got, want := c.WeightPenalty(), 4; got != want {
		t.Fatalf("WeightPenalty() after AddLevel = %d, want %d (stale band never recomputed)", got, want)
	}
}

func TestRefreshWeightPenaltyKeepsStateWhenLimitIsZero(t *testing.T) {
	inv := itemcontainer.NewPlayerInventory(1, item.NewTable([]*item.Template{{ID: 1, Kind: item.KindEtcItem, Weight: 1, Stackable: true, EtcItem: &item.EtcItemDetail{}}}))
	c := &Character{}
	c.AttachRuntime(&Template{CON: 20}, inv)
	c.SetWeightLimitMultiplier(1)
	inv.AddNew(1, c.WeightLimit(), 1)
	inv.UpdateWeight()
	c.RefreshWeightPenalty()
	c.SetWeightLimitMultiplier(0)
	c.RefreshWeightPenalty()
	if got, want := c.WeightPenalty(), 4; got != want {
		t.Fatalf("WeightPenalty() after zero limit = %d, want %d", got, want)
	}
}
