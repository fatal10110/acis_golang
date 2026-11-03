package botclient

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamecipher "github.com/fatal10110/acis_golang/internal/gameserver/network/cipher"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// versionCheckSize is VersionCheck's payload length, opcode included.
const versionCheckSize = 18

// chatAll is the Say2 channel everyone around hears.
const chatAll = 0

// decodeVersionCheck returns the full cipher key (the dynamic half the
// packet carries plus the static half) and whether the server enabled crypt.
func decodeVersionCheck(raw []byte) ([]byte, bool, error) {
	if raw[0] != serverpackets.OpcodeVersionCheck || len(raw) != versionCheckSize {
		return nil, false, fmt.Errorf("first game packet is not VersionCheck (%d bytes, opcode %#x)", len(raw), raw[0])
	}
	key := make([]byte, gamecipher.KeySize)
	copy(key[:8], raw[2:10])
	copy(key[8:], gamecipher.StaticKey[:])
	return key, wire.NewReader(raw[10:14]).ReadInt32() != 0, nil
}

// decodeUserInfo reads the leading position and object id of UserInfo.
func decodeUserInfo(raw []byte) Self {
	r := wire.NewReader(raw[1:])
	var s Self
	s.X, s.Y, s.Z = r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	s.Heading = r.ReadInt32()
	s.ObjectID = r.ReadInt32()
	return s
}

// npcSighting is the leading part of NPCInfo.
type npcSighting struct {
	objectID   int32
	attackable bool
	x, y, z    int32
}

func decodeNPCInfo(raw []byte) npcSighting {
	r := wire.NewReader(raw[1:])
	n := npcSighting{objectID: r.ReadInt32()}
	_ = r.ReadInt32() // template id
	n.attackable = r.ReadInt32() != 0
	n.x, n.y, n.z = r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	return n
}

// decodeObjectPosition reads the object id and the first position of a
// packet laid out as objectId, x, y, z (MoveToLocation's destination,
// StopMove, ValidateLocation, TeleportToLocation).
func decodeObjectPosition(raw []byte) (objectID, x, y, z int32) {
	r := wire.NewReader(raw[1:])
	return r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
}

// decodeObjectID reads the leading object id of Die, Revive or DeleteObject.
func decodeObjectID(raw []byte) int32 {
	return wire.NewReader(raw[1:]).ReadInt32()
}

func encodeProtocolVersion(revision int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeProtocolVersion)
	w.WriteInt32(revision)
	return w.Bytes()
}

// encodeAuthLogin writes the keys in the protocol's fixed order: play key 2
// before play key 1, then the login keys.
func encodeAuthLogin(account string, k SessionKeys) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAuthLogin)
	w.WriteString(account)
	w.WriteInt32(k.PlayKey2)
	w.WriteInt32(k.PlayKey1)
	w.WriteInt32(k.LoginKey1)
	w.WriteInt32(k.LoginKey2)
	return w.Bytes()
}

// encodeRequestCharacterCreate asks for a human fighter (race, sex and class
// 0) named name; the six stat fields are ignored by the server.
func encodeRequestCharacterCreate(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestCharacterCreate)
	w.WriteString(name)
	for range 3 + 6 {
		w.WriteInt32(0)
	}
	w.WriteInt32(1) // hair style
	w.WriteInt32(0) // hair color
	w.WriteInt32(0) // face
	return w.Bytes()
}

func encodeRequestGameStart(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(slot)
	w.WriteUint16(0)
	for range 3 {
		w.WriteInt32(0)
	}
	return w.Bytes()
}

func encodeEnterWorld() []byte {
	return wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes()
}

func encodeLogout() []byte {
	return wire.NewPacketWriter(clientpackets.OpcodeLogout).Bytes()
}

// encodeMove is a mouse-click MoveBackwardToLocation from origin to target.
func encodeMove(tx, ty, tz, ox, oy, oz int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
	for _, v := range [...]int32{tx, ty, tz, ox, oy, oz, 1} {
		w.WriteInt32(v)
	}
	return w.Bytes()
}

func encodeValidatePosition(x, y, z, heading int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeValidatePosition)
	for _, v := range [...]int32{x, y, z, heading, 0} {
		w.WriteInt32(v)
	}
	return w.Bytes()
}

func encodeSay(text string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSay2)
	w.WriteString(text)
	w.WriteInt32(chatAll)
	return w.Bytes()
}

// encodeTargetAction is Action or AttackRequest (same layout) on objectID
// from the bot's position, without shift.
func encodeTargetAction(opcode byte, objectID, x, y, z int32) []byte {
	w := wire.NewPacketWriter(opcode)
	for _, v := range [...]int32{objectID, x, y, z} {
		w.WriteInt32(v)
	}
	w.WriteUint8(0)
	return w.Bytes()
}

// encodeRequestRestartPoint asks a dead character to revive in the nearest
// village (restart type 0).
func encodeRequestRestartPoint() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestRestartPoint)
	w.WriteInt32(0)
	return w.Bytes()
}
