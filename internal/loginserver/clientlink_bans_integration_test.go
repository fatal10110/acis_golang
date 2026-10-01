package loginserver

import (
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

func TestClientLinkFailedPasswordsBanIPAtThreshold(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, _, _, bans := newTestClientLink(t, accounts, false)
	ip := net.ParseIP("127.0.0.1")

	for i := 1; i <= 3; i++ {
		c := dialLoginClient(t, addr)
		c.gameGuard()
		c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "wrong"))
		reply := c.read()
		if reply[0] != serverpackets.OpcodeLoginFail {
			t.Fatalf("attempt %d opcode = %#x, want LoginFail (%#x)", i, reply[0], serverpackets.OpcodeLoginFail)
		}
		c.expectClosed()

		if i < 3 && bans.IsBanned(ip) {
			t.Fatalf("attempt %d banned IP before threshold", i)
		}
	}

	if !bans.IsBanned(ip) {
		t.Fatal("IP was not banned after failed login threshold")
	}
}

func TestClientLinkFailedPasswordAttemptsResetOnSuccess(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, _, sessions, bans := newTestClientLink(t, accounts, false)
	ip := net.ParseIP("127.0.0.1")

	for i := 1; i <= 2; i++ {
		c := dialLoginClient(t, addr)
		c.gameGuard()
		c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "wrong"))
		reply := c.read()
		if reply[0] != serverpackets.OpcodeLoginFail {
			t.Fatalf("attempt %d opcode = %#x, want LoginFail (%#x)", i, reply[0], serverpackets.OpcodeLoginFail)
		}
		c.expectClosed()
	}

	c := dialLoginClient(t, addr)
	c.login(l, "player1", "s3cret")
	if err := c.conn.Close(); err != nil {
		t.Fatalf("close login client: %v", err)
	}
	waitSessionMissing(t, sessions, "player1")

	c = dialLoginClient(t, addr)
	c.gameGuard()
	c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "wrong"))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeLoginFail {
		t.Fatalf("opcode = %#x, want LoginFail (%#x)", reply[0], serverpackets.OpcodeLoginFail)
	}
	c.expectClosed()

	if bans.IsBanned(ip) {
		t.Fatal("IP was banned after a successful login reset failed attempts")
	}
}

func TestClientLinkUnknownAccountAutoCreateOffCountsFailedAttempts(t *testing.T) {
	accounts := newFakeAccountStore()
	addr, l, _, _, bans := newTestClientLink(t, accounts, false)
	ip := net.ParseIP("127.0.0.1")

	for i := 1; i <= 3; i++ {
		c := dialLoginClient(t, addr)
		c.gameGuard()
		c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "missing", "s3cret"))
		reply := c.read()
		if reply[0] != serverpackets.OpcodeLoginFail {
			t.Fatalf("attempt %d opcode = %#x, want LoginFail (%#x)", i, reply[0], serverpackets.OpcodeLoginFail)
		}
		c.expectClosed()
	}

	if !bans.IsBanned(ip) {
		t.Fatal("IP was not banned after unknown-account failed login threshold")
	}
}

// newFailedAttemptLink builds a ClientLink with only what the failed-attempt
// bookkeeping touches, so the window can be driven with explicit times.
func newFailedAttemptLink() (*ClientLink, *manager.IPBanList) {
	bans := manager.NewIPBanList(zerolog.Nop())
	return &ClientLink{
		bans:               bans,
		loginTryBeforeBan:  DefaultLoginTryBeforeBan,
		loginBlockAfterBan: DefaultLoginBlockAfterBan,
		log:                zerolog.Nop(),
	}, bans
}

func failedAttemptCount(l *ClientLink) int {
	l.failedMu.Lock()
	defer l.failedMu.Unlock()
	return len(l.failedAttempts)
}

// testFailedAttemptWindow returns l's expiring failed-attempt window,
// failing the test when the configuration keeps counts forever.
func testFailedAttemptWindow(t *testing.T, l *ClientLink) time.Duration {
	t.Helper()
	window, ok := l.failedAttemptWindow()
	if !ok {
		t.Fatalf("failed-attempt counts never expire with block %v", l.loginBlockAfterBan)
	}
	return window
}

