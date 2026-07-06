package network

import "fmt"

// State is a game client's position in the connect-to-in-world lifecycle:
// StateConnected -> StateAuthed -> StateEntering -> StateInGame. It gates
// which inbound opcodes a client may send at any given moment; see Allowed.
type State int

const (
	// StateConnected is the state right after the TCP handshake, before any
	// credentials have been exchanged.
	StateConnected State = iota
	// StateAuthed is reached once a login session has been validated; from
	// here the client can list, create, delete, and restore characters.
	StateAuthed
	// StateEntering is reached once a character slot has been chosen and its
	// world data is loading; it ends when the client completes entry into
	// the world.
	StateEntering
	// StateInGame is reached once the character has fully entered the
	// world.
	StateInGame
)

// String returns a lower-case, hyphenated name for s, or "state(N)" for a
// value outside the defined constants.
func (s State) String() string {
	switch s {
	case StateConnected:
		return "connected"
	case StateAuthed:
		return "authed"
	case StateEntering:
		return "entering"
	case StateInGame:
		return "in-game"
	default:
		return fmt.Sprintf("state(%d)", int(s))
	}
}

// allowedOpcodes lists, for each state, the first-byte opcodes a client may
// send. It starts out covering only the connect-to-in-world handshake
// sequence; every additional packet registers its opcode here as it gets
// ported, the same way the full dispatch table grows one packet at a time.
// Extended (second-opcode) packet families are not modeled yet.
var allowedOpcodes = map[State]map[byte]bool{
	StateConnected: {
		0x00: true, // protocol version negotiation
		0x08: true, // login credentials + session keys
	},
	StateAuthed: {
		0x09: true, // logout
		0x0b: true, // create character
		0x0c: true, // delete character
		0x0d: true, // select character / start game
		0x0e: true, // request character-creation templates
		0x62: true, // restore (undelete) character
		0x68: true, // request pledge crest
	},
	StateEntering: {
		0x03: true, // enter world
		0x3f: true, // request quest list
	},
	StateInGame: {
		0x09: true, // logout (also valid pre-game; see StateAuthed)
	},
}

// Allowed reports whether opcode is a first-byte opcode a client in state s
// is permitted to send. A packet dispatcher should reject/drop any opcode
// for which this returns false rather than decoding it.
func Allowed(s State, opcode byte) bool {
	return allowedOpcodes[s][opcode]
}
