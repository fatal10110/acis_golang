package network

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamecipher "github.com/fatal10110/acis_golang/internal/gameserver/network/cipher"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/rs/zerolog"
)

// newDispatchTestClient returns a client over a session that discards what
// it is sent; a disconnect sends ServerClose, so the session must be live.
func newDispatchTestClient(t *testing.T) *Client {
	t.Helper()
	cipher, err := gamecipher.NewCipher(make([]byte, gamecipher.KeySize))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	conn := newConn(discardConn{}, zerolog.Nop())
	t.Cleanup(func() { conn.Close() })
	return NewClient(NewSession(conn, cipher))
}

func TestInboundRegistryServe(t *testing.T) {
	const op byte = 0x42
	errHandler := errors.New("handler gave up")
	decodeOK := func(p []byte) (int, error) { return len(p), nil }
	decodeShort := func([]byte) (int, error) { return 0, fmt.Errorf("short: %w", wire.ErrShortPacket) }
	decodeInvalid := func([]byte) (int, error) { return 0, errors.New("clientpackets: invalid value") }

	tests := []struct {
		name       string
		authed     bool
		decode     func([]byte) (int, error)
		handlerErr error
		wantCalled bool
		wantOpen   bool
		wantUnread bool
		wantLog    string
	}{
		{name: "short frame pre-auth disconnects without running the handler", decode: decodeShort, wantUnread: true},
		{name: "short frame post-auth under the threshold drops the frame", authed: true, decode: decodeShort, wantOpen: true, wantUnread: true},
		{name: "invalid field drops the frame and reads on", decode: decodeInvalid, wantOpen: true, wantUnread: true},
		{name: "handler nil reads on", decode: decodeOK, wantCalled: true, wantOpen: true},
		{name: "handler errDisconnect closes", decode: decodeOK, handlerErr: errDisconnect, wantCalled: true},
		{name: "handler wrapped errDisconnect closes", decode: decodeOK, handlerErr: fmt.Errorf("auth: %w", errDisconnect), wantCalled: true},
		{name: "handler other error is logged and reads on", decode: decodeOK, handlerErr: errHandler, wantCalled: true, wantOpen: true, wantLog: errHandler.Error()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var logged bytes.Buffer
			l := &GameClientLink{log: zerolog.New(&logged)}
			cs := &connState{client: newDispatchTestClient(t)}
			if tc.authed {
				cs.client.SetAuthenticated("acc", link.SessionKey{})
			}
			payload := []byte{op, 1, 2, 3}

			var called bool
			var got int
			reg := inboundRegistry[byte]{}
			register(reg, op, tc.decode, func(gotL *GameClientLink, _ context.Context, gotCS *connState, req int) error {
				if gotL != l || gotCS != cs {
					t.Errorf("handler got link %p state %p, want %p %p", gotL, gotCS, l, cs)
				}
				called, got = true, req
				return tc.handlerErr
			})

			found, open := reg.serve(l, context.Background(), cs, op, payload)
			if !found {
				t.Fatalf("serve(%#x) found = false, want true", op)
			}
			if open != tc.wantOpen {
				t.Errorf("serve open = %v, want %v", open, tc.wantOpen)
			}
			if called != tc.wantCalled {
				t.Errorf("handler called = %v, want %v", called, tc.wantCalled)
			}
			if called && got != len(payload) {
				t.Errorf("handler req = %d, want the decoded %d", got, len(payload))
			}
			if cs.client.frameUnread != tc.wantUnread {
				t.Errorf("frameUnread = %v, want %v", cs.client.frameUnread, tc.wantUnread)
			}
			if tc.wantLog != "" && !strings.Contains(logged.String(), tc.wantLog) {
				t.Errorf("log = %q, want it to carry %q", logged.String(), tc.wantLog)
			}
		})
	}
}

func TestInboundRegistryServeUnregisteredOpcode(t *testing.T) {
	reg := inboundRegistry[byte]{}
	register(reg, 0x01, func([]byte) (int, error) { return 0, nil },
		func(*GameClientLink, context.Context, *connState, int) error {
			t.Fatal("handler of another opcode ran")
			return nil
		})
	cs := &connState{client: newDispatchTestClient(t)}

	found, open := reg.serve(&GameClientLink{}, context.Background(), cs, 0x02, []byte{0x02})
	if found || !open {
		t.Fatalf("serve(unregistered) = found %v open %v, want false true", found, open)
	}
}

// Extended frames key their handlers by the uint16 sub-opcode; the decoder
// still reads the whole frame.
func TestInboundRegistryServeExtendedSubOpcode(t *testing.T) {
	const sub uint16 = 0x0010
	reg := inboundRegistry[uint16]{}
	var got []byte
	register(reg, sub, func(p []byte) ([]byte, error) { return p, nil },
		func(_ *GameClientLink, _ context.Context, _ *connState, req []byte) error {
			got = req
			return nil
		})
	cs := &connState{client: newDispatchTestClient(t)}
	payload := []byte{0xd0, 0x10, 0x00, 0x07}

	found, open := reg.serve(&GameClientLink{}, context.Background(), cs, sub, payload)
	if !found || !open {
		t.Fatalf("serve(%#x) = found %v open %v, want true true", sub, found, open)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("decoder saw %x, want the whole frame %x", got, payload)
	}
}

func TestRegisterTwiceForOneOpcodePanics(t *testing.T) {
	reg := inboundRegistry[byte]{}
	decode := func([]byte) (int, error) { return 0, nil }
	handle := func(*GameClientLink, context.Context, *connState, int) error { return nil }
	register(reg, 0x01, decode, handle)

	defer func() {
		if recover() == nil {
			t.Fatal("second register for one opcode did not panic")
		}
	}()
	register(reg, 0x01, decode, handle)
}
