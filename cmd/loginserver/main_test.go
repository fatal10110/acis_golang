package main

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/config"
)

func TestLoginServerConfigFromProperties(t *testing.T) {
	props, err := config.ParseString(`
LoginserverHostname = 127.0.0.1
LoginserverPort = 12106
LoginHostname = 127.0.0.2
LoginPort = 19014
AcceptNewGameServer = True
AutoCreateAccounts = False
LoginTryBeforeBan = 5
LoginBlockAfterBan = 42
URL = jdbc:mariadb://db.example/acis
Login = acis
Password = secret
`)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}

	cfg, err := loginServerConfigFromProperties(loginServerPaths{}, props)
	if err != nil {
		t.Fatalf("loginServerConfigFromProperties: %v", err)
	}

	if cfg.ClientAddr != "127.0.0.1:12106" {
		t.Errorf("ClientAddr = %q, want 127.0.0.1:12106", cfg.ClientAddr)
	}
	if cfg.GameServerAddr != "127.0.0.2:19014" {
		t.Errorf("GameServerAddr = %q, want 127.0.0.2:19014", cfg.GameServerAddr)
	}
	if !cfg.AllowNewGameServers {
		t.Error("AllowNewGameServers = false, want true")
	}
	if cfg.AutoCreateAccounts {
		t.Error("AutoCreateAccounts = true, want explicit false")
	}
	if cfg.LoginTryBeforeBan != 5 {
		t.Errorf("LoginTryBeforeBan = %d, want 5", cfg.LoginTryBeforeBan)
	}
	if cfg.LoginBlockAfterBan != 42*time.Second {
		t.Errorf("LoginBlockAfterBan = %s, want 42s", cfg.LoginBlockAfterBan)
	}
	if cfg.Database.URL != "jdbc:mariadb://db.example/acis" || cfg.Database.Login != "acis" || cfg.Database.Password != "secret" {
		t.Errorf("Database = %+v, want parsed database credentials", cfg.Database)
	}
}

func TestLoginServerConfigDefaultsAutoCreateAccounts(t *testing.T) {
	props, err := config.ParseString(``)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}

	cfg, err := loginServerConfigFromProperties(loginServerPaths{}, props)
	if err != nil {
		t.Fatalf("loginServerConfigFromProperties: %v", err)
	}

	if !cfg.AutoCreateAccounts {
		t.Error("AutoCreateAccounts = false, want default true")
	}
	if cfg.LoginTryBeforeBan != 3 {
		t.Errorf("LoginTryBeforeBan = %d, want default 3", cfg.LoginTryBeforeBan)
	}
	if cfg.LoginBlockAfterBan != 10*time.Minute {
		t.Errorf("LoginBlockAfterBan = %s, want default 10m", cfg.LoginBlockAfterBan)
	}
}

func TestListenAddressWildcard(t *testing.T) {
	got := listenAddress("*", 2106)
	if got != ":2106" {
		t.Fatalf("listenAddress wildcard = %q, want :2106", got)
	}
}
