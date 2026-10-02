package party

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Party.distributeItem / distributeAdena / getValidLooter and
// Player.isLooterOrInLooterParty, reached from PlayerAI's ground pickup
// (distributeItem(player, item, null)) and Monster.dropOrAutoLootItem
// (distributeItem(player, itemId, amount, false, monster)). A partied
// player's adena is split evenly among the members in party range of the
// picker (or the monster) whose adena is not at its cap, the remainder
// lost; anything else goes to the looter the rule picks among the members
// that are alive, have room and are in range, or the picker when none is,
// and every other member hears S1_OBTAINED_S3_S2 (299), S1_OBTAINED_S2_S3
// (376) or S1_OBTAINED_S2 (300) naming the looter.

const lootPotionID int32 = 20

// sysMsg is one decoded SystemMessage.
type sysMsg struct {
	id     int32
	params []any // string for text, int32 otherwise
}

func systemMessages(frames [][]byte) []sysMsg {
	var out []sysMsg
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wire.NewReader(f[1:])
		m := sysMsg{id: r.ReadInt32()}
		for range r.ReadInt32() {
			if r.ReadInt32() == serverpackets.SystemMessageParamText {
				m.params = append(m.params, r.ReadString())
			} else {
				m.params = append(m.params, r.ReadInt32())
			}
		}
		out = append(out, m)
	}
	return out
}

// requireMessage fails unless frames carry exactly one SystemMessage id,
// with params.
func requireMessage(t *testing.T, who string, frames [][]byte, id int, params ...any) {
	t.Helper()
	var found []sysMsg
	for _, m := range systemMessages(frames) {
		if m.id == int32(id) {
			found = append(found, m)
		}
	}
	if len(found) != 1 || !slices.Equal(found[0].params, params) {
		t.Fatalf("%s: messages %d = %+v, want one with %v", who, id, found, params)
	}
}

func requireNoMessage(t *testing.T, who string, frames [][]byte, ids ...int) {
	t.Helper()
	for _, m := range systemMessages(frames) {
		if slices.Contains(ids, int(m.id)) {
			t.Fatalf("%s: got message %d %v, want none of %v", who, m.id, m.params, ids)
		}
	}
}

func encodeAction(objectID int32, x, y, z int) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAction)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(x))
	w.WriteInt32(int32(y))
	w.WriteInt32(int32(z))
	w.WriteUint8(0)
	return w.Bytes()
}

// lootGroup boots seats, forms a party of every seat in party with the
// first as leader under rule, and moves every seat in far out of party
// range of the leader.
func lootGroup(t *testing.T, seats []seat, inParty int, rule party.LootRule, far []int, opts ...gameservertest.Option) *group {
	t.Helper()
	g := bootGroup(t, seats, opts...)
	for i := 1; i < inParty; i++ {
		g.invite(t, 0, i, int32(rule))
	}
	x, y, z := g.srv.PlayerPosition(t, g.players[0].id)
	for _, i := range far {
		obj, _ := g.srv.State.Player(g.players[i].id)
		c, ok := network.OnlineCharacter(obj)
		if !ok {
			t.Fatalf("%s is %T", g.players[i].name, obj)
		}
		c.TeleportTo(x+3000, y, z, 0)
	}
	g.quiet(t)
	return g
}

// pickUp seeds count of templateID owned by ownerID at the picker's feet,
// has the picker click it, and returns every player's frames.
func (g *group) pickUp(t *testing.T, picker int, ownerID, templateID, count int32) [][][]byte {
	t.Helper()
	x, y, z := g.srv.PlayerPosition(t, g.players[picker].id)
	g.srv.SeedGroundItem(t, ownerID, templateID, count, x, y, z)
	g.quiet(t)
	snaps := g.srv.GroundItems.Snapshots(nil)
	if len(snaps) != 1 {
		t.Fatalf("ground items = %d, want 1", len(snaps))
	}
	g.players[picker].c.Send(encodeAction(snaps[0].ObjectID, x, y, z))
	out := make([][][]byte, len(g.players))
	for i, p := range g.players {
		out[i] = drainFrames(t, p.c)
	}
	return out
}

func (g *group) carried(t *testing.T, i int, templateID int32) int {
	t.Helper()
	return g.srv.PlayerInventory(t, g.players[i].id).ItemCount(templateID, -1, true)
}

