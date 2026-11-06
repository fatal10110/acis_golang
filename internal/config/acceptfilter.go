package config

import (
	"fmt"
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
	if ms, ok, err := nonNegativeInt(p, "AcceptFilterBurstTime"); err != nil {
		return netutil.AcceptFilterConfig{}, err
	} else if ok {
		cfg.BurstWindow = time.Duration(ms) * time.Millisecond
	}
	if limit, ok, err := nonNegativeInt(p, "AcceptFilterBurstLimit"); err != nil {
		return netutil.AcceptFilterConfig{}, err
	} else if ok {
		cfg.BurstLimit = limit
	}
	if s, ok, err := nonNegativeInt(p, "AcceptFilterBanTime"); err != nil {
		return netutil.AcceptFilterConfig{}, err
	} else if ok {
		cfg.BanTime = time.Duration(s) * time.Second
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
