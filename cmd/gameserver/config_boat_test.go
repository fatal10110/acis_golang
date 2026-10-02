package main

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
)

// AllowBoat (server.properties) turns the scheduled boats on; it defaults to
// on when the key is absent.
func TestGameServerConfigAllowBoat(t *testing.T) {
	hexProps, err := config.ParseString("ServerID = 3\nHexID = -7fff\n")
	if err != nil {
		t.Fatalf("ParseString hexid: %v", err)
	}
	for _, tc := range []struct {
		props string
		want  bool
	}{
		{"", true},
		{"AllowBoat = True\n", true},
		{"AllowBoat = False\n", false},
	} {
		serverProps, err := config.ParseString(tc.props)
		if err != nil {
			t.Fatalf("ParseString server %q: %v", tc.props, err)
		}
		cfg, err := gameServerConfigFromProperties(gameServerPaths{}, serverProps, hexProps)
		if err != nil {
			t.Fatalf("gameServerConfigFromProperties(%q): %v", tc.props, err)
		}
		if cfg.AllowBoat != tc.want {
			t.Errorf("AllowBoat for %q = %v, want %v", tc.props, cfg.AllowBoat, tc.want)
		}
	}
}
