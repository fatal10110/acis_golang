package main

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
)

func TestGameServerConfigFromProperties(t *testing.T) {
	serverProps, err := config.ParseString(`
Hostname = games.example.com
GameserverHostname = 127.0.0.3
GameserverPort = 17777
LoginHost = 127.0.0.4
LoginPort = 19014
RequestServerID = 7
AcceptAlternateID = False
MaximumOnlineUsers = 123
URL = jdbc:mariadb://db.example/acis
Login = acis
Password = secret
`)
	if err != nil {
		t.Fatalf("ParseString server: %v", err)
	}
	hexProps, err := config.ParseString(`
ServerID = 3
HexID = -7fff
`)
	if err != nil {
		t.Fatalf("ParseString hexid: %v", err)
	}

	cfg, err := gameServerConfigFromProperties(gameServerPaths{}, serverProps, hexProps)
	if err != nil {
		t.Fatalf("gameServerConfigFromProperties: %v", err)
	}

	if cfg.ListenAddr != "127.0.0.3:17777" {
		t.Errorf("ListenAddr = %q, want 127.0.0.3:17777", cfg.ListenAddr)
	}
	if cfg.LoginAddr != "127.0.0.4:19014" {
		t.Errorf("LoginAddr = %q, want 127.0.0.4:19014", cfg.LoginAddr)
	}
	if cfg.Auth.ServerID != 3 {
		t.Errorf("Auth.ServerID = %d, want hexid ServerID 3", cfg.Auth.ServerID)
	}
	if cfg.Auth.AcceptAlternateID {
		t.Error("Auth.AcceptAlternateID = true, want false")
	}
	if cfg.Auth.HostName != "games.example.com" || cfg.Auth.Port != 17777 || cfg.Auth.MaxPlayers != 123 {
		t.Errorf("Auth advertised endpoint/capacity = %+v, want host games.example.com port 17777 max 123", cfg.Auth)
	}
	wantHex, err := model.ParseHexKey("-7fff")
	if err != nil {
		t.Fatalf("ParseHexKey: %v", err)
	}
	if !bytes.Equal(cfg.Auth.HexID, wantHex) {
		t.Errorf("Auth.HexID = %x, want %x", cfg.Auth.HexID, wantHex)
	}
	if cfg.Database.URL != "jdbc:mariadb://db.example/acis" || cfg.Database.Login != "acis" || cfg.Database.Password != "secret" {
		t.Errorf("Database = %+v, want parsed database credentials", cfg.Database)
	}
}

func TestGameServerConfigUsesRequestIDWithoutHexID(t *testing.T) {
	serverProps, err := config.ParseString(`
GameserverHostname = *
GameserverPort = 7777
LoginHost = 127.0.0.1
LoginPort = 9014
RequestServerID = 9
`)
	if err != nil {
		t.Fatalf("ParseString server: %v", err)
	}

	cfg, err := gameServerConfigFromProperties(gameServerPaths{}, serverProps, nil)
	if err != nil {
		t.Fatalf("gameServerConfigFromProperties: %v", err)
	}

	if cfg.ListenAddr != ":7777" {
		t.Errorf("ListenAddr = %q, want :7777", cfg.ListenAddr)
	}
	if cfg.Auth.ServerID != 9 {
		t.Errorf("Auth.ServerID = %d, want RequestServerID 9", cfg.Auth.ServerID)
	}
	if len(cfg.Auth.HexID) != generatedHexIDSize {
		t.Errorf("generated HexID length = %d, want %d", len(cfg.Auth.HexID), generatedHexIDSize)
	}
}
