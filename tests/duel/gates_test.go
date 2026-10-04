package duel

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
)

// Gates outside the fight that read a player's duel standing (#3285).
// Every refusal below comes before anything else the request looks at, so
// a request naming no recipe the player knows still shows the gate: in a
// duel it is refused with CANT_OPERATE_PRIVATE_STORE_DURING_COMBAT, out of
// one it drops without a word.

// unknownRecipe is a recipe id no recipe table holds.
const unknownRecipe = 999_999

func encodeRecipeItemMakeSelf(recipeID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestRecipeItemMakeSelf)
	w.WriteInt32(recipeID)
	return w.Bytes()
}

func encodeRecipeShopMakeItem(crafterID, recipeID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestRecipeShopMakeItem)
	w.WriteInt32(crafterID)
	w.WriteInt32(recipeID)
	w.WriteInt32(0)
	return w.Bytes()
}

// combatStanding is a live player's attack stance.
type combatStanding interface{ InCombat() bool }

// requireDuelNotCombat fails unless player i is in a duel without being in
// combat, so a refusal can only come from the duel.
func (a *arena) requireDuelNotCombat(t *testing.T, i int) {
	t.Helper()
	s := a.standing(t, i)
	if !s.InDuel() {
		t.Fatalf("player %d is not in a duel", i)
	}
	if s.(combatStanding).InCombat() {
		t.Fatalf("player %d is in combat; the duel gate would not be the one refusing", i)
	}
}

// endDuel has loser surrender and waits until every player left the duel,
// every client drained.
func (a *arena) endDuel(t *testing.T, loser int) {
	t.Helper()
	a.players[loser].c.Send(encodeDuelSurrender())
	a.srv.AdvanceUntil(t, "duel ends", func() bool {
		for i := range a.players {
			if a.standing(t, i).InDuel() {
				return false
			}
		}
		return true
	})
	a.quiet(t)
}

// requireRefusedInCombat fails unless frames carry
// CANT_OPERATE_PRIVATE_STORE_DURING_COMBAT.
func requireRefusedInCombat(t *testing.T, frames [][]byte) {
	t.Helper()
	requireMessage(t, frames, 0, serverpackets.SystemMessageCantOperateStoreDuringCombat)
}

// requireNoCombatRefusal fails when frames carry
// CANT_OPERATE_PRIVATE_STORE_DURING_COMBAT.
func requireNoCombatRefusal(t *testing.T, frames [][]byte) {
	t.Helper()
	if indexOfMessage(t, frames, 0, serverpackets.SystemMessageCantOperateStoreDuringCombat) >= 0 {
		t.Fatalf("refused as in combat outside a duel: %v", messages(t, frames))
	}
}

// TestDuelRefusesSelfCraft: RequestRecipeItemMakeSelf.java:34-38 refuses a
// craft while the player isInDuel(), before the recipe is looked up.
func TestDuelRefusesSelfCraft(t *testing.T) {
	t.Parallel()
	a := bootArena(t, "Crafter", "Rival")
	crafter := a.players[0]
	a.challenge(t, 0, 1)
	a.requireDuelNotCombat(t, 0)

	crafter.c.Send(encodeRecipeItemMakeSelf(unknownRecipe))
	requireRefusedInCombat(t, drainFrames(t, crafter.c))

	a.endDuel(t, 1)
	crafter.c.Send(encodeRecipeItemMakeSelf(unknownRecipe))
	requireNoCombatRefusal(t, drainFrames(t, crafter.c))
}

// TestDuelRefusesWorkshopOrder: RequestRecipeShopMakeItem.java:52 refuses
// an order when the crafter or the customer isInDuel(), with the message
// to the customer, before the recipe is looked up.
func TestDuelRefusesWorkshopOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		// duellist duels index 1; the other of 0 (customer) and 2
		// (crafter) stays out of it.
		duellist int
	}{
		{"customer in a duel", 0},
		{"crafter in a duel", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := bootArena(t, "Customer", "Rival", "Crafter")
			customer, crafter := a.players[0], a.players[2]
			a.challenge(t, tc.duellist, 1)
			a.requireDuelNotCombat(t, tc.duellist)
			// The crafter's workshop is open; the order names a recipe it
			// does not list, so only a gate before the recipe answers.
			a.srv.SetPlayerOperateType(t, crafter.id, privatestore.OperateManufacture)

			customer.c.Send(encodeRecipeShopMakeItem(crafter.id, unknownRecipe))
			requireRefusedInCombat(t, drainFrames(t, customer.c))
			if msgs := messages(t, drainFrames(t, crafter.c)); len(msgs) != 0 {
				t.Fatalf("crafter told %v, want nothing", msgs)
			}

			a.endDuel(t, 1)
			customer.c.Send(encodeRecipeShopMakeItem(crafter.id, unknownRecipe))
			requireNoCombatRefusal(t, drainFrames(t, customer.c))
		})
	}
}

// TestDuelRefusesPrivateStore: Player.canOpenPrivateStore (Player.java:2329)
// refuses a store while the player isInDuel(): the operate type goes back
// to NONE and CANT_OPERATE_PRIVATE_STORE_DURING_COMBAT answers. Once the
// duel is over the manage window opens.
func TestDuelRefusesPrivateStore(t *testing.T) {
	t.Parallel()
	a := bootArena(t, "Seller", "Rival")
	seller := a.players[0]
	a.challenge(t, 0, 1)
	a.requireDuelNotCombat(t, 0)
	// A manage window left open is not store mode, so the duel check runs
	// and closes it.
	a.srv.SetPlayerOperateType(t, seller.id, privatestore.OperateSellManage)

	seller.c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestPrivateStoreManageSell).Bytes())
	frames := drainFrames(t, seller.c)
	requireRefusedInCombat(t, frames)
	if at := indexOf(frames, 0, serverpackets.OpcodePrivateStoreManageListSell); at >= 0 {
		t.Fatal("sell manage window opened in a duel")
	}
	if got := a.srv.PlayerOperateType(t, seller.id); got != privatestore.OperateNone {
		t.Fatalf("operate type after the refusal = %d, want none", got)
	}

	a.endDuel(t, 1)
	seller.c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestPrivateStoreManageSell).Bytes())
	frames = drainFrames(t, seller.c)
	requireNoCombatRefusal(t, frames)
	if indexOf(frames, 0, serverpackets.OpcodePrivateStoreManageListSell) < 0 {
		t.Fatal("sell manage window did not open once the duel was over")
	}
}
