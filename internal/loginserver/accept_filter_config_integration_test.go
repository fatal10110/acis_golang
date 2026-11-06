package loginserver

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/netutil"
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/rs/zerolog"
)

// TestClientLinkAcceptFilterRaisedLimitAdmitsTheDefaultBannedBurst sends
// one burst, connections 100 ms apart from one host, to a login-client
// listener on the default filter and to one whose loginserver.properties
// raises the burst limit to ten: the default closes the fifth before Init,
// the raised limit sends Init to eleven and closes the twelfth.
func TestClientLinkAcceptFilterRaisedLimitAdmitsTheDefaultBannedBurst(t *testing.T) {
	for _, tc := range []struct {
		name, props string
		admitted    int
	}{
		{"default", "", 4},
		{"raised", "AcceptFilterBurstLimit = 10\n", 11},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := config.ParseString(tc.props)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := config.AcceptFilter(p)
			if err != nil {
				t.Fatal(err)
			}
			var clock atomic.Int64
			clock.Store(time.Unix(1_000_000, 0).UnixNano())
			now := func() time.Time { return time.Unix(0, clock.Load()) }
			filter := netutil.NewAcceptFilter(cfg, zerolog.Nop())
			addr, _, _, _, _ := newTestClientLinkOn(t, func(ln net.Listener) net.Listener {
				return netutil.FilterAccepts(ln, filter, now)
			}, newFakeAccountStore(), false)

			for range tc.admitted {
				dialLoginClient(t, addr)
				clock.Add(int64(100 * time.Millisecond))
			}
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			t.Cleanup(func() { conn.Close() })
			(&fakeLoginClient{t: t, conn: conn}).expectClosed()
		})
	}
}
