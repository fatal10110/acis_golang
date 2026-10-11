package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// TestLoadBossWaitTimesUsesNpcsProperties pins the npcs.properties
// AntharasWaitTime (default 30), ValakasWaitTime (default 20) and
// FrintezzaWaitTime (default 10), read in minutes and held as a 32-bit
// count of milliseconds.
func TestLoadBossWaitTimesUsesNpcsProperties(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want script.BossWaitTimes
	}{
		{name: "defaults", body: "", want: script.BossWaitTimes{Antharas: 30 * time.Minute, Valakas: 20 * time.Minute, Frintezza: 10 * time.Minute}},
		{name: "shipped", body: "AntharasWaitTime = 30\nValakasWaitTime = 20\nFrintezzaWaitTime = 10\n", want: script.BossWaitTimes{Antharas: 30 * time.Minute, Valakas: 20 * time.Minute, Frintezza: 10 * time.Minute}},
		{name: "set", body: "AntharasWaitTime = 1\nValakasWaitTime = 0\nFrintezzaWaitTime = 45\n", want: script.BossWaitTimes{Antharas: time.Minute, Frintezza: 45 * time.Minute}},
		// 40000 minutes are 2,400,000,000 ms, past the 32-bit range: the
		// count wraps to -1,894,967,296 ms.
		{name: "wraps", body: "AntharasWaitTime = 40000\n", want: script.BossWaitTimes{Antharas: -1894967296 * time.Millisecond, Valakas: 20 * time.Minute, Frintezza: 10 * time.Minute}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "npcs.properties")
			if err := os.WriteFile(configPath, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := loadBossWaitTimes(gameServerPaths{NpcsConfigPath: configPath})
			if err != nil {
				t.Fatalf("loadBossWaitTimes() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("loadBossWaitTimes() = %+v, want %+v", got, tc.want)
			}
		})
	}
	for _, body := range []string{
		"ValakasWaitTime = soon\n",
		// Past the 32-bit range the minute count itself does not parse.
		"AntharasWaitTime = 3000000000\n",
		"FrintezzaWaitTime = -2147483649\n",
	} {
		configPath := filepath.Join(t.TempDir(), "npcs.properties")
		if err := os.WriteFile(configPath, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadBossWaitTimes(gameServerPaths{NpcsConfigPath: configPath}); err == nil {
			t.Fatalf("wait time %q was accepted", strings.TrimSpace(body))
		}
	}
}

// TestScriptEnvCarriesBossWaitTimes: the script helpers read the wait
// times the gameplay config loaded.
func TestScriptEnvCarriesBossWaitTimes(t *testing.T) {
	props, err := config.Parse(strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	waits := script.BossWaitTimes{Antharas: time.Minute, Valakas: 2 * time.Minute, Frintezza: 3 * time.Minute}
	env, err := provideScriptEnv(props, manager.KillRewardConfig{}, nil, nil, nil, nil, &gameData{}, task.NewGameClock(nil), nil, nil, gameplayConfig{BossWaits: waits})
	if err != nil {
		t.Fatalf("provideScriptEnv() error = %v", err)
	}
	if env.BossWaits != waits {
		t.Fatalf("env.BossWaits = %+v, want %+v", env.BossWaits, waits)
	}
}
