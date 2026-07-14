package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeEtcStatusUpdate is the wire opcode for EtcStatusUpdate.
const OpcodeEtcStatusUpdate = 0xf3

// EtcStatus is the compact set of miscellaneous status flags shown in the
// client status window.
type EtcStatus struct {
	Charges           int32
	WeightPenalty     int32
	Blocked           bool
	DangerArea        bool
	GradePenalty      bool
	CharmOfCourage    bool
	DeathPenaltyLevel int32
}

// FrameEtcStatusUpdate builds the miscellaneous status update packet.
func FrameEtcStatusUpdate(s EtcStatus) wire.Frame {
	w := newFrameWriter(OpcodeEtcStatusUpdate)
	w.WriteInt32(s.Charges)
	w.WriteInt32(s.WeightPenalty)
	w.WriteInt32(boolInt32(s.Blocked))
	w.WriteInt32(boolInt32(s.DangerArea))
	w.WriteInt32(boolInt32(s.GradePenalty))
	w.WriteInt32(boolInt32(s.CharmOfCourage))
	w.WriteInt32(s.DeathPenaltyLevel)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
