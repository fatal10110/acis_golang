package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/wedding"
)

// TestLoadWeddingConfigUsesNpcsProperties pins the npcs.properties
// WeddingPrice (default 1000000), WeddingAllowSameSex (default false) and
// WeddingFormalWear (default true) keys; a malformed price fails boot.
func TestLoadWeddingConfigUsesNpcsProperties(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		want    wedding.Config
		wantErr bool
	}{
		{name: "defaults", body: "", want: wedding.Config{Price: 1000000, FormalWear: true}},
		{name: "shipped", body: "WeddingPrice = 1000000\nWeddingAllowSameSex = False\nWeddingFormalWear = True\n", want: wedding.Config{Price: 1000000, FormalWear: true}},
		{name: "set", body: "WeddingPrice = 250\nWeddingAllowSameSex = True\nWeddingFormalWear = False\n", want: wedding.Config{Price: 250, SameSex: true}},
		{name: "malformed", body: "WeddingPrice = lots\n", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "npcs.properties")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := loadWeddingConfig(gameServerPaths{NpcsConfigPath: path})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("loadWeddingConfig() = %+v, want an error", got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("loadWeddingConfig() = %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}