func TestClientLinkFailedAttemptWindow(t *testing.T) {
	l, _ := newFailedAttemptLink()
	if got := testFailedAttemptWindow(t, l); got != time.Hour {
		t.Fatalf("default window = %v, want 1h", got)
	}
	l.loginBlockAfterBan = 3 * time.Hour
	if got := testFailedAttemptWindow(t, l); got != 3*time.Hour {
		t.Fatalf("window with 3h block = %v, want 3h (never shorter than the ban)", got)
	}
	l.loginBlockAfterBan = time.Second
	if got := testFailedAttemptWindow(t, l); got != time.Hour {
		t.Fatalf("window with 1s block = %v, want the 1h floor", got)
	}
	for _, block := range []time.Duration{0, -time.Second} {
		l.loginBlockAfterBan = block
		if window, ok := l.failedAttemptWindow(); ok {
			t.Fatalf("permanent-ban block %v expires counts after %v, want never", block, window)
		}
	}
}

// TestClientLinkPermanentBanCountsNeverExpire: with a permanent ban, pacing
// failures more than an hour apart must still reach the threshold, and a
// sweep must keep the count.
func TestClientLinkPermanentBanCountsNeverExpire(t *testing.T) {
	l, bans := newFailedAttemptLink()
	l.loginBlockAfterBan = 0
	ip := net.ParseIP("192.0.2.5")
	t0 := time.Now()
	gap := minFailedAttemptWindow + time.Second

	for i := range DefaultLoginTryBeforeBan - 1 {
		at := t0.Add(time.Duration(i) * gap)
		l.recordFailedAttempt(ip, at)
		l.sweepFailedAttempts(at.Add(gap))
		if bans.IsBanned(ip) {
			t.Fatalf("attempt %d banned before the threshold", i+1)
		}
	}
	if got := failedAttemptCount(l); got != 1 {
		t.Fatalf("tracked addresses after sweeps = %d, want 1", got)
	}
	l.recordFailedAttempt(ip, t0.Add(time.Duration(DefaultLoginTryBeforeBan-1)*gap))
	if !bans.IsBanned(ip) {
		t.Fatal("failures paced more than an hour apart were never banned under a permanent-ban config")
	}
}

// TestClientLinkSubThresholdFailuresFromManyIPsAreSwept drives the
// cycling-address shape: many addresses each stop one failure short of the
// ban. Once they have been idle past the window a sweep drops them all,
// while an address still inside the window keeps its count toward the ban.
func TestClientLinkSubThresholdFailuresFromManyIPsAreSwept(t *testing.T) {
	l, bans := newFailedAttemptLink()
	window := testFailedAttemptWindow(t, l)
	t0 := time.Now()

	const cycled = 2000
	for i := range cycled {
		ip := net.IPv4(10, byte(i>>16), byte(i>>8), byte(i))
		for range DefaultLoginTryBeforeBan - 1 {
			l.recordFailedAttempt(ip, t0)
		}
		if bans.IsBanned(ip) {
			t.Fatalf("ip %s banned below the threshold", ip)
		}
	}
	recent := net.ParseIP("192.0.2.1")
	for range DefaultLoginTryBeforeBan - 1 {
		l.recordFailedAttempt(recent, t0.Add(window/2))
	}
	if got := failedAttemptCount(l); got != cycled+1 {
		t.Fatalf("tracked addresses = %d, want %d", got, cycled+1)
	}

	sweepAt := t0.Add(window + time.Second)
	l.sweepFailedAttempts(sweepAt)
	if got := failedAttemptCount(l); got != 1 {
		t.Fatalf("tracked addresses after sweep = %d, want 1 (only the in-window address)", got)
	}

	l.recordFailedAttempt(recent, sweepAt)
	if !bans.IsBanned(recent) {
		t.Fatal("in-window address was not banned at the threshold after a sweep")
	}
	if got := failedAttemptCount(l); got != 0 {
		t.Fatalf("tracked addresses after ban = %d, want 0", got)
	}
}

