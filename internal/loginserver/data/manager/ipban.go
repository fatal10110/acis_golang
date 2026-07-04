// Package manager holds login server data managers that track mutable,
// in-memory state loaded from config at boot.
package manager

import (
	"bufio"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// IPBanList tracks banned addresses and, for temporary bans, when the ban
// expires. The zero value is not usable; construct with NewIPBanList or
// LoadIPBanList.
//
// mu guards bans.
type IPBanList struct {
	mu   sync.Mutex
	bans map[string]time.Time // key: addr.String(); zero Time means the ban never expires

	log *logrus.Logger
}

// NewIPBanList returns an empty IPBanList.
func NewIPBanList(log *logrus.Logger) *IPBanList {
	if log == nil {
		log = logrus.StandardLogger()
	}
	return &IPBanList{bans: make(map[string]time.Time), log: log}
}

// LoadIPBanList reads path, one address per line, and returns an IPBanList
// with a permanent ban for each address resolved from a non-blank line.
// Lines containing '#' are skipped entirely, matching the source file's
// simple comment convention. A line whose address can't be resolved is
// logged and skipped rather than failing the load.
//
// A missing or unreadable file yields an empty list rather than an error:
// IP ban listing is optional infrastructure that must not block boot.
func LoadIPBanList(path string, log *logrus.Logger) *IPBanList {
	l := NewIPBanList(log)

	f, err := os.Open(path)
	if err != nil {
		l.log.Warnf("%s is missing or unreadable, ip ban listing is skipped", path)
		return l
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "#") {
			continue
		}

		addr, err := resolveAddress(line)
		if err != nil {
			l.log.Errorf("invalid ban address (%s)", line)
			continue
		}
		l.set(addr, time.Time{})
	}

	l.log.Infof("loaded %d banned ip(s)", len(l.bans))
	return l
}

// Ban adds addr to the list for d, or permanently if d <= 0. An address
// that is already banned keeps its existing expiry.
func (l *IPBanList) Ban(addr net.IP, d time.Duration) {
	until := time.Time{}
	if d > 0 {
		until = time.Now().Add(d)
	}
	l.set(addr, until)
}

// IsBanned reports whether addr is currently banned, removing the ban first
// if its expiry has passed. A nil addr is treated as banned.
func (l *IPBanList) IsBanned(addr net.IP) bool {
	if addr == nil {
		return true
	}
	key := addr.String()

	l.mu.Lock()
	defer l.mu.Unlock()

	until, banned := l.bans[key]
	if !banned {
		return false
	}
	if !until.IsZero() && time.Now().After(until) {
		delete(l.bans, key)
		l.log.Infof("removed expired ip address ban %s", key)
		return false
	}
	return true
}

func (l *IPBanList) set(addr net.IP, until time.Time) {
	key := addr.String()

	l.mu.Lock()
	defer l.mu.Unlock()

	if _, exists := l.bans[key]; !exists {
		l.bans[key] = until
	}
}

// resolveAddress parses host as a literal IP, falling back to a DNS lookup
// for a hostname, and returns the first resulting address.
func resolveAddress(host string) (net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return ip, nil
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	return ips[0], nil
}
