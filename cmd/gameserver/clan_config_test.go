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
		{"", clan.Config{JoinDays: 5, CreateDays: 10}},
		{"DaysBeforeJoinAClan = 2\nMembersCanWithdrawFromClanWH = True\n", clan.Config{JoinDays: 2, CreateDays: 10, MembersCanWithdrawFromWarehouse: true}},
		{"MembersCanWithdrawFromClanWH = False\n", clan.Config{JoinDays: 5, CreateDays: 10}},
	} {
		path := filepath.Join(t.TempDir(), "clans.properties")
		if err := os.WriteFile(path, []byte(tc.props), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadClanConfig(gameServerPaths{ClansConfigPath: path}, zerolog.Nop())
		if err != nil {
			t.Fatalf("loadClanConfig(%q) error = %v", tc.props, err)
		}
		if got != tc.want {
			t.Fatalf("loadClanConfig(%q) = %+v, want %+v", tc.props, got, tc.want)
		}
	}
}
