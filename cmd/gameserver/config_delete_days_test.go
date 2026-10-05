package main

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/config"
)

// DeleteCharAfterDays (server.properties) is the grace period a deleted
// character waits before it is purged, in days; it defaults to 7 when the key
// is absent, and 0 purges at once.
func TestGameServerConfigDeleteCharAfterDays(t *testing.T) {
	hexProps, err := config.ParseString("ServerID = 3\nHexID = -7fff\n")
	if err != nil {
		t.Fatalf("ParseString hexid: %v", err)
	}
	for _, tc := range []struct {
		props string
		want  time.Duration
	}{
		{"", 7 * 24 * time.Hour},
		{"DeleteCharAfterDays = 3\n", 3 * 24 * time.Hour},
		{"DeleteCharAfterDays = 0\n", 0},
	} {
		serverProps, err := config.ParseString(tc.props)
		if err != nil {
			t.Fatalf("ParseString server %q: %v", tc.props, err)
		}
		cfg, err := gameServerConfigFromProperties(gameServerPaths{}, serverProps, hexProps)
		if err != nil {
			t.Fatalf("gameServerConfigFromProperties(%q): %v", tc.props, err)
		}
		if cfg.CharacterDeleteAfter != tc.want {
			t.Errorf("CharacterDeleteAfter for %q = %v, want %v", tc.props, cfg.CharacterDeleteAfter, tc.want)
		}
	}
}
