package clientpackets

// OpcodeExtended is the first byte for game client packets with a
// little-endian uint16 sub-opcode.
const OpcodeExtended = 0xd0

// OpcodeRequestSkillCoolTime asks for remaining skill reuse timers. The
// skill reuse model is not active yet, so the game server accepts and
// ignores it.
const OpcodeRequestSkillCoolTime = 0x9d

// Extended client packet opcodes.
const (
	OpcodeRequestAutoSoulShot         uint16 = 0x0005
	OpcodeRequestManorList            uint16 = 0x0008
	OpcodeRequestExPledgeCrestLarge   uint16 = 0x0010
	OpcodeRequestCursedWeaponList     uint16 = 0x0022
	OpcodeRequestCursedWeaponLocation uint16 = 0x0023
	OpcodeRequestConfirmTargetItem    uint16 = 0x0029
	OpcodeRequestConfirmRefinerItem   uint16 = 0x002a
	OpcodeRequestConfirmGemStone      uint16 = 0x002b
	OpcodeRequestConfirmCancelItem    uint16 = 0x002d
)
