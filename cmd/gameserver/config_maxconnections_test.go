package main

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
)

// TestGameServerConfigRejectsMaxConnectionsBelowOne: a present
// MaxConnections below 1 fails config loading instead of falling back to
// the default pool size an absent key gets.
func TestGameServerConfigRejectsMaxConnectionsBelowOne(t *testing.T) {
	hexProps, err := config.ParseString("ServerID = 3\nHexID = -7fff\n")
	if err != nil {
		t.Fatalf("ParseString hexid: %v", err)
	}
	for _, value := range []string{"0", "-1"} {
		serverProps, err := config.ParseString("URL = jdbc:mariadb://localhost/acis\nMaxConnections = " + value + "\n")
		if err != nil {
			t.Fatalf("ParseString server: %v", err)
		}
		_, err = gameServerConfigFromProperties(gameServerPaths{}, serverProps, hexProps)
		if err == nil || !strings.Contains(err.Error(), "MaxConnections") {
			t.Errorf("MaxConnections = %s: error = %v, want a MaxConnections error", value, err)
		}
	}
}
