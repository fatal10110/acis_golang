package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

func bowConsumeItems() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: 14, Kind: item.KindWeapon, Slot: item.SlotLRHand, Weapon: &item.WeaponDetail{Type: item.WeaponBow, MPConsume: 4}},
		{ID: 17, Kind: item.KindEtcItem, Slot: item.SlotLHand, EtcItem: &item.EtcItemDetail{Type: item.EtcItemArrow}},
		{ID: 30, Kind: item.KindWeapon, Slot: item.SlotLRHand, Weapon: &item.WeaponDetail{Type: item.WeaponBow}},
	})
}

func TestConsumeBowShotSpendsOffhandArrowAndWeaponMP(t *testing.T) {
	bow := &item.Instance{ObjectID: 101, TemplateID: 14, Count: 1, Location: item.LocationPaperdoll, LocationData: itemcontainer.RHand, ManaLeft: -1}
	arrows := &item.Instance{ObjectID: 102, TemplateID: 17, Count: 5, Location: item.LocationPaperdoll, LocationData: itemcontainer.LHand, ManaLeft: -1}
	c := liveCharacter(1, combatTemplate(), bowConsumeItems(), bow, arrows)

	mpCalls := 0
	c.SetMPStatusBroadcaster(func() { mpCalls++ })
	c.ConsumeBowShot()
	c.ConsumeBowMP()

	if arrows.Count != 4 {
		t.Fatalf("arrow count = %d, want 4", arrows.Count)
	}
	if got := c.CurrentMP(); got != 26 {
		t.Fatalf("CurrentMP() = %d, want 26", got)
	}
	if mpCalls != 1 {
		t.Fatalf("MP status broadcasts = %d, want 1", mpCalls)
	}
}

func TestConsumeBowShotEmptyOffhandStillSpendsMP(t *testing.T) {
	bow := &item.Instance{ObjectID: 101, TemplateID: 14, Count: 1, Location: item.LocationPaperdoll, LocationData: itemcontainer.RHand, ManaLeft: -1}
	c := liveCharacter(1, combatTemplate(), bowConsumeItems(), bow)

	mpCalls := 0
	c.SetMPStatusBroadcaster(func() { mpCalls++ })
	c.ConsumeBowShot()
	c.ConsumeBowMP()

	if c.Inventory().ItemAt(itemcontainer.LHand) != nil {
		t.Fatal("empty off-hand grew an arrow stack")
	}
	if got := c.CurrentMP(); got != 26 {
		t.Fatalf("CurrentMP() = %d, want 26", got)
	}
	if mpCalls != 1 {
		t.Fatalf("MP status broadcasts = %d, want 1", mpCalls)
	}
}

func TestConsumeBowShotZeroMPCostSkipsStatusBroadcast(t *testing.T) {
	bow := &item.Instance{ObjectID: 101, TemplateID: 30, Count: 1, Location: item.LocationPaperdoll, LocationData: itemcontainer.RHand, ManaLeft: -1}
	arrows := &item.Instance{ObjectID: 102, TemplateID: 17, Count: 2, Location: item.LocationPaperdoll, LocationData: itemcontainer.LHand, ManaLeft: -1}
	c := liveCharacter(1, combatTemplate(), bowConsumeItems(), bow, arrows)

	mpCalls := 0
	c.SetMPStatusBroadcaster(func() { mpCalls++ })
	c.ConsumeBowShot()
	c.ConsumeBowMP()

	if arrows.Count != 1 {
		t.Fatalf("arrow count = %d, want 1", arrows.Count)
	}
	if got := c.CurrentMP(); got != 30 {
		t.Fatalf("CurrentMP() = %d, want 30", got)
	}
	if mpCalls != 0 {
		t.Fatalf("MP status broadcasts = %d, want 0", mpCalls)
	}
}
