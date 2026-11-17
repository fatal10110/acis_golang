package main

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
)

// TestLoginServerConfigRejectsMaxConnectionsBelowOne: a present
// MaxConnections below 1 fails config loading instead of falling back to
// the default pool size an absent key gets.
func TestLoginServerConfigRejectsMaxConnectionsBelowOne(t *testing.T) {
	for _, value := range []string{"0", "-1"} {
		props, err := config.ParseString("URL = jdbc:mariadb://localhost/acis\nMaxConnections = " + value + "\n")
		if err != nil {
			t.Fatalf("ParseString: %v", err)
		}
		_, err = loginServerConfigFromProperties(loginServerPaths{}, props)
		if err == nil || !strings.Contains(err.Error(), "MaxConnections") {
			t.Errorf("MaxConnections = %s: error = %v, want a MaxConnections error", value, err)
		}
	}
}
