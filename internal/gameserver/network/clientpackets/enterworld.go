package clientpackets

// OpcodeEnterWorld is the wire opcode for EnterWorld, valid once a
// character has been selected and its world data is loading. It carries no
// payload beyond the opcode: it simply signals that the client has finished
// loading and is ready for the world-entry packet burst.
const OpcodeEnterWorld = 0x03