func TestClientLinkFailedAttemptsWithinWindowStillBan(t *testing.T) {
	l, bans := newFailedAttemptLink()
	window := testFailedAttemptWindow(t, l)
	ip := net.ParseIP("192.0.2.2")
	t0 := time.Now()

	// Consecutive failures exactly one window apart still accumulate.
	for i := range DefaultLoginTryBeforeBan {
		l.recordFailedAttempt(ip, t0.Add(time.Duration(i)*window))
	}
	if !bans.IsBanned(ip) {
		t.Fatal("failures spaced exactly one window apart did not ban")
	}
}

func TestClientLinkFailedAttemptsIdlePastWindowRestartCount(t *testing.T) {
	l, bans := newFailedAttemptLink()
	window := testFailedAttemptWindow(t, l)
	ip := net.ParseIP("192.0.2.3")
	t0 := time.Now()

	for range DefaultLoginTryBeforeBan - 1 {
		l.recordFailedAttempt(ip, t0)
	}
	// No sweep ran: the stale count is discarded when the next failure lands.
	restart := t0.Add(window + time.Second)
	l.recordFailedAttempt(ip, restart)
	if bans.IsBanned(ip) {
		t.Fatal("failure after an idle window counted toward the old attempts")
	}
	for range DefaultLoginTryBeforeBan - 1 {
		l.recordFailedAttempt(ip, restart)
	}
	if !bans.IsBanned(ip) {
		t.Fatal("restarted count did not ban at the threshold")
	}
}

func TestClientLinkPurgeLoopSweepsFailedAttempts(t *testing.T) {
	_, l, _, _, _ := newTestClientLink(t, newFakeAccountStore(), false, func(l *ClientLink) {
		l.loginTimeout = 20 * time.Millisecond
		l.failedAttempts = map[string]failedAttempt{
			"192.0.2.4": {count: 1, last: time.Now().Add(-2 * testFailedAttemptWindow(t, l))},
		}
	})

	deadline := time.Now().Add(2 * time.Second)
	for failedAttemptCount(l) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("purge loop did not sweep an expired failed-attempt count")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestClientLinkFailedAttemptsConcurrentAccess(t *testing.T) {
	l, _ := newFailedAttemptLink()
	window := testFailedAttemptWindow(t, l)
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for i := range 200 {
				ip := net.IPv4(10, 1, byte(g), byte(i))
				now := time.Now()
				l.recordFailedAttempt(ip, now)
				if i%3 == 0 {
					l.clearFailedAttempts(ip)
				}
				l.sweepFailedAttempts(now.Add(2 * window))
			}
		})
	}
	wg.Wait()
	if got := failedAttemptCount(l); got != 0 {
		t.Fatalf("tracked addresses = %d, want 0 after sweeping past the window", got)
	}
}

// syncBuffer is an io.Writer safe for a logger shared across goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

// Addresses banned at the threshold and never seen again lose their ban
// entries once a sweep runs past the ban duration, while a permanent ban
// stays. A swept ban no longer answers IsBanned; an unswept one would still
// be active here, since the list's own clock has not reached its expiry.
func TestClientLinkBanSweepDropsExpiredBansWithoutReconnect(t *testing.T) {
	l, bans := newFailedAttemptLink()
	permanent := net.ParseIP("192.0.2.30")
	bans.Ban(permanent, 0)

	now := time.Now()
	var banned []net.IP
	for i := range 500 {
		ip := net.IPv4(10, 3, byte(i>>8), byte(i))
		for range DefaultLoginTryBeforeBan {
			l.recordFailedAttempt(ip, now)
		}
		banned = append(banned, ip)
	}
	if !bans.IsBanned(banned[0]) {
		t.Fatal("threshold failures did not ban")
	}

	l.bans.SweepExpired(now.Add(DefaultLoginBlockAfterBan-time.Second), l.hasLiveConnection)
	if !bans.IsBanned(banned[1]) {
		t.Fatal("sweep before the ban duration elapsed removed the ban")
	}

	l.bans.SweepExpired(now.Add(DefaultLoginBlockAfterBan+time.Second), l.hasLiveConnection)
	for _, ip := range banned {
		if bans.IsBanned(ip) {
			t.Fatalf("expired ban for %v survived the sweep", ip)
		}
	}
	if !bans.IsBanned(permanent) {
		t.Fatal("sweep removed a permanent ban")
	}
}

