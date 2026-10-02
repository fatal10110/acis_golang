package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// Duel server packet sub-opcodes.
const (
	OpcodeExDuelAskStart       uint16 = 0x004b
	OpcodeExDuelReady          uint16 = 0x004c
	OpcodeExDuelStart          uint16 = 0x004d
	OpcodeExDuelEnd            uint16 = 0x004e
	OpcodeExDuelUpdateUserInfo uint16 = 0x004f
)

// FrameExDuelAskStart asks the receiver whether it takes requester's
// challenge to a duel, a party duel when party is set.
func FrameExDuelAskStart(requester string, party bool) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExDuelAskStart)
	w.WriteString(requester)
	w.WriteInt32(boolInt32(party))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameExDuelReady readies the receiver's duel window.
func FrameExDuelReady(party bool) wire.Frame { return frameDuelFlag(OpcodeExDuelReady, party) }

// FrameExDuelStart starts the receiver's duel.
func FrameExDuelStart(party bool) wire.Frame { return frameDuelFlag(OpcodeExDuelStart, party) }

// FrameExDuelEnd ends the receiver's duel.
func FrameExDuelEnd(party bool) wire.Frame { return frameDuelFlag(OpcodeExDuelEnd, party) }

func frameDuelFlag(opcode uint16, party bool) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(opcode)
	w.WriteInt32(boolInt32(party))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// DuelUserInfo is a duellist as its opponents' duel window shows it.
type DuelUserInfo struct {
	Name                 string
	ObjectID             int32
	ClassID, Level       int32
	HP, MaxHP, MP, MaxMP int32
	CP, MaxCP            int32
}

// FrameExDuelUpdateUserInfo refreshes one opponent in the receiver's duel
// window.
func FrameExDuelUpdateUserInfo(u DuelUserInfo) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExDuelUpdateUserInfo)
	w.WriteString(u.Name)
	w.WriteInt32(u.ObjectID)
	w.WriteInt32(u.ClassID)
	w.WriteInt32(u.Level)
	w.WriteInt32(u.HP)
	w.WriteInt32(u.MaxHP)
	w.WriteInt32(u.MP)
	w.WriteInt32(u.MaxMP)
	w.WriteInt32(u.CP)
	w.WriteInt32(u.MaxCP)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
