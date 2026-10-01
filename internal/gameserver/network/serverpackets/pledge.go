package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// Clan packet opcodes.
const (
	OpcodeManagePledgePower          = 0x30
	OpcodeAskJoinPledge              = 0x32
	OpcodeJoinPledge                 = 0x33
	OpcodePledgeShowMemberListAdd    = 0x55
	OpcodePledgeShowMemberListDelete = 0x56
	OpcodePledgeShowMemberListDelAll = 0x82
	OpcodePledgeInfo                 = 0x83
	OpcodePledgeShowInfoUpdate       = 0x88
	OpcodePledgeStatusChanged        = 0xcd

	OpcodeExPledgePowerGradeList    uint16 = 0x003b
	OpcodeExPledgeReceivePowerInfo  uint16 = 0x003c
	OpcodeExPledgeReceiveMemberInfo uint16 = 0x003d
)

// PledgeHeader is a clan's header as PledgeShowInfoUpdate carries it.
type PledgeHeader struct {
	ClanID      int32
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
}

// FramePledgeShowInfoUpdate builds the clan header refresh.
func FramePledgeShowInfoUpdate(h PledgeHeader) wire.Frame {
	w := newFrameWriter(OpcodePledgeShowInfoUpdate)
	w.WriteInt32(h.ClanID)
	w.WriteInt32(h.CrestID)
	w.WriteInt32(h.Level)
	w.WriteInt32(h.CastleID)
	w.WriteInt32(h.ClanHallID)
	w.WriteInt32(h.Rank)
	w.WriteInt32(h.Reputation)
	if h.Dissolving {
		w.WriteInt32(3)
	} else {
		w.WriteInt32(0)
	}
	w.WriteInt32(0)
	w.WriteInt32(h.AllyID)
	w.WriteString(h.AllyName)
	w.WriteInt32(h.AllyCrestID)
	w.WriteInt32(boolInt32(h.AtWar))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FramePledgeShowMemberListAdd builds the roster row added for a new
// member.
func FramePledgeShowMemberListAdd(m PledgeMemberListMember) wire.Frame {
	w := newFrameWriter(OpcodePledgeShowMemberListAdd)
	writePledgeMemberListMember(w, m)
	w.WriteInt32(m.PledgeType)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FramePledgeShowMemberListDelete builds the removal of the roster row
// named name.
func FramePledgeShowMemberListDelete(name string) wire.Frame {
	w := newFrameWriter(OpcodePledgeShowMemberListDelete)
	w.WriteString(name)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FramePledgeShowMemberListDeleteAll builds the packet that clears and
// disables the clan window.
func FramePledgeShowMemberListDeleteAll() wire.Frame {
	w := newFrameWriter(OpcodePledgeShowMemberListDelAll)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FramePledgeInfo builds a clan's name card: id, name and alliance name.
func FramePledgeInfo(clanID int32, name, allyName string) wire.Frame {
	w := newFrameWriter(OpcodePledgeInfo)
	w.WriteInt32(clanID)
	w.WriteString(name)
	w.WriteString(allyName)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FramePledgeStatusChanged builds a clan's leader, crest and alliance
// status.
func FramePledgeStatusChanged(leaderID, clanID, crestID, allyID, allyCrestID int32) wire.Frame {
	w := newFrameWriter(OpcodePledgeStatusChanged)
	w.WriteInt32(leaderID)
	w.WriteInt32(clanID)
	w.WriteInt32(crestID)
	w.WriteInt32(allyID)
	w.WriteInt32(allyCrestID)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameAskJoinPledge builds the invitation dialog from requesterID into
// the clan named pledgeName.
func FrameAskJoinPledge(requesterID int32, pledgeName string) wire.Frame {
	w := newFrameWriter(OpcodeAskJoinPledge)
	w.WriteInt32(requesterID)
	w.WriteString(pledgeName)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameJoinPledge builds the notice that the player joined clanID.
func FrameJoinPledge(clanID int32) wire.Frame {
	w := newFrameWriter(OpcodeJoinPledge)
	w.WriteInt32(clanID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameManagePledgePower builds a rank's privileges for the privilege
// window.
func FrameManagePledgePower(rank, action, privs int32) wire.Frame {
	w := newFrameWriter(OpcodeManagePledgePower)
	w.WriteInt32(rank)
	w.WriteInt32(action)
	w.WriteInt32(privs)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FramePledgePowerGradeList builds the member count of each rank 1-9;
// counts is indexed by rank, 0 being the leader's.
func FramePledgePowerGradeList(counts [10]int) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExPledgePowerGradeList)
	w.WriteInt32(9)
	for rank := 1; rank < len(counts); rank++ {
		w.WriteInt32(int32(rank))
		w.WriteInt32(int32(counts[rank]))
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FramePledgeReceivePowerInfo builds a member's rank and that rank's
// privileges.
func FramePledgeReceivePowerInfo(powerGrade int32, name string, privs int32) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExPledgeReceivePowerInfo)
	w.WriteInt32(powerGrade)
	w.WriteString(name)
	w.WriteInt32(privs)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// PledgeMemberInfo is one member's detail card.
type PledgeMemberInfo struct {
	PledgeType int32
	Name       string
	Title      string
	PowerGrade int32
	// PledgeName is the clan's name, or the member's sub-unit's.
	PledgeName string
	// Mentor is the member's apprentice's or sponsor's name.
	Mentor string
}

// FramePledgeReceiveMemberInfo builds a member's detail card.
func FramePledgeReceiveMemberInfo(m PledgeMemberInfo) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExPledgeReceiveMemberInfo)
	w.WriteInt32(m.PledgeType)
	w.WriteString(m.Name)
	w.WriteString(m.Title)
	w.WriteInt32(m.PowerGrade)
	w.WriteString(m.PledgeName)
	w.WriteString(m.Mentor)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
