package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeRelationChanged is the wire opcode for RelationChanged.
const OpcodeRelationChanged = 0xce

// Relation bitmask flags carried by RelationChanged, matching
// RelationChanged.java's RELATION_* constants.
const (
	RelationPvPFlag     = 0x00002
	RelationHasKarma    = 0x00004
	RelationLeader      = 0x00080
	RelationInSiege     = 0x00200
	RelationAttacker    = 0x00400
	RelationAlly        = 0x00800
	RelationEnemy       = 0x01000
	RelationMutualWar   = 0x08000
	RelationOneSidedWar = 0x10000
)

// RelationChangedInfo carries a single playable's relation state as seen by
// one recipient, mirroring the reference's RelationChanged(Playable,
// relation, isAutoAttackable) constructor.
type RelationChangedInfo struct {
	ObjectID         int32
	Relation         int32
	IsAutoAttackable bool
	Karma            int32
	PvPFlag          int32
}

// FrameRelationChanged builds the packet the client uses to recolor a
// playable's name and relation icon.
func FrameRelationChanged(info RelationChangedInfo) wire.Frame {
	w := newFrameWriter(OpcodeRelationChanged)
	w.WriteInt32(info.ObjectID)
	w.WriteInt32(info.Relation)
	w.WriteInt32(boolInt32(info.IsAutoAttackable))
	w.WriteInt32(info.Karma)
	w.WriteInt32(info.PvPFlag)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
