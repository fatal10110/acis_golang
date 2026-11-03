package clan

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestAllianceInfoListsClansInRegistryOrder pins the order the alliance
// window and its system messages list the member clans in: the order of
// the hash table the clan registry is kept in, by bucket of the clan id
// with its high half folded in. Ash (0x01020304, bucket 6) and Birch
// (0x01020607, bucket 5) list after Knights (bucket 1) and Rivals
// (bucket 2) though their ids are lower, Birch before Ash.
func TestAllianceInfoListsClansInRegistryOrder(t *testing.T) {
	ally := itoa(knightsClanID)
	w := bootAllianceWorldSeeded(t, []string{
		`UPDATE clan_data SET ally_id = ` + ally + `, ally_name = 'Ally' WHERE clan_id IN (` + ally + `, ` + itoa(rivalsClanID) + `)`,
		`INSERT INTO clan_data (clan_id, clan_name, clan_level, ally_id, ally_name) VALUES
			(` + itoa(int32(0x01020304)) + `, 'Ash', 3, ` + ally + `, 'Ally'),
			(` + itoa(int32(0x01020607)) + `, 'Birch', 3, ` + ally + `, 'Ally')`,
	})
	want := []string{"Knights", "Rivals", "Birch", "Ash"}

	w.member.Send(encodeAllyBare(clientpackets.OpcodeRequestAllyInfo))
	info := drainFrames(t, w.member)
	if len(info) == 0 || info[0][0] != serverpackets.OpcodeAllianceInfo {
		t.Fatalf("alliance info = %x, want AllianceInfo first", opcodes(info))
	}
	r := wire.NewReader(info[0][1:])
	r.ReadString()
	r.ReadInt32()
	r.ReadInt32()
	r.ReadString()
	r.ReadString()
	var listed []string
	for range int(r.ReadInt32()) {
		listed = append(listed, r.ReadString())
		r.ReadInt32()
		r.ReadInt32()
		r.ReadString()
		r.ReadInt32()
		r.ReadInt32()
	}
	if !slices.Equal(listed, want) {
		t.Fatalf("AllianceInfo clans = %v, want %v", listed, want)
	}

	var named []string
	for _, f := range info[1:] {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		if id, params := sysMsg(t, f); id == allyMsgClanName {
			named = append(named, params...)
		}
	}
	if !slices.Equal(named, want) {
		t.Fatalf("alliance info clan lines = %v, want %v", named, want)
	}
}
