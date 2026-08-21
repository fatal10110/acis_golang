package network

import (
	"errors"
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/link"
)

// TestDecodeClientPacketClassifiesShortPacketVsValidationErrors proves
// decodeClientPacket only routes wire.ErrShortPacket-class decode errors
// (buffer-underflow-equivalent) toward disconnect, matching
// L2GameClientPacket.Read(): other decode validation errors are logged and
// the packet dropped without ever counting toward the underflow threshold
// or disconnecting, pre-auth or post-auth. Exercised directly against
// decodeClientPacket rather than through a specific wired opcode, since no
// currently-wired decoder both returns a non-underflow validation error and
// reaches decodeClientPacket without being pre-filtered by its caller's own
// switch (e.g. an extended sub-opcode mismatch never reaches the decoder
// the outer dispatch already selected by that same sub-opcode).
func TestDecodeClientPacketClassifiesShortPacketVsValidationErrors(t *testing.T) {
	shortPacket := func([]byte) (int, error) {
		return 0, fmt.Errorf("short: %w", wire.ErrShortPacket)
	}
	validation := func([]byte) (int, error) {
		return 0, errors.New("clientpackets: invalid value")
	}

	t.Run("short packet disconnects immediately pre-auth", func(t *testing.T) {
		l := &GameClientLink{}
		client := NewClient(nil)

		if _, err := decodeClientPacket(l, client, nil, shortPacket); !errors.Is(err, errMalformedPacketDisconnect) {
			t.Fatalf("decodeClientPacket() error = %v, want errMalformedPacketDisconnect", err)
		}
	})

	t.Run("validation error never disconnects pre-auth", func(t *testing.T) {
		l := &GameClientLink{}
		client := NewClient(nil)

		if _, err := decodeClientPacket(l, client, nil, validation); errors.Is(err, errMalformedPacketDisconnect) {
			t.Fatalf("decodeClientPacket() error = %v, want no disconnect", err)
		}
	})

	t.Run("short packet tolerates first, disconnects past threshold post-auth", func(t *testing.T) {
		l := &GameClientLink{}
		client := NewClient(nil)
		client.SetAuthenticated("acc", link.SessionKey{})

		for i := range maxUnderflowsPerMin {
			if _, err := decodeClientPacket(l, client, nil, shortPacket); errors.Is(err, errMalformedPacketDisconnect) {
				t.Fatalf("decodeClientPacket() call %d disconnected before threshold", i+1)
			}
		}
		if _, err := decodeClientPacket(l, client, nil, shortPacket); !errors.Is(err, errMalformedPacketDisconnect) {
			t.Fatalf("decodeClientPacket() past threshold error = %v, want errMalformedPacketDisconnect", err)
		}
	})

	t.Run("validation error never disconnects post-auth even past threshold", func(t *testing.T) {
		l := &GameClientLink{}
		client := NewClient(nil)
		client.SetAuthenticated("acc", link.SessionKey{})

		for i := range maxUnderflowsPerMin + 2 {
			if _, err := decodeClientPacket(l, client, nil, validation); errors.Is(err, errMalformedPacketDisconnect) {
				t.Fatalf("decodeClientPacket() call %d disconnected on validation error", i+1)
			}
		}
	})
}
