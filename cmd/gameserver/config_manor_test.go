package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadManorSettingsUsesServerProperties pins the manor switches:
// AllowManor and RateDropManor default to on and 1, as shipped, and a
// malformed rate fails boot.
func TestLoadManorSettingsUsesServerProperties(t *testing.T) {
	for _, tc := range []struct {
		body    string
		want    manorSettings
		wantErr bool
	}{
		{"", manorSettings{Allowed: true, CropRate: 1}, false},
		{"AllowManor = False\nRateDropManor = 3\n", manorSettings{Allowed: false, CropRate: 3}, false},
		{"RateDropManor = many\n", manorSettings{}, true},
	} {
		configPath := filepath.Join(t.TempDir(), "server.properties")
		if err := os.WriteFile(configPath, []byte(tc.body), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadManorSettings(gameServerPaths{ConfigPath: configPath})
		if (err != nil) != tc.wantErr {
			t.Fatalf("loadManorSettings(%q) error = %v, want error %v", tc.body, err, tc.wantErr)
		}
		if got != tc.want {
			t.Fatalf("loadManorSettings(%q) = %+v, want %+v", tc.body, got, tc.want)
		}
	}
}
