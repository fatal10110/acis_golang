package network

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/link"
)

// TestMaintainStopsDuringStalledHandshake cancels Maintain while the login
// server holds the handshake: one peer accepts and never writes, the other
// sends InitLS, reads the game server's key, then goes silent. Either way
// Maintain returns at once instead of waiting out the handshake read
// deadline, and the stalled connection is closed.
func TestMaintainStopsDuringStalledHandshake(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stall func(t *testing.T, fake *fakeLoginServer, conn net.Conn)
	}{
		{"silent after accept", func(*testing.T, *fakeLoginServer, net.Conn) {}},
		{"silent after InitLS", func(t *testing.T, fake *fakeLoginServer, conn net.Conn) {
			bootstrap := crypt.NewLinkCrypt()
			if err := wire.WriteFrame(conn, bootstrap.Encrypt(link.EncodeInitLS(fake.priv.PublicKey.N.Bytes()))); err != nil {
				t.Fatalf("write InitLS: %v", err)
			}
			if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
				t.Fatal(err)
			}
			payload, err := wire.ReadFrame(conn)
			if err != nil {
				t.Fatalf("read BlowFishKey: %v", err)
			}
			if err := bootstrap.Decrypt(payload); err != nil || payload[0] != link.OpcodeBlowFishKey {
				t.Fatalf("first game server frame = %#x, %v; want BlowFishKey", payload[0], err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeLoginServer(t)
			accepted := make(chan net.Conn, 1)
			go func() {
				if conn, err := fake.ln.Accept(); err == nil {
					accepted <- conn
				}
			}()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stopped := make(chan struct{})
			go func() {
				defer close(stopped)
				auth := func() LoginServerAuth {
					return LoginServerAuth{ServerID: 1, HexID: testHexID, HostName: "*", Port: 7777, MaxPlayers: 300}
				}
				Maintain(ctx, fake.addr(), auth, LoginLinkHandlers{}, DefaultReconnectDelay, nil, zerolog.Nop())
			}()

			conn := within(t, accepted, "the link connection")
			defer conn.Close()
			tc.stall(t, fake, conn)

			cancel()
			within(t, stopped, "Maintain to return after the cancel")
			if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
				t.Fatal(err)
			}
			for {
				if _, err := conn.Read(make([]byte, 256)); err != nil {
					if errors.Is(err, os.ErrDeadlineExceeded) {
						t.Fatal("stalled link connection still open after the cancel")
					}
					return
				}
			}
		})
	}
}
