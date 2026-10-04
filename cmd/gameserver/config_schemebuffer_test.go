package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
)

// TestLoadSchemeBufferConfigUsesNpcsProperties pins the npcs.properties
// BufferMaxSchemesPerChar (default 4) and BufferStaticCostPerBuff (default
// -1) keys; a malformed value fails boot.
func TestLoadSchemeBufferConfigUsesNpcsProperties(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		want    schemebuffer.Config
		wantErr bool
	}{
		{name: "defaults", body: "", want: schemebuffer.Config{MaxSchemes: 4, StaticCost: -1}},
		{name: "shipped", body: "BufferMaxSchemesPerChar = 4\nBufferStaticCostPerBuff = -1\n", want: schemebuffer.Config{MaxSchemes: 4, StaticCost: -1}},
		{name: "set", body: "BufferMaxSchemesPerChar = 7\nBufferStaticCostPerBuff = 250\n", want: schemebuffer.Config{MaxSchemes: 7, StaticCost: 250}},
		{name: "malformed", body: "BufferMaxSchemesPerChar = four\n", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "npcs.properties")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := loadSchemeBufferConfig(gameServerPaths{NpcsConfigPath: path})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("loadSchemeBufferConfig() = %+v, want an error", got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("loadSchemeBufferConfig() = %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}
