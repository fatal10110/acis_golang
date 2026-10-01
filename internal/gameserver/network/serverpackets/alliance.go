package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// Alliance packet opcodes.
const (
	OpcodeAskJoinAlly  = 0xa8
	OpcodeAllianceInfo = 0xb4
)

// FrameAskJoinAlly builds the alliance invitation dialog: the inviter's
// object id and the alliance's name.
func FrameAskJoinAlly(requesterID int32, allyName string) wire.Frame {
	w := newFrameWriter(OpcodeAskJoinAlly)
	w.WriteInt32(requesterID)
	w.WriteString(allyName)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// AllianceClan is one clan of an AllianceInfo window.
type AllianceClan struct {
	Name       string
	Level      int32
	LeaderName string
	Total      int32
	Online     int32
}

// Alliance is an alliance as AllianceInfo carries it: its name, member
// totals, leading clan and leader, then each clan.
type Alliance struct {
	Name       string
	Total      int32
	Online     int32
	LeaderClan string
	LeaderName string
	Clans      []AllianceClan
}

// FrameAllianceInfo builds the alliance information window.
func FrameAllianceInfo(a Alliance) wire.Frame {
	w := newFrameWriter(OpcodeAllianceInfo)
	w.WriteString(a.Name)
	w.WriteInt32(a.Total)
	w.WriteInt32(a.Online)
	w.WriteString(a.LeaderClan)
	w.WriteString(a.LeaderName)
	w.WriteInt32(int32(len(a.Clans)))
	for _, c := range a.Clans {
		w.WriteString(c.Name)
		w.WriteInt32(0)
		w.WriteInt32(c.Level)
		w.WriteString(c.LeaderName)
		w.WriteInt32(c.Total)
		w.WriteInt32(c.Online)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
