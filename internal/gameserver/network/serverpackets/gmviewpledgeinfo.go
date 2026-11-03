package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeGMViewPledgeInfo is the wire opcode for a game master's view of a
// player's clan.
const OpcodeGMViewPledgeInfo = 0x90

// FrameGMViewPledgeInfo builds a game master's view of player's clan: the
// header of list, then every member of list whatever its sub-unit; a
// member's last field reports a sponsor. list.PledgeType is not used.
func FrameGMViewPledgeInfo(player string, list PledgeMemberList) wire.Frame {
	w := newFrameWriter(OpcodeGMViewPledgeInfo)
	w.WriteString(player)
	w.WriteInt32(list.ClanID)
	w.WriteInt32(0)
	w.WriteString(list.PledgeName)
	w.WriteString(list.LeaderName)
	w.WriteInt32(list.CrestID)
	w.WriteInt32(list.Level)
	w.WriteInt32(list.CastleID)
	w.WriteInt32(list.ClanHallID)
	w.WriteInt32(list.Rank)
	w.WriteInt32(list.Reputation)
	if list.Dissolving {
		w.WriteInt32(3)
	} else {
		w.WriteInt32(0)
	}
	w.WriteInt32(0)
	w.WriteInt32(list.AllyID)
	w.WriteString(list.AllyName)
	w.WriteInt32(list.AllyCrestID)
	w.WriteInt32(boolInt32(list.AtWar))

	w.WriteInt32(int32(len(list.Members)))
	for _, member := range list.Members {
		writePledgeMemberListMember(w, member)
		w.WriteInt32(boolInt32(member.HasSponsor))
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
