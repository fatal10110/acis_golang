package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/castlemanor"
)

// TestLoadCastleManorConfig pins the manor cycle settings: the
// clans.properties Manor* times default to the shipped 20:00 refresh,
// 06:00 approval, 6 maintenance minutes and a save every 2 hours, the
// switch and crop rate come from server.properties, and a malformed value
// fails boot.
func TestLoadCastleManorConfig(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		body    string
		want    castlemanor.Config
		wantErr bool
	}{
		{"", castlemanor.Config{Enabled: true, CropRate: 2, RefreshHour: 20, ApproveHour: 6, MaintenanceMin: 6, SavePeriod: 2 * time.Hour}, false},
		{
			"ManorRefreshTime = 21\nManorRefreshMin = 30\nManorApproveTime = 7\nManorApproveMin = 15\nManorMaintenanceMin = 10\nManorSavePeriodRate = 3\n",
			castlemanor.Config{Enabled: true, CropRate: 2, RefreshHour: 21, RefreshMin: 30, ApproveHour: 7, ApproveMin: 15, MaintenanceMin: 10, SavePeriod: 3 * time.Hour},
			false,
		},
		{"ManorRefreshTime = late\n", castlemanor.Config{}, true},
	} {
		path := filepath.Join(t.TempDir(), "clans.properties")
		if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadCastleManorConfig(gameServerPaths{ClansConfigPath: path}, manorSettings{Allowed: true, CropRate: 2})
		if (err != nil) != tc.wantErr {
			t.Fatalf("loadCastleManorConfig(%q) error = %v, want error %v", tc.body, err, tc.wantErr)
		}
		if !tc.wantErr && got != tc.want {
			t.Fatalf("loadCastleManorConfig(%q) = %+v, want %+v", tc.body, got, tc.want)
		}
	}
}
