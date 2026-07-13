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
		0xd0: true, // extended packets used during loading
	},
	StateInGame: {
		0x01: true, // move backward to location
		0x04: true, // action
		0x09: true, // logout (also valid pre-game; see StateAuthed)
		0x0a: true, // attack request
		0x0f: true, // request item list
		0x11: true, // request unequip item
		0x12: true, // request drop item
		0x14: true, // use item
		0x15: true, // trade request
		0x16: true, // add trade item
		0x17: true, // trade done
		0x1a: true, // dummy packet
		0x1b: true, // social action
		0x1c: true, // change move type
		0x1d: true, // change wait type
		0x1e: true, // sell item
		0x1f: true, // buy item
		0x23: true, // dummy packet
		0x2e: true, // dummy packet
		0x2f: true, // request magic skill use
		0x30: true, // appearing
		0x31: true, // warehouse deposit list
		0x32: true, // warehouse withdraw list
		0x33: true, // register shortcut
		0x34: true, // dummy packet
		0x35: true, // delete shortcut
		0x36: true, // cannot move anymore
		0x37: true, // cancel target
		0x3e: true, // dummy packet
		0x3f: true, // request skill list
		0x42: true, // get on vehicle
		0x43: true, // get off vehicle
		0x44: true, // answer trade request
		0x45: true, // action use
		0x46: true, // restart
		0x48: true, // validate position
		0x4a: true, // start rotating
		0x4b: true, // finish rotating
		0x58: true, // enchant item
		0x59: true, // destroy item
		0x5c: true, // move in vehicle
		0x5d: true, // cannot move in vehicle
		0x63: true, // request quest list
		0x64: true, // abort quest
		0x6b: true, // acquire skill info
		0x6c: true, // acquire skill
		0x6d: true, // restart point
		0x72: true, // crystallize item
		0x89: true, // change pet name
		0x8a: true, // pet use item
		0x8b: true, // give item to pet
		0x8c: true, // get item from pet
		0x8f: true, // pet get item
		0x97: true, // time check
		0x9d: true, // request skill reuse timers
		0x9e: true, // package sendable item list
		0x9f: true, // package send
		0xc5: true, // dialog answer
		0xca: true, // game guard reply
		0xcd: true, // show mini map
		0xd0: true, // extended packets
	},
}

// Allowed reports whether opcode is a first-byte opcode a client in state s
// is permitted to send. A packet dispatcher should reject/drop any opcode
// for which this returns false rather than decoding it.
func Allowed(s State, opcode byte) bool {
	return allowedOpcodes[s][opcode]
}
