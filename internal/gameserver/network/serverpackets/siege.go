package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// Siege packet opcodes.
const (
	OpcodeSiegeInfo         = 0xc9
	OpcodeSiegeAttackerList = 0xca
	OpcodeSiegeDefenderList = 0xcb
)

// SiegeClan is one clan row of a siege list.
type SiegeClan struct {
	ClanID      int32
	Name        string
	LeaderName  string
	CrestID     int32
	AllyID      int32
	AllyName    string
	AllyCrestID int32
}

// The defender list's side codes.
const (
	SiegeDefenderOwner    int32 = 1
	SiegeDefenderPending  int32 = 2
	SiegeDefenderApproved int32 = 3
)

// SiegeDefender is one row of the defender list: its clan and side code.
type SiegeDefender struct {
	SiegeClan
	Side int32
}

// SiegeInfoView is the siege window of a castle: its owner (Owner false
// for a castle no clan holds), whether the viewer leads the owning clan,
// the time now and the siege date, both in Unix seconds.
type SiegeInfoView struct {
	CastleID int32
	IsLord   bool
	OwnerID  int32
	Owner    bool
	// OwnerName, LeaderName, AllyID and AllyName are the owning clan's.
	OwnerName  string
	LeaderName string
	AllyID     int32
	AllyName   string
	Now        int32
	SiegeDate  int32
}

// FrameSiegeInfo builds the siege window; a castle no clan holds shows the
// NPCs as its owner.
func FrameSiegeInfo(v SiegeInfoView) wire.Frame {
	w := newFrameWriter(OpcodeSiegeInfo)
	w.WriteInt32(v.CastleID)
	w.WriteInt32(boolInt32(v.IsLord))
	w.WriteInt32(v.OwnerID)
	if v.Owner {
		w.WriteString(v.OwnerName)
		w.WriteString(v.LeaderName)
		w.WriteInt32(v.AllyID)
		w.WriteString(v.AllyName)
	} else {
		w.WriteString("NPC")
		w.WriteString("")
		w.WriteInt32(0)
		w.WriteString("")
	}
	w.WriteInt32(v.Now)
	w.WriteInt32(v.SiegeDate)
	w.WriteInt32(0)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameSiegeAttackerList builds the attacking clans of residence id.
func FrameSiegeAttackerList(id int32, attackers []SiegeClan) wire.Frame {
	w := newFrameWriter(OpcodeSiegeAttackerList)
	writeSiegeListHeader(w, id, len(attackers))
	for _, c := range attackers {
		writeSiegeClan(w, c, nil)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameSiegeDefenderList builds the defending clans of castle id, each
// with its side code: the owner and the approved defenders, then the clans
// waiting for approval.
func FrameSiegeDefenderList(id int32, defenders []SiegeDefender) wire.Frame {
	w := newFrameWriter(OpcodeSiegeDefenderList)
	writeSiegeListHeader(w, id, len(defenders))
	for _, d := range defenders {
		side := d.Side
		writeSiegeClan(w, d.SiegeClan, &side)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func writeSiegeListHeader(w *wire.Writer, id int32, n int) {
	w.WriteInt32(id)
	w.WriteInt32(0)
	w.WriteInt32(1)
	w.WriteInt32(0)
	w.WriteInt32(int32(n))
	w.WriteInt32(int32(n))
}

func writeSiegeClan(w *wire.Writer, c SiegeClan, side *int32) {
	w.WriteInt32(c.ClanID)
	w.WriteString(c.Name)
	w.WriteString(c.LeaderName)
	w.WriteInt32(c.CrestID)
	w.WriteInt32(0)
	if side != nil {
		w.WriteInt32(*side)
	}
	w.WriteInt32(c.AllyID)
	w.WriteString(c.AllyName)
	w.WriteString("")
	w.WriteInt32(c.AllyCrestID)
}

// Siege system message parameter: the castle's name, from its id.
const SystemMessageParamCastleName = 5

// CastleNameParam is a castle-name parameter.
func CastleNameParam(castleID int32) SystemMessageParam {
	return SystemMessageParam{Type: SystemMessageParamCastleName, Value: castleID}
}