// Connections opened before their address was banned keep the expired ban
// alive through sweeps, so their failures after it expires change nothing,
// as without any sweep: the threshold-crossing ban lands on the expired
// entry, has no effect, and the next check lifts the entry.
func TestClientLinkBanSweepKeepsExpiredBanForOpenConnections(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, _, _, bans := newTestClientLink(t, accounts, false)
	ip := net.ParseIP("127.0.0.1")

	clients := make([]*fakeLoginClient, DefaultLoginTryBeforeBan)
	for i := range clients {
		clients[i] = dialLoginClient(t, addr)
		clients[i].gameGuard()
	}

	bans.Ban(ip, time.Nanosecond) // a ban that has already run out
	sweepAt := time.Now().Add(time.Second)
	if !l.hasLiveConnection(ip.String()) {
		t.Fatal("open connections are not tracked")
	}
	l.bans.SweepExpired(sweepAt, l.hasLiveConnection)

	for i, c := range clients {
		c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "wrong"))
		if reply := c.read(); reply[0] != serverpackets.OpcodeLoginFail {
			t.Fatalf("attempt %d opcode = %#x, want LoginFail (%#x)", i+1, reply[0], serverpackets.OpcodeLoginFail)
		}
		c.expectClosed()
	}

	if bans.IsBanned(ip) {
		t.Fatal("threshold ban took effect over an expired entry the sweep should have kept")
	}
	deadline := time.Now().Add(2 * time.Second)
	for l.hasLiveConnection(ip.String()) {
		if time.Now().After(deadline) {
			t.Fatal("closed connections are still tracked")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestClientLinkPurgeLoopSweepsExpiredBans(t *testing.T) {
	var logs syncBuffer
	bans := manager.NewIPBanList(zerolog.New(&logs))
	bans.Ban(net.ParseIP("192.0.2.40"), time.Nanosecond)
	bans.Ban(net.ParseIP("192.0.2.41"), 0)
	newTestClientLink(t, newFakeAccountStore(), false, func(l *ClientLink) {
		l.loginTimeout = 20 * time.Millisecond
		l.bans = bans
	})

	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(logs.String(), `"address":"192.0.2.40","message":"removed expired IP address ban"`) {
		if time.Now().After(deadline) {
			t.Fatalf("purge loop did not sweep an expired ban; log: %s", logs.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if strings.Contains(logs.String(), "192.0.2.41") {
		t.Fatal("purge loop swept a permanent ban")
	}
	if !bans.IsBanned(net.ParseIP("192.0.2.41")) {
		t.Fatal("permanent ban lost")
	}
}

func TestClientLinkBanSweepConcurrentAccess(t *testing.T) {
	l, bans := newFailedAttemptLink()
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for i := range 200 {
				ip := net.IPv4(10, 4, byte(g), byte(i))
				l.trackConnection(ip)
				_ = bans.IsBanned(ip)
				bans.Ban(ip, time.Nanosecond)
				l.untrackConnection(ip)
				bans.SweepExpired(time.Now().Add(time.Second), l.hasLiveConnection)
			}
		})
	}
	wg.Wait()
	bans.SweepExpired(time.Now().Add(time.Second), l.hasLiveConnection)
	for g := range 8 {
		for i := range 200 {
			if l.hasLiveConnection(net.IPv4(10, 4, byte(g), byte(i)).String()) {
				t.Fatal("connection count leaked")
			}
		}
	}
}
