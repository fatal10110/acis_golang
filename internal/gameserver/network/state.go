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
		0x20: true, // request linked html
		0x21: true, // request bypass command
		0x23: true, // dummy packet
		0x24: true, // invite into a clan
		0x25: true, // answer a clan invitation
		0x26: true, // withdraw from the clan
		0x27: true, // expel a clan member
		0x29: true, // invite to party
		0x2a: true, // answer party invitation
		0x2b: true, // leave party
		0x2c: true, // expel party member
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
		0x38: true, // say2 chat (opcode mapped; not yet wired, see wiresafe.go)
		0x3c: true, // clan member list
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
		0x5b: true, // admin command typed in chat
		0x5c: true, // move in vehicle
		0x5d: true, // cannot move in vehicle
		0x5e: true, // friend invite
		0x5f: true, // answer friend invite
		0x60: true, // friend list
		0x61: true, // friend delete
		0x63: true, // request quest list
		0x64: true, // abort quest
		0x66: true, // clan name card
		0x68: true, // request pledge crest
		0x6b: true, // acquire skill info
		0x6c: true, // acquire skill
		0x6d: true, // restart point
		0x72: true, // crystallize item
		0x73: true, // private store sell: manage
		0x74: true, // private store sell: set list
		0x76: true, // private store sell: quit
		0x77: true, // private store sell: set title
		0x79: true, // buy from a private store
		0x81: true, // online game-master list
		0x88: true, // request ally crest
		0x89: true, // change pet name
		0x8a: true, // pet use item
		0x8b: true, // give item to pet
		0x8c: true, // get item from pet
		0x8f: true, // pet get item
		0x90: true, // private store buy: manage
		0x91: true, // private store buy: set list
		0x93: true, // private store buy: quit
		0x94: true, // private store buy: set title
		0x96: true, // sell to a private store
		0x97: true, // time check
		0x9d: true, // request skill reuse timers
		0x9e: true, // package sendable item list
		0x9f: true, // package send
		0xa0: true, // block list commands
		0xa7: true, // multisell exchange
		0xac: true, // open recipe book
		0xad: true, // delete a recipe
		0xae: true, // recipe craft window
		0xaf: true, // craft a recipe
		0xb1: true, // workshop: set name
		0xb2: true, // workshop: set list
		0xb3: true, // workshop: quit
		0xb5: true, // workshop: craft window
		0xb6: true, // workshop: order a craft
		0xb7: true, // workshop: back to its list
		0xb9: true, // recommend a player
		0xba: true, // symbol draw window
		0xbb: true, // symbol draw details
		0xbc: true, // draw a symbol
		0xbd: true, // symbol deletion window
		0xbe: true, // symbol deletion details
		0xbf: true, // delete a symbol
		0xc0: true, // clan rank privileges
		0xc1: true, // create or edit a macro
		0xc2: true, // delete a macro
		0xc5: true, // dialog answer
		0xc6: true, // try on merchant items
		0xca: true, // game guard reply
		0xcc: true, // friend message
		0xcd: true, // show mini map
		0xcf: true, // record info (view refresh)
		0xd0: true, // extended packets
	},
}

// Allowed reports whether opcode is a first-byte opcode a client in state s
// is permitted to send. A packet dispatcher should reject/drop any opcode
// for which this returns false rather than decoding it.
func Allowed(s State, opcode byte) bool {
	return allowedOpcodes[s][opcode]
}
