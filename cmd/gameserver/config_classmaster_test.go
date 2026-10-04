package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/classmaster"
)

// TestLoadClassMasterConfigUsesNpcsProperties pins the npcs.properties
// AllowEntireTree (default False) and ConfigClassMaster keys: an absent
// line offers no occupation change, and one that does not parse fails
// boot.
func TestLoadClassMasterConfigUsesNpcsProperties(t *testing.T) {
	load := func(t *testing.T, body string) (classmaster.Config, error) {
		t.Helper()
		configPath := filepath.Join(t.TempDir(), "npcs.properties")
		if err := os.WriteFile(configPath, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return loadClassMasterConfig(gameServerPaths{NpcsConfigPath: configPath})
	}
	t.Run("absent", func(t *testing.T) {
		got, err := load(t, "")
		if err != nil || got.AllowEntireTree || got.Allowed(1) || got.Allowed(2) || got.Allowed(3) {
			t.Fatalf("loadClassMasterConfig() = %+v, %v; want nothing offered", got, err)
		}
	})
	t.Run("shipped", func(t *testing.T) {
		got, err := load(t, "AllowEntireTree = False\nConfigClassMaster = 1;[];[];2;[];[];3;[];[]\n")
		if err != nil || got.AllowEntireTree || !got.Allowed(1) || !got.Allowed(2) || !got.Allowed(3) {
			t.Fatalf("loadClassMasterConfig() = %+v, %v; want tiers 1-3 offered free", got, err)
		}
	})
	t.Run("priced entire tree", func(t *testing.T) {
		got, err := load(t, "AllowEntireTree = True\nConfigClassMaster = 2;[57(1000000)];[6622(1)]\n")
		if err != nil || !got.AllowEntireTree || got.Allowed(1) || !got.Allowed(2) {
			t.Fatalf("loadClassMasterConfig() = %+v, %v; want tier 2 under the entire tree", got, err)
		}
		job, _ := got.Job(2)
		if len(job.Required) != 1 || job.Required[0] != (classmaster.Item{ID: 57, Count: 1000000}) ||
			len(job.Reward) != 1 || job.Reward[0] != (classmaster.Item{ID: 6622, Count: 1}) {
			t.Fatalf("Job(2) = %+v, want 1000000 adena for one 6622", job)
		}
	})
	t.Run("malformed", func(t *testing.T) {
		if _, err := load(t, "ConfigClassMaster = 1;[57];[]\n"); err == nil {
			t.Fatal("loadClassMasterConfig() error = nil, want a parse error")
		}
	})
}
