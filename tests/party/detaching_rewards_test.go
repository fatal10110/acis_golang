package party

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// A member leaving the world is out of its party's kills from the moment
// its departure starts: the reference takes a disconnecting player out of
// its party (Player.cleanup, Party.removePartyMember) before it stores the
// inventory and the character row, so nothing a party kill pays after that
// can miss those saves. These tests hold a member at that point, marked as
// leaving while still in its party and in sight.

// TestLeavingMemberSharesNoPartyKill: Member starts leaving the world, then
// the leader kills the reward monster. Member is out of the share as if out
// of the party: it gets nothing, and the leader, now the only member within
// the level cutoff, takes the whole 5000 exp and 250 SP at the one-member
// bonus. Novice still sees its zero gain.
func TestLeavingMemberSharesNoPartyKill(t *testing.T) {
	t.Parallel()
	g, monster := rewardParty(t)
	member := g.character(t, 1)
	exp, sp := member.Exp, member.SP
	member.MarkDetaching()

	if !monster.TakeDamage(1000, g.combatant(t, 0)) {
		t.Fatal("leader's hit did not kill the monster")
	}
	want := []gain{{5000, 250, true}, {}, {0, 0, true}, {}}
	if got := g.earned(t); !equalGains(got, want) {
		t.Fatalf("gains = %+v, want %+v", got, want)
	}
	if member.Exp != exp || member.SP != sp {
		t.Fatalf("leaving Member holds %d exp %d SP, want %d and %d", member.Exp, member.SP, exp, sp)
	}
}

// TestLeavingMemberTakesNoPartyLoot: a member leaving the world takes no
// auto-looted adena share, which the members staying split, and is skipped
// by the by-turn rule, whose item goes to the next member instead.
func TestLeavingMemberTakesNoPartyLoot(t *testing.T) {
	t.Parallel()
	t.Run("adena share", func(t *testing.T) {
		g := lootGroup(t, []seat{{"Leader", 20}, {"Member", 20}, {"Third", 20}}, 3, party.LootByTurn, nil, gameservertest.WithAutoLoot(true))
		g.character(t, 1).MarkDetaching()
		killPartyDrop(t, g, partyDropMonster(item.AdenaID, 30))
		for i, want := range []int{15, 0, 15} {
			if got := g.carried(t, i, item.AdenaID); got != want {
				t.Errorf("%s carries %d adena, want %d", g.players[i].name, got, want)
			}
			if got := g.persisted(t, i, item.AdenaID); got != want {
				t.Errorf("%s's persisted adena = %d, want %d", g.players[i].name, got, want)
			}
		}
	})
	t.Run("by turn", func(t *testing.T) {
		g := lootGroup(t, []seat{{"Leader", 20}, {"Member", 20}}, 2, party.LootByTurn, nil, gameservertest.WithAutoLoot(true))
		g.character(t, 1).MarkDetaching()
		killPartyDrop(t, g, partyDropMonster(30, 3))
		if got := g.carried(t, 1, 30); got != 0 {
			t.Errorf("leaving Member carries %d weapons, want none", got)
		}
		if got := g.carried(t, 0, 30); got != 3 {
			t.Errorf("Leader carries %d weapons, want 3", got)
		}
	})
}

// TestPartyKillsDuringLogoutAreSavedOrNotGiven runs party kills on the
// leader's queue while Member logs out, on the production pool: whatever
// Member's character holds once it has left, exp, SP and adena alike, is
// what its saves wrote. Nothing paid to it after its saves read it.
func TestPartyKillsDuringLogoutAreSavedOrNotGiven(t *testing.T) {
	t.Parallel()
	g := lootGroup(t, []seat{{"Leader", 20}, {"Member", 20}}, 2, party.LootByTurn, nil,
		gameservertest.WithAutoLoot(true), gameservertest.WithRealPool())
	member := g.character(t, 1)
	leader := g.combatant(t, 0)
	tmpl := partyDropMonster(item.AdenaID, 30)
	tmpl.RewardExp, tmpl.RewardSp = 1000, 50
	x, y, z := g.srv.PlayerPosition(t, g.players[0].id)
	const kills = 40
	monsters := make([]*npc.Hostile, kills)
	for i := range monsters {
		monsters[i] = g.srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: x + 60, Y: y + i, Z: z})
	}
	g.quiet(t)

	queue := g.srv.PlayerQueue(t, g.players[0].id)
	for _, m := range monsters {
		if !queue.Post(func() { m.TakeDamage(1_000_000, leader) }) {
			t.Fatal("leader's queue closed")
		}
	}
	g.players[1].c.Send(encodeSingle(clientpackets.OpcodeLogout))
	g.srv.AdvanceUntil(t, "Member leaves the world", func() bool {
		_, ok := g.srv.State.Player(g.players[1].id)
		return !ok
	})
	done := make(chan struct{})
	if !queue.Post(func() { close(done) }) {
		t.Fatal("leader's queue closed")
	}
	<-done
	g.srv.FlushItems(t)
	g.srv.FlushPersistence(t)

	var exp int64
	var sp int
	if err := g.srv.DB.QueryRowContext(context.Background(), "SELECT exp, sp FROM characters WHERE obj_Id = ?", g.players[1].id).Scan(&exp, &sp); err != nil {
		t.Fatal(err)
	}
	if exp != member.Exp || sp != member.SP {
		t.Errorf("Member saved %d exp %d SP, holds %d and %d", exp, sp, member.Exp, member.SP)
	}
	if got, want := g.persisted(t, 1, item.AdenaID), member.Inventory().Adena(); got != want {
		t.Errorf("Member saved %d adena, holds %d", got, want)
	}
}
