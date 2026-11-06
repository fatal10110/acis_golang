package config

import (
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/netutil"
)

func TestAcceptFilterAbsentKeysKeepTheDefaults(t *testing.T) {
	t.Parallel()
	p, err := ParseString("GameserverPort = 7777\n")
	if err != nil {
		t.Fatal(err)
	}
	got, err := AcceptFilter(p)
	if err != nil {
		t.Fatal(err)
	}
	want := netutil.DefaultAcceptFilterConfig()
	if got.Enabled != want.Enabled || got.BurstWindow != want.BurstWindow || got.BurstLimit != want.BurstLimit ||
		got.BanTime != want.BanTime || got.Allowlist != nil {
		t.Fatalf("AcceptFilter() = %+v, want the defaults %+v", got, want)
	}
}

func TestAcceptFilterReadsEveryKey(t *testing.T) {
	t.Parallel()
	p, err := ParseString(strings.Join([]string{
		"EnableAcceptFilter = False",
		"AcceptFilterBurstTime = 250",
		"AcceptFilterBurstLimit = 600",
		"AcceptFilterBanTime = 30",
		"AcceptFilterAllowlist = 10.1.2.3, 192.168.0.0/16;2001:db8::/32 ::ffff:172.16.0.1 ::ffff:172.16.0.0/108",
	}, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := AcceptFilter(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled || got.BurstWindow != 250*time.Millisecond || got.BurstLimit != 600 || got.BanTime != 30*time.Second {
		t.Fatalf("AcceptFilter() = %+v", got)
	}
	want := []netip.Prefix{
		netip.MustParsePrefix("10.1.2.3/32"), netip.MustParsePrefix("192.168.0.0/16"),
		netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("172.16.0.1/32"),
		netip.MustParsePrefix("172.16.0.0/12"),
	}
	if !slices.Equal(got.Allowlist, want) {
		t.Fatalf("Allowlist = %v, want %v", got.Allowlist, want)
	}

	on, err := ParseString("EnableAcceptFilter = true\nAcceptFilterAllowlist =\n")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := AcceptFilter(on); err != nil || !got.Enabled || len(got.Allowlist) != 0 {
		t.Fatalf("AcceptFilter(on, empty allowlist) = %+v, %v", got, err)
	}
}

func TestAcceptFilterRejectsMalformedValues(t *testing.T) {
	t.Parallel()
	for _, line := range []string{
		"AcceptFilterBurstTime = 1s",
		"AcceptFilterBurstTime = -1",
		"AcceptFilterBurstLimit = many",
		"AcceptFilterBurstLimit = -3",
		"AcceptFilterBanTime = -300",
		"AcceptFilterAllowlist = 10.0.0.256",
		"AcceptFilterAllowlist = 10.0.0.0/33",
		"AcceptFilterAllowlist = loadtest.local",
		"AcceptFilterAllowlist = fe80::1%eth0",
		"AcceptFilterAllowlist = ::ffff:0.0.0.0/80",
	} {
		p, err := ParseString(line + "\n")
		if err != nil {
			t.Fatal(err)
		}
		if got, err := AcceptFilter(p); err == nil {
			t.Errorf("AcceptFilter(%q) = %+v, want an error", line, got)
		}
	}
}
