package netutil

import (
	"errors"
	"net"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// defaultBanTime is the default config's BanTime.
const defaultBanTime = 300 * time.Second

// defaultFilter is an AcceptFilter on the default config.
func defaultFilter() *AcceptFilter {
	return NewAcceptFilter(DefaultAcceptFilterConfig(), zerolog.Nop())
}

var (
	filterHost  = netip.MustParseAddr("192.0.2.1")
	filterOther = netip.MustParseAddr("192.0.2.2")
	filterStart = time.Unix(1_000_000, 0)
)

// allowAll runs Allow for each offset from filterStart and returns the
// answers.
func allowAll(f *AcceptFilter, addr netip.Addr, offsets ...time.Duration) []bool {
	got := make([]bool, len(offsets))
	for i, d := range offsets {
		got[i] = f.Allow(addr, filterStart.Add(d))
	}
	return got
}

func wantAnswers(t *testing.T, what string, got []bool, want ...bool) {
	t.Helper()
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: answers = %v, want %v", what, got, want)
		}
	}
}

func TestAcceptFilterBansTheFifthBurstConnection(t *testing.T) {
	f := defaultFilter()
	ms := time.Millisecond
	// The first creates the record, the next three each count a try, the
	// fifth inside a second of the previous one bans.
	wantAnswers(t, "burst", allowAll(f, filterHost, 0, 100*ms, 200*ms, 300*ms, 400*ms), true, true, true, true, false)
	// The ban holds while attempts keep coming, however slowly.
	wantAnswers(t, "banned", allowAll(f, filterHost, 900*ms, 5*time.Second, 200*time.Second), false, false, false)
	// Another address is unaffected.
	if !f.Allow(filterOther, filterStart.Add(200*time.Second)) {
		t.Fatal("another address refused while the first is banned")
	}
}

func TestAcceptFilterSlowConnectionKeepsTries(t *testing.T) {
	f := defaultFilter()
	ms := time.Millisecond
	// Three burst tries, then a connection a full second later: admitted,
	// and the tries are not cleared, so the next burst connection bans.
	wantAnswers(t, "tries", allowAll(f, filterHost, 0, 10*ms, 20*ms, 30*ms, 1030*ms, 1040*ms), true, true, true, true, true, false)
}

func TestAcceptFilterBurstWindowIsExclusive(t *testing.T) {
	f := defaultFilter()
	// Exactly one second apart is not a burst: no try is counted.
	wantAnswers(t, "spaced", allowAll(f, filterHost, 0, time.Second, 2*time.Second, 3*time.Second, 4*time.Second, 5*time.Second), true, true, true, true, true, true)
	ms := time.Millisecond
	wantAnswers(t, "burst after spacing", allowAll(f, filterHost, 5001*ms, 5002*ms, 5003*ms, 5004*ms), true, true, true, false)
}

func TestAcceptFilterSweepForgetsSilentAddresses(t *testing.T) {
	f := defaultFilter()
	ms := time.Millisecond
	allowAll(f, filterHost, 0, ms, 2*ms, 3*ms, 4*ms) // banned at 4ms
	last := filterStart.Add(4 * ms)

	f.Sweep(last.Add(defaultBanTime))
	if f.Allow(filterHost, last.Add(defaultBanTime)) {
		t.Fatal("sweep exactly defaultBanTime after the last attempt lifted the ban")
	}
	last = last.Add(defaultBanTime)
	f.Sweep(last.Add(defaultBanTime + ms))
	if !f.Allow(filterHost, last.Add(defaultBanTime+ms)) {
		t.Fatal("sweep past defaultBanTime of silence kept the ban")
	}
	// Forgotten means a fresh record: four burst connections pass again.
	at := last.Add(defaultBanTime + ms)
	for i := 1; i <= 3; i++ {
		if !f.Allow(filterHost, at.Add(time.Duration(i)*ms)) {
			t.Fatalf("burst connection %d after the sweep refused", i+1)
		}
	}
}

// filterOn starts a filtered loopback listener sweeping on ticks and
// returns it with its clock setter.
func filterOn(t *testing.T, f *AcceptFilter, ticks <-chan time.Time) (*filteredListener, func(time.Time)) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var clock atomic.Int64
	clock.Store(filterStart.UnixNano())
	now := func() time.Time { return time.Unix(0, clock.Load()) }
	l := newFilteredListener(ln, f, now, ticks, nil)
	t.Cleanup(func() { l.Close() })
	return l, func(v time.Time) { clock.Store(v.UnixNano()) }
}

func TestFilterAcceptsClosesRefusedConnectionUnanswered(t *testing.T) {
	f := defaultFilter()
	l, _ := filterOn(t, f, nil)

	accepted := make(chan net.Conn)
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				close(accepted)
				return
			}
			accepted <- conn
		}
	}()

	dial := func() net.Conn {
		conn, err := net.Dial("tcp", l.Addr().String())
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return conn
	}
	for i := 1; i <= 4; i++ {
		dial()
		if conn := <-accepted; conn == nil {
			t.Fatalf("connection %d not accepted", i)
		} else {
			conn.Close()
		}
	}
	refused := dial()
	if err := refused.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var buf [1]byte
	n, err := refused.Read(buf[:])
	var ne net.Error
	if n != 0 || err == nil || errors.As(err, &ne) && ne.Timeout() {
		t.Fatalf("refused connection read = %d, %v; want a close with no byte", n, err)
	}
	select {
	case conn := <-accepted:
		t.Fatalf("refused connection handed to the caller: %v", conn)
	default:
	}
}

func TestFilteredListenerSweepsOnEachTick(t *testing.T) {
	f := defaultFilter()
	ticks := make(chan time.Time)
	_, set := filterOn(t, f, ticks)

	ms := time.Millisecond
	allowAll(f, filterHost, 0, ms, 2*ms, 3*ms, 4*ms)
	set(filterStart.Add(4*ms + defaultBanTime + ms))
	// The second send completes only once the first tick's sweep is done.
	ticks <- time.Time{}
	ticks <- time.Time{}
	if !f.Allow(filterHost, filterStart.Add(4*ms+defaultBanTime+ms)) {
		t.Fatal("tick did not sweep the silent banned address")
	}
}

func TestFilteredListenerStopsSweepingWhenClosed(t *testing.T) {
	l, _ := filterOn(t, defaultFilter(), make(chan time.Time))
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-l.swept:
	default:
		t.Fatal("sweeping still running after Close returned")
	}
}

func TestFilteredListenerStopsSweepingWhenInnerListenerCloses(t *testing.T) {
	l, _ := filterOn(t, defaultFilter(), make(chan time.Time))
	l.Listener.Close()
	if _, err := l.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Accept on a closed listener = %v, want net.ErrClosed", err)
	}
	select {
	case <-l.swept:
	default:
		t.Fatal("sweeping still running after the inner listener closed")
	}
}
