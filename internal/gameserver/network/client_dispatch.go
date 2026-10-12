package network

import (
	"context"
	"errors"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
)

// errDisconnect is what an inbound handler returns to close the connection.
var errDisconnect = errors.New("network: disconnect client")

// inboundFunc handles one client frame of the opcode it is registered for.
// It returns nil to read the next frame, errDisconnect (wrapped or not) to
// close the connection, and any other error to have it logged and read on.
type inboundFunc func(l *GameClientLink, ctx context.Context, cs *connState, payload []byte) error

// inboundOpcode is a dispatch key: the frame's opcode byte, or an extended
// frame's uint16 sub-opcode.
type inboundOpcode interface{ ~uint8 | ~uint16 }

// inboundRegistry maps the opcodes of one dispatch level (the frame's
// opcode byte, or an extended frame's uint16 sub-opcode) to their handlers.
// It is filled before any connection is served and only read afterwards, so
// every connection's read goroutine may share it.
type inboundRegistry[K inboundOpcode] map[K]inboundFunc

// gameInbound holds the handlers of the frame's opcode byte. The read loop
// asks it first and falls back to its own opcode switch for an opcode with
// no entry.
var gameInbound = newGameInbound()

// newGameInbound lists every opcode-byte handler. Which opcodes a client
// may send in its current state is the state gate's business, decided
// before any of these runs.
func newGameInbound() inboundRegistry[byte] {
	reg := inboundRegistry[byte]{}
	register(reg, clientpackets.OpcodeProtocolVersion, clientpackets.DecodeProtocolVersion, (*GameClientLink).handleProtocolVersion)
	register(reg, clientpackets.OpcodeAuthLogin, clientpackets.DecodeAuthLogin, (*GameClientLink).handleAuthLogin)
	register(reg, clientpackets.OpcodeRequestNewCharacter, noPayload, (*GameClientLink).handleRequestNewCharacter)
	register(reg, clientpackets.OpcodeRequestCharacterCreate, clientpackets.DecodeRequestCharacterCreate, (*GameClientLink).handleRequestCharacterCreate)
	register(reg, clientpackets.OpcodeRequestCharacterDelete, clientpackets.DecodeRequestCharacterDelete, (*GameClientLink).handleRequestCharacterDelete)
	register(reg, clientpackets.OpcodeCharacterRestore, clientpackets.DecodeCharacterRestore, (*GameClientLink).handleCharacterRestore)
	register(reg, clientpackets.OpcodeRequestGameStart, clientpackets.DecodeRequestGameStart, (*GameClientLink).handleRequestGameStart)
	register(reg, clientpackets.OpcodeEnterWorld, noPayload, (*GameClientLink).handleEnterWorld)
	register(reg, clientpackets.OpcodeRequestPledgeCrest, clientpackets.DecodeRequestPledgeCrest, (*GameClientLink).handleRequestPledgeCrest)
	register(reg, clientpackets.OpcodeRequestAllyCrest, clientpackets.DecodeRequestAllyCrest, (*GameClientLink).handleRequestAllyCrest)
	return reg
}

// register binds op to handle, behind the one decode policy every client
// frame goes through: a frame whose decode calls for a disconnect closes the
// connection, any other decode failure (already logged) drops the frame
// and reads on, and a decoded frame reaches handle. Registering op twice is a
// wiring mistake and panics.
func register[K inboundOpcode, T any](
	reg inboundRegistry[K], op K,
	decode func([]byte) (T, error),
	handle func(*GameClientLink, context.Context, *connState, T) error,
) {
	if _, dup := reg[op]; dup {
		panic(fmt.Sprintf("network: inbound opcode %#x registered twice", op))
	}
	reg[op] = func(l *GameClientLink, ctx context.Context, cs *connState, payload []byte) error {
		req, err := decodeClientPacket(l, cs.client, payload, decode)
		if err != nil {
			if errors.Is(err, errMalformedPacketDisconnect) {
				return errDisconnect
			}
			return nil
		}
		return handle(l, ctx, cs, req)
	}
}

// serve runs the handler registered for op on payload. found reports that
// op has one; open reports whether the connection keeps reading.
func (reg inboundRegistry[K]) serve(l *GameClientLink, ctx context.Context, cs *connState, op K, payload []byte) (found, open bool) {
	handle, ok := reg[op]
	if !ok {
		return false, true
	}
	err := handle(l, ctx, cs, payload)
	switch {
	case err == nil:
		return true, true
	case errors.Is(err, errDisconnect):
		return true, false
	default:
		l.log.Warn().Err(err).Str("opcode", fmt.Sprintf("%#x", op)).Str("account", cs.client.AccountName()).
			Msg("game client handler")
		return true, true
	}
}
