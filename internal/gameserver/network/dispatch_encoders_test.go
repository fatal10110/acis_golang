package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/link"
)

func encodeProtocolVersion(revision int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeProtocolVersion)
	w.WriteInt32(revision)
	return w.Bytes()
}

func encodeAuthLogin(name string, key link.SessionKey) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAuthLogin)
	w.WriteString(name)
	w.WriteInt32(key.PlayKey2)
	w.WriteInt32(key.PlayKey1)
	w.WriteInt32(key.LoginKey1)
	w.WriteInt32(key.LoginKey2)
	return w.Bytes()
}

func encodeRequestCharacterCreate(name string, race, sex, classID int32, hairStyle, hairColor, face byte) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestCharacterCreate)
	w.WriteString(name)
	w.WriteInt32(race)
	w.WriteInt32(sex)
	w.WriteInt32(classID)
	for i := 0; i < 6; i++ {
		w.WriteInt32(0)
	}
	w.WriteInt32(int32(hairStyle))
	w.WriteInt32(int32(hairColor))
	w.WriteInt32(int32(face))
	return w.Bytes()
}

func encodeRequestCharacterDelete(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestCharacterDelete)
	w.WriteInt32(slot)
	return w.Bytes()
}

func encodeCharacterRestore(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeCharacterRestore)
	w.WriteInt32(slot)
	return w.Bytes()
}

func encodeRequestGameStart(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(slot)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeEnterWorld() []byte {
	return wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes()
}

func encodeRequestManorList() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestManorList)
	return w.Bytes()
}

func encodeUnknownExtendedOpcode() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(0xffff) // unmapped second-opcode, must count as unknown
	return w.Bytes()
}

func encodeRequestCursedWeaponList() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestCursedWeaponList)
	return w.Bytes()
}

func encodeRequestCursedWeaponLocation() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestCursedWeaponLocation)
	return w.Bytes()
}

func encodeRequestAutoSoulShot(itemID, typ int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestAutoSoulShot)
	w.WriteInt32(itemID)
	w.WriteInt32(typ)
	return w.Bytes()
}

func encodeUseItem(objectID int32, ctrl bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	w.WriteInt32(objectID)
	w.WriteInt32(wire.BoolInt32(ctrl))
	return w.Bytes()
}

func encodeRequestUnEquipItem(bodySlot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestUnEquipItem)
	w.WriteInt32(bodySlot)
	return w.Bytes()
}

func encodeRequestEnchantItem(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestEnchantItem)
	w.WriteInt32(objectID)
	return w.Bytes()
}

func encodeRequestAcquireSkillInfo(skillID, level, skillType int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAcquireSkillInfo)
	w.WriteInt32(skillID)
	w.WriteInt32(level)
	w.WriteInt32(skillType)
	return w.Bytes()
}

func encodeRequestAcquireSkill(skillID, level, skillType int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAcquireSkill)
	w.WriteInt32(skillID)
	w.WriteInt32(level)
	w.WriteInt32(skillType)
	return w.Bytes()
}

func encodeRequestPackageSendableItemList(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestPackageItemList)
	w.WriteInt32(objectID)
	return w.Bytes()
}

func encodeRequestMagicSkillUse(skillID int32, ctrl, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	w.WriteInt32(skillID)
	w.WriteInt32(wire.BoolInt32(ctrl))
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func encodeRequestExMagicSkillUseGround(x, y, z, skillID int32, ctrl, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestExMagicSkillUseGround)
	w.WriteInt32(x)
	w.WriteInt32(y)
	w.WriteInt32(z)
	w.WriteInt32(skillID)
	w.WriteInt32(wire.BoolInt32(ctrl))
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func encodeRequestSkillCoolTime() []byte {
	return wire.NewPacketWriter(clientpackets.OpcodeRequestSkillCoolTime).Bytes()
}

func encodeRequestActionUse(actionID int32, ctrl, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestActionUse)
	w.WriteInt32(actionID)
	if ctrl {
		w.WriteInt32(1)
	} else {
		w.WriteInt32(0)
	}
	if shift {
		w.WriteUint8(1)
	} else {
		w.WriteUint8(0)
	}
	return w.Bytes()
}

func encodeMoveBackwardToLocation(target, origin location.Location, moveMovement int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
	w.WriteInt32(int32(target.X))
	w.WriteInt32(int32(target.Y))
	w.WriteInt32(int32(target.Z))
	w.WriteInt32(int32(origin.X))
	w.WriteInt32(int32(origin.Y))
	w.WriteInt32(int32(origin.Z))
	w.WriteInt32(moveMovement)
	return w.Bytes()
}

func encodeValidatePosition(at location.Location, heading int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeValidatePosition)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteInt32(heading)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeCannotMoveAnymore(at location.Location, heading int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeCannotMoveAnymore)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteInt32(heading)
	return w.Bytes()
}

func encodeStartRotating(degree, side int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeStartRotating)
	w.WriteInt32(degree)
	w.WriteInt32(side)
	return w.Bytes()
}

func encodeFinishRotating(degree, side int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeFinishRotating)
	w.WriteInt32(degree)
	w.WriteInt32(side)
	return w.Bytes()
}

func encodeAction(objectID int32, origin location.Location, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAction)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(origin.X))
	w.WriteInt32(int32(origin.Y))
	w.WriteInt32(int32(origin.Z))
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func encodeAttackRequest(objectID int32, origin location.Location, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAttackRequest)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(origin.X))
	w.WriteInt32(int32(origin.Y))
	w.WriteInt32(int32(origin.Z))
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func encodeRequestTargetCancel(unselect uint16) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestTargetCancel)
	w.WriteUint16(unselect)
	return w.Bytes()
}

func encodeRequestChangeMoveType(run bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestChangeMoveType)
	w.WriteInt32(wire.BoolInt32(run))
	return w.Bytes()
}

func encodeRequestChangeWaitType(stand bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestChangeWaitType)
	w.WriteInt32(wire.BoolInt32(stand))
	return w.Bytes()
}

func encodeRequestSocialAction(actionID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestSocialAction)
	w.WriteInt32(actionID)
	return w.Bytes()
}

func encodeRequestDropItem(objectID, count int32, at location.Location) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestDropItem)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	return w.Bytes()
}

func encodeRequestDestroyItem(objectID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestDestroyItem)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	return w.Bytes()
}

func encodeRequestCrystallizeItem(objectID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestCrystallizeItem)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	return w.Bytes()
}

func encodeRequestShortCutReg(typ, slot, id, characterType int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestShortCutReg)
	w.WriteInt32(typ)
	w.WriteInt32(slot)
	w.WriteInt32(id)
	w.WriteInt32(characterType)
	return w.Bytes()
}

func encodeRequestShortCutDel(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestShortCutDel)
	w.WriteInt32(slot)
	return w.Bytes()
}

func encodeSendTimeCheck(requestID, responseID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSendTimeCheck)
	w.WriteInt32(requestID)
	w.WriteInt32(responseID)
	return w.Bytes()
}

func encodeSingleOpcode(opcode byte) []byte {
	return wire.NewPacketWriter(opcode).Bytes()
}

func encodeDlgAnswer(messageID, answer, requesterID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeDlgAnswer)
	w.WriteInt32(messageID)
	w.WriteInt32(answer)
	w.WriteInt32(requesterID)
	return w.Bytes()
}
