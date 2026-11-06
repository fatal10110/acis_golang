package netutil

import (
	"bytes"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestDefaultAcceptFilterConfigIsTheShippedFilter(t *testing.T) {
	want := AcceptFilterConfig{Enabled: true, BurstWindow: time.Second, BurstLimit: 3, BanTime: 300 * time.Second}
	got := DefaultAcceptFilterConfig()
	if got.Enabled != want.Enabled || got.BurstWindow != want.BurstWindow || got.BurstLimit != want.BurstLimit ||
		got.BanTime != want.BanTime || len(got.Allowlist) != 0 {
		t.Fatalf("DefaultAcceptFilterConfig() = %+v, want %+v", got, want)
	}
}

// spaced returns n offsets step apart, from zero.
func spaced(n int, step time.Duration) []time.Duration {
	out := make([]time.Duration, n)
	for i := range out {
		out[i] = time.Duration(i) * step
	}
	return out
}

func TestAcceptFilterRaisedLimitAdmitsTheBurstTheDefaultBans(t *testing.T) {
	cfg := DefaultAcceptFilterConfig()
	cfg.BurstLimit = 10
	burst := spaced(12, 100*time.Millisecond)
	// The first plus ten tries pass, the twelfth bans.
	got := allowAll(NewAcceptFilter(cfg, zerolog.Nop()), filterHost, burst...)
	wantAnswers(t, "raised", got, true, true, true, true, true, true, true, true, true, true, true, false)
	// The default bans the fifth of the same burst.
	wantAnswers(t, "default", allowAll(defaultFilter(), filterHost, burst...), true, true, true, true, false)
}

func TestAcceptFilterWiderWindowCountsSlowerBursts(t *testing.T) {
	cfg := DefaultAcceptFilterConfig()
	cfg.BurstWindow = 2 * time.Second
	slow := spaced(6, 1500*time.Millisecond)
	wantAnswers(t, "2 s window", allowAll(NewAcceptFilter(cfg, zerolog.Nop()), filterHost, slow...), true, true, true, true, false, false)
	// 1.5 s apart is no burst under the default 1 s window.
	wantAnswers(t, "default", allowAll(defaultFilter(), filterHost, slow...), true, true, true, true, true, true)
}

func TestAcceptFilterBanTimeIsConfigurable(t *testing.T) {
	cfg := DefaultAcceptFilterConfig()
	cfg.BanTime = 10 * time.Second
	f := NewAcceptFilter(cfg, zerolog.Nop())
	ms := time.Millisecond
	allowAll(f, filterHost, 0, ms, 2*ms, 3*ms, 4*ms) // banned at 4ms
	last := filterStart.Add(4 * ms)
	f.Sweep(last.Add(10 * time.Second))
	if f.Allow(filterHost, last.Add(10*time.Second)) {
		t.Fatal("sweep exactly BanTime after the last attempt lifted the ban")
	}
	last = last.Add(10 * time.Second)
	f.Sweep(last.Add(10*time.Second + ms))
	if !f.Allow(filterHost, last.Add(10*time.Second+ms)) {
		t.Fatal("sweep past BanTime of silence kept the ban")
	}
}

func TestAcceptFilterAllowlistIsNeverRecorded(t *testing.T) {
	cfg := DefaultAcceptFilterConfig()
	cfg.Allowlist = []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("2001:db8::1/128")}
	f := NewAcceptFilter(cfg, zerolog.Nop())
	for i := range 500 {
		at := filterStart.Add(time.Duration(i) * time.Millisecond)
		if !f.Allow(filterHost, at) || !f.Allow(netip.MustParseAddr("2001:db8::1"), at) {
			t.Fatalf("allowlisted connection %d refused", i+1)
		}
	}
	if n := len(f.holders); n != 0 {
		t.Fatalf("allowlisted addresses recorded: %d holders", n)
	}
	outside := netip.MustParseAddr("198.51.100.1")
	ms := time.Millisecond
	wantAnswers(t, "outside", allowAll(f, outside, 0, ms, 2*ms, 3*ms, 4*ms), true, true, true, true, false)
}

func TestAcceptFilterLogsEachBanOnceAtWarn(t *testing.T) {
	var out bytes.Buffer
	f := NewAcceptFilter(DefaultAcceptFilterConfig(), zerolog.New(&out))
	ms := time.Millisecond
	// The fifth bans; the refusals after it do not log again.
	allowAll(f, filterHost, 0, ms, 2*ms, 3*ms, 4*ms, 5*ms, 6*ms)
	want := `{"level":"warn","ip":"192.0.2.1","burst_window":1000,"burst_limit":3,"ban_time":300000,"message":"connection burst; address banned at accept"}` + "\n"
	if out.String() != want {
		t.Fatalf("log =\n%s\nwant\n%s", out.String(), want)
	}
	if strings.Count(out.String(), "\n") != 1 {
		t.Fatal("ban logged more than once")
	}
}

func TestFilterAcceptsDisabledLeavesTheListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	cfg := DefaultAcceptFilterConfig()
	cfg.Enabled = false
	if got := FilterAccepts(ln, NewAcceptFilter(cfg, zerolog.Nop()), time.Now); got != ln {
		t.Fatalf("FilterAccepts with the filter off = %T, want the listener itself", got)
	}
}
