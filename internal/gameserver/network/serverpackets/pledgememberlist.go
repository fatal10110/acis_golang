package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

const (
	// OpcodePledgeShowMemberListAll is the wire opcode for a pledge member list.
	OpcodePledgeShowMemberListAll = 0x53
	// OpcodePledgeShowMemberListUpdate is the wire opcode for a pledge member row update.
	OpcodePledgeShowMemberListUpdate = 0x54
)

// PledgeMemberList is the clan header and member rows sent to the pledge UI.
type PledgeMemberList struct {
	ClanID      int32
	PledgeType  int32
	PledgeName  string
	LeaderName  string
	CrestID     int32
	Level       int32
	CastleID    int32
	ClanHallID  int32
	Rank        int32
	Reputation  int32
	Dissolving  bool
	AllyID      int32
	AllyName    string
	AllyCrestID int32
	AtWar       bool
	Members     []PledgeMemberListMember
}

// PledgeMemberListMember is one member row in the pledge UI.
type PledgeMemberListMember struct {
	Name           string
	Level          int32
	ClassID        int32
	Sex            int32
	Race           int32
	OnlineObjectID int32
	PledgeType     int32
	HasSponsor     bool
}

// FramePledgeShowMemberListAll builds a pledge member list packet.
func FramePledgeShowMemberListAll(list PledgeMemberList) wire.Frame {
	w := newFrameWriter(OpcodePledgeShowMemberListAll)
	w.WriteInt32(boolInt32(list.PledgeType != 0))
	w.WriteInt32(list.ClanID)
	w.WriteInt32(list.PledgeType)
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

	w.WriteInt32(int32(pledgeMemberCount(list.Members, list.PledgeType)))
	for _, member := range list.Members {
		if member.PledgeType != list.PledgeType {
			continue
		}
		writePledgeMemberListMember(w, member)
		w.WriteInt32(boolInt32(member.HasSponsor))
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FramePledgeShowMemberListUpdate builds a pledge member row update packet.
func FramePledgeShowMemberListUpdate(member PledgeMemberListMember) wire.Frame {
	w := newFrameWriter(OpcodePledgeShowMemberListUpdate)
	writePledgeMemberListMember(w, member)
	w.WriteInt32(member.PledgeType)
	w.WriteInt32(boolInt32(member.HasSponsor))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func pledgeMemberCount(members []PledgeMemberListMember, pledgeType int32) int {
	var count int
	for _, member := range members {
		if member.PledgeType == pledgeType {
			count++
		}
	}
	return count
}

func writePledgeMemberListMember(w *wire.Writer, member PledgeMemberListMember) {
	w.WriteString(member.Name)
	w.WriteInt32(member.Level)
	w.WriteInt32(member.ClassID)
	w.WriteInt32(member.Sex)
	w.WriteInt32(member.Race)
	w.WriteInt32(member.OnlineObjectID)
}
