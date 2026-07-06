package clientpackets

// OpcodeRequestNewCharacter is the wire opcode for RequestNewCharacter,
// valid once a client is authenticated. It carries no payload beyond the
// opcode: it simply asks for the list of professions the creation screen
// should offer.
const OpcodeRequestNewCharacter = 0x0e
