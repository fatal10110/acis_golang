package config

import (
	"fmt"
	"math"
	"net/netip"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/netutil"
)

// AcceptFilter reads the client-port accept filter keys of p, the same in
// loginserver.properties and server.properties. An absent key keeps its
// netutil.DefaultAcceptFilterConfig value without a missing-key warning:
// shipped files carry none of these keys.
//
//	EnableAcceptFilter       on/off
//	AcceptFilterBurstTime    burst window, ms
//	AcceptFilterBurstLimit   burst connections allowed before the ban
//	AcceptFilterBanTime      seconds of silence that lift a ban
//	AcceptFilterAllowlist    IPs and CIDR prefixes never filtered
func AcceptFilter(p *Properties) (netutil.AcceptFilterConfig, error) {
	cfg := netutil.DefaultAcceptFilterConfig()
	if value, ok := p.Lookup("EnableAcceptFilter"); ok {
		cfg.Enabled = strings.EqualFold(value, "true")
	}
	if d, ok, err := nonNegativeDuration(p, "AcceptFilterBurstTime", time.Millisecond); err != nil {
		return netutil.AcceptFilterConfig{}, err
	} else if ok {
		cfg.BurstWindow = d
	}
	if limit, ok, err := nonNegativeInt(p, "AcceptFilterBurstLimit"); err != nil {
		return netutil.AcceptFilterConfig{}, err
	} else if ok {
		cfg.BurstLimit = limit
	}
	if d, ok, err := nonNegativeDuration(p, "AcceptFilterBanTime", time.Second); err != nil {
		return netutil.AcceptFilterConfig{}, err
	} else if ok {
		cfg.BanTime = d
	}
	if value, ok := p.Lookup("AcceptFilterAllowlist"); ok {
		for _, entry := range splitTrimTrailingEmpty(defaultDelimitersRE, value) {
			if entry == "" {
				continue
			}
			prefix, err := allowlistPrefix(entry)
			if err != nil {
				return netutil.AcceptFilterConfig{}, fmt.Errorf("parse AcceptFilterAllowlist entry %q: %w", entry, err)
			}
			cfg.Allowlist = append(cfg.Allowlist, prefix)
		}
	}
	return cfg, nil
}

func nonNegativeInt(p *Properties, key string) (int, bool, error) {
	n, ok, err := p.OptionalInt(key)
	if err == nil && n < 0 {
		err = fmt.Errorf("%s = %d: must not be negative", key, n)
	}
	return n, ok, err
}

// nonNegativeDuration reads key as a count of unit. A count whose duration
// would overflow is rejected like a negative one: a wrapped value turns into
// a negative window that disables the filter or forgets every ban.
func nonNegativeDuration(p *Properties, key string, unit time.Duration) (time.Duration, bool, error) {
	n, ok, err := nonNegativeInt(p, key)
	if err != nil || !ok {
		return 0, ok, err
	}
	if int64(n) > math.MaxInt64/int64(unit) {
		return 0, false, fmt.Errorf("%s = %d: too large", key, n)
	}
	return time.Duration(n) * unit, true, nil
}

// allowlistPrefix parses an address, as a single-address prefix, or a CIDR
// prefix. An IPv4-mapped IPv6 one is reduced to IPv4 form, the form the
// filter sees IPv4 clients in.
func allowlistPrefix(entry string) (netip.Prefix, error) {
	if !strings.Contains(entry, "/") {
		addr, err := netip.ParseAddr(entry)
		if err != nil {
			return netip.Prefix{}, err
		}
		if addr.Zone() != "" {
			return netip.Prefix{}, fmt.Errorf("address zone not allowed")
		}
		addr = addr.Unmap()
		return netip.PrefixFrom(addr, addr.BitLen()), nil
	}
	prefix, err := netip.ParsePrefix(entry)
	if err != nil {
		return netip.Prefix{}, err
	}
	if addr := prefix.Addr(); addr.Is4In6() {
		if prefix.Bits() < 96 {
			return netip.Prefix{}, fmt.Errorf("IPv4-mapped prefix shorter than /96")
		}
		prefix = netip.PrefixFrom(addr.Unmap(), prefix.Bits()-96)
	}
	return prefix.Masked(), nil
}
