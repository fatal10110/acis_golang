package network

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"

// connState is the mutable state of one game-client connection. The
// connection's read goroutine (Handle) owns it: only that goroutine and the
// tasks it runs on the player's queue and waits for read or write it, so it
// needs no lock. Handle's deferred teardown reads the same value, so it sees
// whatever the last handler left.
type connState struct {
	// client is the connection's client; it stays the same for the whole
	// connection.
	client *Client
	// conn is the connection client's session runs over, for the waits on
	// pending persistence that tie a request to this connection.
	conn *Conn
	// key is the connection's cipher key, which VersionCheck hands the
	// client.
	key []byte
	// chars resolves the character-list slot indices RequestCharacterDelete,
	// CharacterRestore and RequestGameStart address.
	chars []*player.Character
	// entering is the player RequestGameStart restored and registered, for
	// EnterWorld to spawn; live is that player once EnterWorld took it.
	entering, live *livePlayer
	// lost records a read loop ended by a lost connection, which keeps the
	// player in the world a while before it detaches.
	lost bool
	// acted records that the previous frame's handler ran for an opcode that
	// ends spawn protection; the protection ends once that handler is done,
	// before the next frame is read, so the handler itself still sees the
	// player protected.
	acted bool
}