func (g *group) persisted(t *testing.T, i int, templateID int32) int {
	t.Helper()
	g.srv.FlushItems(t)
	rows, err := g.srv.Items.ListByOwner(context.Background(), g.players[i].id)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	n := 0
	for _, row := range rows {
		if row.TemplateID == templateID {
			n += row.Count
		}
	}
	return n
}

// TestPartyPickupByTurnGoesToNextMember: under the by-turn rule the
// leader's pickup goes to the member after the last looter, the turn
// starting at the first member, then comes back to the leader. The picker
// hears who took it; the looter hears it picked the stack up.
func TestPartyPickupByTurnGoesToNextMember(t *testing.T) {
	g := lootGroup(t, []seat{{"Leader", 20}, {"Member", 20}}, 2, party.LootByTurn, nil)

	frames := g.pickUp(t, 0, 0, lootPotionID, 5)
	if got := g.carried(t, 1, lootPotionID); got != 5 {
		t.Fatalf("Member carries %d potions, want the 5 picked up", got)
	}
	if got := g.carried(t, 0, lootPotionID); got != 0 {
		t.Fatalf("Leader carries %d potions, want none", got)
	}
	if got := g.persisted(t, 1, lootPotionID); got != 5 {
		t.Fatalf("Member's stored potions = %d, want 5", got)
	}
	if len(g.srv.GroundItems.Snapshots(nil)) != 0 {
		t.Fatal("the picked-up stack is still on the ground")
	}
	requireMessage(t, "Leader", frames[0], serverpackets.SystemMessageS1ObtainedS3S2, "Member", lootPotionID, int32(5))
	requireMessage(t, "Member", frames[1], serverpackets.SystemMessageYouPickedUpS2S1, lootPotionID, int32(5))
	requireNoMessage(t, "Member", frames[1], serverpackets.SystemMessageS1ObtainedS3S2)
	requireNoMessage(t, "Leader", frames[0], serverpackets.SystemMessageYouPickedUpS2S1)

	// The next turn is the leader's own, whoever picks.
	frames = g.pickUp(t, 1, 0, lootPotionID, 1)
	if got := g.carried(t, 0, lootPotionID); got != 1 {
		t.Fatalf("Leader carries %d potions, want the 1 of its turn", got)
	}
	requireMessage(t, "Member", frames[1], serverpackets.SystemMessageS1ObtainedS2, "Leader", lootPotionID)
	requireMessage(t, "Leader", frames[0], serverpackets.SystemMessageYouPickedUpS1, lootPotionID)
}

// TestPartyPickupSkipsMembersOutOfRange: the random and by-turn rules pass
// over a member out of party range of the picker; with nobody else to take
// it, the picker keeps the item.
func TestPartyPickupSkipsMembersOutOfRange(t *testing.T) {
	for name, rule := range map[string]party.LootRule{"random": party.LootRandom, "by turn": party.LootByTurn} {
		t.Run(name, func(t *testing.T) {
			g := lootGroup(t, []seat{{"Leader", 20}, {"Member", 20}}, 2, rule, []int{1})
			frames := g.pickUp(t, 0, 0, lootPotionID, 2)
			if got := g.carried(t, 0, lootPotionID); got != 2 {
				t.Fatalf("Leader carries %d potions, want the 2 it picked up", got)
			}
			if got := g.carried(t, 1, lootPotionID); got != 0 {
				t.Fatalf("the far Member carries %d potions, want none", got)
			}
			requireMessage(t, "Leader", frames[0], serverpackets.SystemMessageYouPickedUpS2S1, lootPotionID, int32(2))
			requireMessage(t, "Member", frames[1], serverpackets.SystemMessageS1ObtainedS3S2, "Leader", lootPotionID, int32(2))
		})
	}
}

// TestPartyPickupSplitsAdena: adena picked up in a party is split evenly
// among the members in range of the picker, the remainder lost; a member
// out of range and a player outside the party get nothing.
func TestPartyPickupSplitsAdena(t *testing.T) {
	g := lootGroup(t, []seat{{"Leader", 20}, {"Member", 20}, {"Far", 20}, {"Outsider", 20}}, 3, party.LootFindersKeepers, []int{2})

	frames := g.pickUp(t, 0, 0, item.AdenaID, 101)
	for i, want := range []int{50, 50, 0, 0} {
		if got := g.carried(t, i, item.AdenaID); got != want {
			t.Fatalf("%s carries %d adena, want %d", g.players[i].name, got, want)
		}
	}
	for i := range 2 {
		requireMessage(t, g.players[i].name, frames[i], serverpackets.SystemMessageEarnedS1Adena, int32(50))
		if got := g.persisted(t, i, item.AdenaID); got != 50 {
			t.Fatalf("%s's stored adena = %d, want 50", g.players[i].name, got)
		}
	}
	requireNoMessage(t, "Far", frames[2], serverpackets.SystemMessageEarnedS1Adena)
	if len(g.srv.GroundItems.Snapshots(nil)) != 0 {
		t.Fatal("the shared adena is still on the ground")
	}
}

