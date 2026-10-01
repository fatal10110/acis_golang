package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/rs/zerolog"
)

// Reference: Config.java loads MembersCanWithdrawFromClanWH from
// clans.properties, false when the key is missing.
func TestLoadClanConfigWarehouseWithdrawalRight(t *testing.T) {
	for _, tc := range []struct {
		props string
		want  clan.Config
	}{
		{"", clanConfigDefaults(5, false)},
		{"DaysBeforeJoinAClan = 2\nMembersCanWithdrawFromClanWH = True\n", clanConfigDefaults(2, true)},
		{"MembersCanWithdrawFromClanWH = False\n", clanConfigDefaults(5, false)},
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, "clans.properties")
		if err := os.WriteFile(path, []byte(tc.props), 0o600); err != nil {
			t.Fatal(err)
		}
		players := filepath.Join(dir, "players.properties")
		if err := os.WriteFile(players, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadClanConfig(gameServerPaths{ClansConfigPath: path, PlayersConfigPath: players}, zerolog.Nop())
		if err != nil {
			t.Fatalf("loadClanConfig(%q) error = %v", tc.props, err)
		}
		if got != tc.want {
			t.Fatalf("loadClanConfig(%q) = %+v, want %+v", tc.props, got, tc.want)
		}
	}
}

// clanConfigDefaults is loadClanConfig's result for a clans.properties that
// sets only the join days and the warehouse withdrawal right.
func clanConfigDefaults(joinDays int, withdraw bool) clan.Config {
	return clan.Config{
		JoinDays:                        joinDays,
		CreateDays:                      10,
		MembersForWar:                   15,
		WarPenaltyDays:                  5,
		LifeCrystalNeeded:               true,
		MembersCanWithdrawFromWarehouse: withdraw,
	}
}
