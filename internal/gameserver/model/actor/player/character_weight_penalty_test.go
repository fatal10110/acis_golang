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
