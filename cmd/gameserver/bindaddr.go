package main

import (
	"context"
	"errors"
	"net"

	"github.com/rs/zerolog"
)

// gameBindAddress returns the address the game client listener binds for
// addr, a host:port from listenAddress. A host name resolves to one of its
// addresses, IPv4 first. A name that does not resolve logs and falls back
// to every address, so a bad name never stops the boot.
func gameBindAddress(ctx context.Context, addr string, lookup func(context.Context, string) ([]net.IPAddr, error), log zerolog.Logger) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || host == "" || net.ParseIP(host) != nil {
		return addr
	}
	ips, err := lookup(ctx, host)
	if err == nil && len(ips) == 0 {
		err = errors.New("no address")
	}
	if err != nil {
		log.Error().Err(err).Str("host", host).Msg("The GameServer bind address is invalid, using all available IPs.")
		return net.JoinHostPort("", port)
	}
	chosen := ips[0]
	for _, ip := range ips {
		if ip.IP.To4() != nil {
			chosen = ip
			break
		}
	}
	return net.JoinHostPort(chosen.String(), port)
}
