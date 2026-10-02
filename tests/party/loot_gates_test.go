package party

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Party.getValidLooter (alive, validateCapacityByItemId(id, 1),
// in party range) and PlayerAI.thinkPickUp, which checks the picker's own
// capacity only outside a party or under ITEM_LOOTER, before the herb and
// cursed weapon branches; PcInventory.validateCapacity passes any check for
// no slot, which is all a herb needs.

const lootHerbID int32 = 8600

// TestPartyPickupPassesOverDeadAndFullMembers: the random and by-turn rules
// never hand an item to a dead member or to one with no room for it; with
// nobody else to take it, the picker keeps it.
func TestPartyPickupPassesOverDeadAndFullMembers(t *testing.T) {
	for name, rule := range map[string]party.LootRule{"random": party.LootRandom, "by turn": party.LootByTurn} {
		t.Run(name, func(t *testing.T) {
			g := lootGroup(t, []seat{{"Leader", 20}, {"Dead", 20}, {"Full", 20}}, 3, rule, nil)
			g.srv.MarkPlayerDead(t, g.players[1].id)
			g.srv.SetInventorySlotLimit(t, g.players[2].id, 0)

			frames := g.pickUp(t, 0, 0, lootPotionID, 2)
			for i, want := range []int{2, 0, 0} {
				if got := g.carried(t, i, lootPotionID); got != want {
					t.Fatalf("%s carries %d potions, want %d", g.players[i].name, got, want)
				}
			}
			requireMessage(t, "Leader", frames[0], serverpackets.SystemMessageYouPickedUpS2S1, lootPotionID, int32(2))
			for i := 1; i < 3; i++ {
				requireMessage(t, g.players[i].name, frames[i], serverpackets.SystemMessageS1ObtainedS3S2, "Leader", lootPotionID, int32(2))
			}
		})
	}
}

// TestPartyPickupPickerCapacity: under finders-keepers a full picker is
// refused with SLOTS_FULL and the item stays on the ground. Under random or
// by turn its own room is not checked: with no member able to take the
// item, the full picker keeps it.
func TestPartyPickupPickerCapacity(t *testing.T) {
	t.Run("finders keepers", func(t *testing.T) {
		g := lootGroup(t, []seat{{"Leader", 20}, {"Member", 20}}, 2, party.LootFindersKeepers, nil)
		g.srv.SetInventorySlotLimit(t, g.players[0].id, 0)

		frames := g.pickUp(t, 0, 0, lootPotionID, 2)
		requireMessage(t, "Leader", frames[0], serverpackets.SystemMessageSlotsFull)
		if got := g.carried(t, 0, lootPotionID); got != 0 {
			t.Fatalf("the full Leader carries %d potions, want none", got)
		}
		if len(g.srv.GroundItems.Snapshots(nil)) != 1 {
			t.Fatal("a refused pickup left the ground")
		}
		requireNoMessage(t, "Member", frames[1], serverpackets.SystemMessageS1ObtainedS3S2)
	})
	for name, rule := range map[string]party.LootRule{"random": party.LootRandom, "by turn": party.LootByTurn} {
		t.Run(name, func(t *testing.T) {
			g := lootGroup(t, []seat{{"Leader", 20}, {"Member", 20}}, 2, rule, []int{1})
			g.srv.SetInventorySlotLimit(t, g.players[0].id, 0)

			frames := g.pickUp(t, 0, 0, lootPotionID, 2)
			requireNoMessage(t, "Leader", frames[0], serverpackets.SystemMessageSlotsFull)
			if got := g.carried(t, 0, lootPotionID); got != 2 {
				t.Fatalf("the full Leader carries %d potions, want the 2 nobody else could take", got)
			}
			if len(g.srv.GroundItems.Snapshots(nil)) != 0 {
				t.Fatal("the picked-up stack is still on the ground")
			}
		})
	}
}

// TestPartyHerbPickupIgnoresPickerCapacity: a herb needs no slot, and a
// capacity check for no slot always passes, so under every loot rule even a
// picker already past its slot limit uses the herb it picks up.
func TestPartyHerbPickupIgnoresPickerCapacity(t *testing.T) {
	for name, rule := range map[string]party.LootRule{"finders keepers": party.LootFindersKeepers, "random": party.LootRandom, "by turn": party.LootByTurn} {
		t.Run(name, func(t *testing.T) {
			g := lootGroup(t, []seat{{"Leader", 20}, {"Member", 20}}, 2, rule, []int{1})
			// The Leader keeps a stack nobody else is near enough to take,
			// then its limit drops below what it holds.
			g.pickUp(t, 0, 0, lootPotionID, 1)
			if got := g.carried(t, 0, lootPotionID); got != 1 {
				t.Fatalf("Leader carries %d potions, want 1", got)
			}
			g.srv.SetInventorySlotLimit(t, g.players[0].id, 0)

			frames := g.pickUp(t, 0, 0, lootHerbID, 1)
			requireNoMessage(t, "Leader", frames[0], serverpackets.SystemMessageSlotsFull)
			if len(g.srv.GroundItems.Snapshots(nil)) != 0 {
				t.Fatal("the Leader past its limit could not use the herb")
			}
			for i := range 2 {
				if got := g.carried(t, i, lootHerbID); got != 0 {
					t.Fatalf("%s carries %d herbs, want none", g.players[i].name, got)
				}
			}
		})
	}
}

// TestPartyPickupCursedWeaponStaysWithPicker: a cursed weapon skips the
// party's loot rule. The picker keeps it, even under by turn and even
// with a full inventory, and the other members hear nothing.
func TestPartyPickupCursedWeaponStaysWithPicker(t *testing.T) {
	const cursedID int32 = 30
	table, err := entity.NewCursedWeaponTable([]entity.CursedWeapon{{ItemID: cursedID}})
	if err != nil {
		t.Fatalf("NewCursedWeaponTable: %v", err)
	}
	g := lootGroup(t, []seat{{"Leader", 20}, {"Member", 20}}, 2, party.LootByTurn, nil, gameservertest.WithCursedWeapons(table))
	g.srv.SetInventorySlotLimit(t, g.players[0].id, 0)

	frames := g.pickUp(t, 0, 0, cursedID, 1)
	if got := g.carried(t, 0, cursedID); got != 1 {
		t.Fatalf("Leader carries %d cursed weapons, want the one it picked up", got)
	}
	if got := g.carried(t, 1, cursedID); got != 0 {
		t.Fatalf("Member carries %d cursed weapons, want none", got)
	}
	requireNoMessage(t, "Leader", frames[0], serverpackets.SystemMessageSlotsFull)
	requireNoMessage(t, "Member", frames[1], serverpackets.SystemMessageS1ObtainedS2)
}
