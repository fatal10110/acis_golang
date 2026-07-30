package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// TestConsumeHerbReportsWhetherAConsumerTookIt pins the result a herb
// deliverer needs: a detached character consumes nothing, and saying so lets
// the caller deliver the herb another way instead of discarding it.
func TestConsumeHerbReportsWhetherAConsumerTookIt(t *testing.T) {
	c := &Character{ID: 1}

	if c.ConsumeHerb(8600) {
		t.Fatal("ConsumeHerb() = true with no consumer wired")
	}

	var consumed []int32
	c.SetHerbConsumer(func(itemID int32) { consumed = append(consumed, itemID) })
	if !c.ConsumeHerb(8600) {
		t.Fatal("ConsumeHerb() = false with a consumer wired")
	}
	if len(consumed) != 1 || consumed[0] != 8600 {
		t.Fatalf("consumed = %v, want [8600]", consumed)
	}

	c.SetHerbConsumer(nil)
	if c.ConsumeHerb(8600) {
		t.Fatal("ConsumeHerb() = true after detach")
	}
	if len(consumed) != 1 {
		t.Fatalf("consumed = %v, want no further consumption after detach", consumed)
	}
}

// TestAddRewardItemNotifiesTheUpdateHook pins the delivery half of an
// auto-looted kill reward: the mutation methods stay silent because they also
// serve client requests, so this server-driven caller is the one that has to
// register the inventory with the batching task.
func TestAddRewardItemNotifiesTheUpdateHook(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 57, Name: "adena", Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	c := &Character{ID: 1}
	inv := itemcontainer.RestorePlayerInventory(c.ID, templates, nil)
	c.AttachRuntime(&Template{}, inv)

	notified := 0
	inv.SetUpdateNotifier(func() { notified++ })

	if !c.AddRewardItem(57, 10, 0x30000001) {
		t.Fatal("AddRewardItem() = false for a known stackable template")
	}
	if notified != 1 {
		t.Fatalf("notifier calls = %d, want 1", notified)
	}

	if c.AddRewardItem(9999, 1, 0x30000002) {
		t.Fatal("AddRewardItem() = true for an unknown template")
	}
	if notified != 1 {
		t.Fatalf("notifier calls after a rejected add = %d, want 1", notified)
	}
}
