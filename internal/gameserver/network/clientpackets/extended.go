package clientpackets

// OpcodeExtended is the first byte for game client packets with a
// little-endian uint16 sub-opcode.
const OpcodeExtended = 0xd0

// Extended client packet opcodes.
const (
	OpcodeRequestManorList uint16 = 0x0008
)
