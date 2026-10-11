package party

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Party.distributeItem(player, itemId, amount, false, monster)
// hands an auto-looted item to the looter through Player.addItem(itemId,
// amount, true), whose ItemContainer.addItem(itemId, count) creates count
// count-1 instances of a non-stackable under MultipleItemDrop, one without
// it (ItemContainer.java:224-253), and grows a held stackable without
// creating an instance.

// partyDropMonster drops count of itemID, always.
func partyDropMonster(itemID, count int32) *npc.Template {
	kind := item.DropNormal
	if itemID == item.AdenaID {
		kind = item.DropCurrency
	}
	return &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 20, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		Drops: []item.DropCategory{{Kind: kind, Chance: 100, Drops: []item.Drop{{ItemID: itemID, Min: count, Max: count, Chance: 100}}}},
	}
}

// killPartyDrop spawns tmpl beside the leader and kills it with the
// leader's hit, returning every player's frames.
func killPartyDrop(t *testing.T, g *group, tmpl *npc.Template) [][][]byte {
	t.Helper()
	x, y, z := g.srv.PlayerPosition(t, g.players[0].id)
	monster := g.srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: x + 60, Y: y, Z: z})
	g.quiet(t)
	if !monster.TakeDamage(1_000_000, g.combatant(t, 0)) {
		t.Fatal("leader's hit did not kill the monster")
	}
	frames := make([][][]byte, len(g.players))
	for i, p := range g.players {
		frames[i] = drainFrames(t, p.c)
	}
	return frames
}

// TestPartyAutoLootSplitsNonStackable: three auto-looted weapons go to the
// member whose turn it is as three instances under the shipped
// MultipleItemDrop, as one without it; either way the looter hears the
// whole amount and the killer hears who took it.
func TestPartyAutoLootSplitsNonStackable(t *testing.T) {
	for _, tt := range []struct {
		name string
		opts []gameservertest.Option
		want int
	}{
		{name: "multiple item drop", want: 3},
		{name: "single item drop", opts: []gameservertest.Option{gameservertest.WithSingleItemDrop()}, want: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opts := append([]gameservertest.Option{gameservertest.WithAutoLoot(true)}, tt.opts...)
			g := lootGroup(t, []seat{{"Leader", 20}, {"Member", 20}}, 2, party.LootByTurn, nil, opts...)
			frames := killPartyDrop(t, g, partyDropMonster(30, 3))

			if got := g.carried(t, 1, 30); got != tt.want {
				t.Fatalf("Member carries %d weapons, want %d", got, tt.want)
			}
			if got := g.carried(t, 0, 30); got != 0 {
				t.Fatalf("Leader carries %d weapons, want none", got)
			}
			if got := g.persisted(t, 1, 30); got != tt.want {
				t.Fatalf("Member's persisted weapons = %d, want %d", got, tt.want)
			}
			requireMessage(t, "Member", frames[1], serverpackets.SystemMessageYouPickedUpS2S1, int32(30), int32(3))
			requireMessage(t, "Leader", frames[0], serverpackets.SystemMessageS1ObtainedS3S2, "Member", int32(30), int32(3))
		})
	}
}

// TestPartyAdenaShareOntoHeldStacksTakesNoObjectID: the first auto-looted
// adena starts each member's stack, one object id each; the second grows
// those stacks and takes none.
func TestPartyAdenaShareOntoHeldStacksTakesNoObjectID(t *testing.T) {
	g := lootGroup(t, []seat{{"Leader", 20}, {"Member", 20}}, 2, party.LootByTurn, nil, gameservertest.WithAutoLoot(true))

	for kill, wantIDs := range []int{2, 0} {
		x, y, z := g.srv.PlayerPosition(t, g.players[0].id)
		monster := g.srv.SpawnHostileNPCTemplateAt(t, partyDropMonster(item.AdenaID, 10), location.Location{X: x + 60, Y: y, Z: z})
		g.quiet(t)
		before := g.srv.IssuedObjectIDs()
		if !monster.TakeDamage(1_000_000, g.combatant(t, 0)) {
			t.Fatal("leader's hit did not kill the monster")
		}
		for _, p := range g.players {
			drainFrames(t, p.c)
		}
		if got := g.srv.IssuedObjectIDs() - before; got != wantIDs {
			t.Fatalf("kill %d spent %d object ids, want %d", kill+1, got, wantIDs)
		}
		for i := range 2 {
			if got := g.carried(t, i, item.AdenaID); got != 5*(kill+1) {
				t.Fatalf("after kill %d %s carries %d adena, want %d", kill+1, g.players[i].name, got, 5*(kill+1))
			}
		}
	}
}
