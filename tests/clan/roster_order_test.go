package clan

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestRosterListsMembersInTableOrder asks for the roster of a clan whose
// two offline fillers, 900000 and 900001, hash out of id order: in the
// roster's 16-bucket table 900001 lands in bucket 12 and 900000 in bucket
// 13, so the leader's roster request lists Filler1 ahead of Filler0.
func TestRosterListsMembersInTableOrder(t *testing.T) {
	t.Parallel()
	w := bootClanWorld(t, 40, 0, 0, seedClan(t, clanSeed{level: 5, fillers: 2}))

	w.leader.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestPledgeMemberList).Bytes())
	frames := drainFrames(t, w.leader)
	list, ok := firstOpcode(frames, serverpackets.OpcodePledgeShowMemberListAll)
	if !ok {
		t.Fatalf("roster request = %x, want PledgeShowMemberListAll", opcodes(frames))
	}
	_, _, _, members := unitList(t, list)
	if len(members) != 3 || !slices.Contains(members, "Founder") {
		t.Fatalf("roster = %q, want Founder, Filler0 and Filler1", members)
	}
	if i0, i1 := slices.Index(members, "Filler0"), slices.Index(members, "Filler1"); i0 < 0 || i1 < 0 || i1 > i0 {
		t.Fatalf("roster = %q, want Filler1 before Filler0", members)
	}
}
