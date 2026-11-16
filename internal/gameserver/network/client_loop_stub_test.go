package network

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/rs/zerolog"
)

// TestEveryAcceptedOpcodeHasAHandler pins that the state gate and the
// registry agree in both directions: an opcode a client may send in some
// state always reaches a handler, so the read loop's not-implemented
// fallback stays a guard, and a registered handler is always reachable in
// some state, so a new register line without its gate entry cannot sit
// behind a gate that rejects every frame.
func TestEveryAcceptedOpcodeHasAHandler(t *testing.T) {
	for state, ops := range allowedOpcodes {
		for op := range ops {
			if _, ok := gameInbound[op]; !ok {
				t.Errorf("opcode %#x accepted in state %s has no gameInbound handler", op, state)
			}
		}
	}
	for op := range gameInbound {
		allowed := false
		for _, ops := range allowedOpcodes {
			if ops[op] {
				allowed = true
				break
			}
		}
		if !allowed {
			t.Errorf("gameInbound handler for opcode %#x is not allowed in any state", op)
		}
	}
}

// TestSilentOpcodesLogNothing pins the frames read and dropped without a
// word: nothing is logged or counted, and only the opcodes no request is
// built for leave the player not having acted.
func TestSilentOpcodesLogNothing(t *testing.T) {
	for _, op := range append(append([]byte(nil), unwiredOpcodes...), noRequestOpcodes...) {
		t.Run(fmt.Sprintf("%#x", op), func(t *testing.T) {
			var logged bytes.Buffer
			l := newBareGameClientLink(t, GameClientLinkConfig{Log: zerolog.New(&logged)})
			cs := &connState{client: newDispatchTestClient(t), acted: true}

			if open := l.dispatch(context.Background(), cs, []byte{op, 0x01, 0x02}); !open {
				t.Fatalf("dispatch(%#x) open = false, want true", op)
			}
			if got := logged.String(); got != "" {
				t.Errorf("log = %q, want nothing", got)
			}
			if cs.client.unknownCount != 0 {
				t.Errorf("unknown-packet count = %d, want 0", cs.client.unknownCount)
			}
			if want := !slices.Contains(noRequestOpcodes, op); cs.acted != want {
				t.Errorf("acted = %v, want %v", cs.acted, want)
			}
		})
	}
}

// TestDispatchUnregisteredOpcodeLogsNotImplemented pins the fallback for an
// accepted opcode with no handler: the frame is dropped, the connection
// reads on, and the info line keeps its exact format.
func TestDispatchUnregisteredOpcodeLogsNotImplemented(t *testing.T) {
	const op byte = 0xfe
	if _, ok := gameInbound[op]; ok {
		t.Fatalf("opcode %#x has a handler; pick an unregistered one", op)
	}
	var logged bytes.Buffer
	l := newBareGameClientLink(t, GameClientLinkConfig{Log: zerolog.New(&logged)})
	cs := &connState{client: newDispatchTestClient(t)}

	if open := l.dispatch(context.Background(), cs, []byte{op}); !open {
		t.Fatal("dispatch(unregistered) open = false, want true")
	}
	const want = `{"level":"info","opcode":"0xfe","state":"connected","message":"game client: accepted opcode not implemented yet"}` + "\n"
	if got := logged.String(); got != want {
		t.Errorf("log = %q, want %q", got, want)
	}
}

// TestUnwiredOpcodesInGameAnswerNothing drives every unwired opcode through
// a live in-game connection past the unknown-packet threshold: none of them
// is answered or counted, so the next request's answer is the first frame
// the client reads.
func TestUnwiredOpcodesInGameAnswerNothing(t *testing.T) {
	c, _, _, _ := newLinkedGameClientEnterWorld(t)

	for _, op := range unwiredOpcodes {
		for i := 0; i < maxUnknownPerMin+1; i++ {
			c.Send([]byte{op})
		}
	}
	c.Send(encodeRequestManorList())
	if frame := c.Read(); frame[0] != serverpackets.OpcodeExtended {
		t.Fatalf("first frame after unwired opcodes = %#x, want ExSendManorList under Extended (%#x)", frame[0], serverpackets.OpcodeExtended)
	}
}