// TestPartyPickupLootLockAdmitsTheParty: a stack reserved to a member may
// be picked up by another member of the party, whom the owner hears take
// it; a player outside the party is still refused.
func TestPartyPickupLootLockAdmitsTheParty(t *testing.T) {
	g := lootGroup(t, []seat{{"Leader", 20}, {"Member", 20}, {"Outsider", 20}}, 2, party.LootFindersKeepers, nil)

	frames := g.pickUp(t, 0, g.players[1].id, lootPotionID, 3)
	if got := g.carried(t, 0, lootPotionID); got != 3 {
		t.Fatalf("Leader carries %d potions, want the Member's 3", got)
	}
	requireMessage(t, "Member", frames[1], serverpackets.SystemMessageS1ObtainedS3S2, "Leader", lootPotionID, int32(3))
	requireNoMessage(t, "Outsider", frames[2], serverpackets.SystemMessageS1ObtainedS3S2)

	frames = g.pickUp(t, 2, g.players[1].id, lootPotionID, 3)
	requireMessage(t, "Outsider", frames[2], serverpackets.SystemMessageFailedToPickupS2S1S, lootPotionID, int32(3))
	if len(g.srv.GroundItems.Snapshots(nil)) != 1 {
		t.Fatal("a player outside the party took the Member's stack")
	}
}

// partyLootMonster drops 10 adena and one weapon, always.
func partyLootMonster() *npc.Template {
	return &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 20, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		Drops: []item.DropCategory{
			{Kind: item.DropCurrency, Chance: 100, Drops: []item.Drop{{ItemID: item.AdenaID, Min: 10, Max: 10, Chance: 100}}},
			{Kind: item.DropNormal, Chance: 100, Drops: []item.Drop{{ItemID: 30, Min: 1, Max: 1, Chance: 100}}},
		},
	}
}

// TestPartyAutoLootFollowsTheRule: an auto-looted kill follows the party's
// rule around the monster: the adena is split between the members, the
// weapon goes to the member whose turn it is, and the killer hears it.
func TestPartyAutoLootFollowsTheRule(t *testing.T) {
	g := lootGroup(t, []seat{{"Leader", 20}, {"Member", 20}}, 2, party.LootByTurn, nil, gameservertest.WithAutoLoot(true))
	x, y, z := g.srv.PlayerPosition(t, g.players[0].id)
	monster := g.srv.SpawnHostileNPCTemplateAt(t, partyLootMonster(), location.Location{X: x + 60, Y: y, Z: z})
	g.quiet(t)

	if !monster.TakeDamage(1_000_000, g.combatant(t, 0)) {
		t.Fatal("leader's hit did not kill the monster")
	}
	frames := make([][][]byte, len(g.players))
	for i, p := range g.players {
		frames[i] = drainFrames(t, p.c)
	}
	for i, want := range []int{5, 5} {
		if got := g.carried(t, i, item.AdenaID); got != want {
			t.Fatalf("%s carries %d adena, want %d", g.players[i].name, got, want)
		}
	}
	if got := g.carried(t, 1, 30); got != 1 {
		t.Fatalf("Member carries %d weapons, want the auto-looted one", got)
	}
	if got := g.carried(t, 0, 30); got != 0 {
		t.Fatalf("Leader carries %d weapons, want none", got)
	}
	if len(g.srv.GroundItems.Snapshots(nil)) != 0 {
		t.Fatal("an auto-looted drop fell to the ground")
	}
	requireMessage(t, "Leader", frames[0], serverpackets.SystemMessageS1ObtainedS2, "Member", int32(30))
	requireMessage(t, "Member", frames[1], serverpackets.SystemMessageYouPickedUpS1, int32(30))
	for i := range 2 {
		requireMessage(t, g.players[i].name, frames[i], serverpackets.SystemMessageEarnedS1Adena, int32(5))
	}
}
