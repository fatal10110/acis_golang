package main

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
)

// TestGameServerConfigReadsUnicodeDigits pins the hexid ServerID and the
// server.properties integers to the reference's Config, which reads them
// with Integer.parseInt: fullwidth and Arabic-Indic digits read as their
// value (a Java probe on OpenJDK 21.0.11, recorded in #3091, prints
// Integer.parseInt("１２") and Integer.parseInt("١٢") as 12).
func TestGameServerConfigReadsUnicodeDigits(t *testing.T) {
	serverProps, err := config.ParseString(`
URL = jdbc:mariadb://localhost/acis
GameserverPort = ١٧٧٧٧
MaximumOnlineUsers = １２３
`)
	if err != nil {
		t.Fatalf("ParseString server: %v", err)
	}
	hexProps, err := config.ParseString(`
ServerID = ３
HexID = -7fff
`)
	if err != nil {
		t.Fatalf("ParseString hexid: %v", err)
	}
	cfg, err := gameServerConfigFromProperties(gameServerPaths{}, serverProps, hexProps)
	if err != nil {
		t.Fatalf("gameServerConfigFromProperties: %v", err)
	}
	if cfg.Auth.ServerID != 3 {
		t.Errorf("Auth.ServerID = %d, want 3", cfg.Auth.ServerID)
	}
	if cfg.Auth.Port != 17777 {
		t.Errorf("Auth.Port = %d, want 17777", cfg.Auth.Port)
	}
	if cfg.Auth.MaxPlayers != 123 {
		t.Errorf("Auth.MaxPlayers = %d, want 123", cfg.Auth.MaxPlayers)
	}
}
