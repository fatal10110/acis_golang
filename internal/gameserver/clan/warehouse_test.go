package clan

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// TestWarehouseRightsEndWithMembership asks whether members may use the
// clan warehouse once they are out of the clan: a member asking for another
// clan's warehouse, and an expelled member whose character still carries
// the clan id because its own queue has not cleared it yet. Both are
// refused silently, whichever withdrawal rule applies. A refusal naming the
// leader-only rule would answer a player who is no longer in the clan.
func TestWarehouseRightsEndWithMembership(t *testing.T) {
	for _, members := range []bool{true, false} {
		s, _, _, writes := orderedClan(t, 0, 1)
		s.cfg.MembersCanWithdrawFromWarehouse = members
		leader := inClan(orderLeaderID, "Leader", orderClanID)
		member := inClan(orderLeaderID+1, "Member0", orderClanID)
		if _, ok := s.SetRankPrivileges(leader, MemberPowerGrade, int32(PrivWarehouseSearch)); !ok {
			t.Fatal("set the members' warehouse privilege")
		}
		want := WithdrawLeaderOnly
		if members {
			want = WithdrawAllowed
		}
		if got := s.CanWithdraw(member, orderClanID); got != want {
			t.Fatalf("members=%v: member's withdrawal = %v, want %v", members, got, want)
		}
		if got := s.CanWithdraw(leader, orderClanID); got != WithdrawAllowed {
			t.Fatalf("members=%v: leader's withdrawal = %v, want allowed", members, got)
		}
		for _, c := range []*player.Character{leader, member} {
			if got := s.CanWithdraw(c, orderClanID+100); got != WithdrawSilent {
				t.Fatalf("members=%v: %s withdrawing from another clan = %v, want silent", members, c.Name, got)
			}
			if s.InClan(c, orderClanID+100) {
				t.Fatalf("members=%v: %s is in another clan", members, c.Name)
			}
		}

		if _, _, res := s.Oust(leader, "Member0", func(int32) *player.Character { return member }, time.Now()); res != Ousted {
			t.Fatalf("members=%v: oust = %v, want Ousted", members, res)
		}
		writes.drain()
		if member.ClanID() != orderClanID {
			t.Fatal("the expulsion cleared the member's clan id off its queue")
		}
		if s.InClan(member, orderClanID) {
			t.Fatalf("members=%v: expelled member is still in the clan", members)
		}
		if got := s.CanWithdraw(member, orderClanID); got != WithdrawSilent {
			t.Fatalf("members=%v: expelled member's withdrawal = %v, want silent", members, got)
		}
		if !s.InClan(leader, orderClanID) || s.CanWithdraw(leader, orderClanID) != WithdrawAllowed {
			t.Fatalf("members=%v: the leader lost the warehouse with the expulsion", members)
		}
	}
}
