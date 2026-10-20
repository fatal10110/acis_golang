package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSingleKeyLoadersFailOnBadNumber pins that a malformed number fails the
// load naming its key instead of falling back to the default, and that an
// int key rejects a value outside int32 the same way.
func TestSingleKeyLoadersFailOnBadNumber(t *testing.T) {
	type loader func(gameServerPaths) error
	players := func(p string) gameServerPaths { return gameServerPaths{PlayersConfigPath: p} }
	server := func(p string) gameServerPaths { return gameServerPaths{ConfigPath: p} }
	npcs := func(p string) gameServerPaths { return gameServerPaths{NpcsConfigPath: p} }
	for _, tc := range []struct {
		key   string
		isInt bool
		paths func(string) gameServerPaths
		load  loader
	}{
		{"PlayerSpawnProtection", true, players, func(p gameServerPaths) error { _, err := loadPlayerSpawnProtection(p); return err }},
		{"RespawnRestoreHP", false, players, func(p gameServerPaths) error { _, err := loadRespawnRestoreHP(p); return err }},
		{"DeathPenaltyChance", true, players, func(p gameServerPaths) error { _, err := loadDeathPenaltyChance(p); return err }},
		{"MaxBuffsAmount", true, players, func(p gameServerPaths) error { _, err := loadMaxBuffsAmount(p); return err }},
		{"PerfectShieldBlockRate", true, players, func(p gameServerPaths) error { _, err := loadPerfectShieldBlockRate(p); return err }},
		{"WeightLimit", false, players, func(p gameServerPaths) error { _, err := loadWeightLimitMultiplier(p); return err }},
		{"RateKarmaExpLost", false, server, func(p gameServerPaths) error { _, err := loadRateKarmaExpLost(p); return err }},
		{"CharacterSelectTime", true, server, func(p gameServerPaths) error { _, err := loadCharacterSelectDelay(p); return err }},
		{"ServerBypassTime", true, server, func(p gameServerPaths) error { _, err := loadServerBypassDelay(p); return err }},
		{"ManufactureTime", true, server, func(p gameServerPaths) error { _, err := loadManufactureDelay(p); return err }},
		{"MultisellTime", true, server, func(p gameServerPaths) error { _, err := loadMultisellDelay(p); return err }},
		{"ManorTime", true, server, func(p gameServerPaths) error { _, err := loadManorDelay(p); return err }},
		{"RollDiceTime", true, server, func(p gameServerPaths) error { _, err := loadRollDiceDelay(p); return err }},
		{"SocialTime", true, server, func(p gameServerPaths) error { _, err := loadSocialDelay(p); return err }},
		{"SubclassTime", true, server, func(p gameServerPaths) error { _, err := loadSubclassDelay(p); return err }},
		{"SpawnMultiplier", false, npcs, func(p gameServerPaths) error { _, err := loadSpawnMultiplier(p); return err }},
		{"RandomWalkRate", true, npcs, func(p gameServerPaths) error { _, err := loadRandomWalkRate(p); return err }},
	} {
		t.Run(tc.key, func(t *testing.T) {
			bad := []string{"soon"}
			if tc.isInt {
				bad = append(bad, "2147483648", "-2147483649")
			}
			for _, value := range bad {
				path := filepath.Join(t.TempDir(), "bad.properties")
				if err := os.WriteFile(path, []byte(tc.key+" = "+value+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				err := tc.load(tc.paths(path))
				if err == nil || !strings.Contains(err.Error(), tc.key) {
					t.Errorf("%s = %s: error %v, want one naming %s", tc.key, value, err, tc.key)
				}
			}
			// The int32 bounds themselves still load.
			if tc.isInt {
				for _, value := range []string{"2147483647", "-2147483648"} {
					path := filepath.Join(t.TempDir(), "edge.properties")
					if err := os.WriteFile(path, []byte(tc.key+" = "+value+"\n"), 0o600); err != nil {
						t.Fatal(err)
					}
					if err := tc.load(tc.paths(path)); err != nil {
						t.Errorf("%s = %s: error %v, want none", tc.key, value, err)
					}
				}
			}
		})
	}
}
