package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

// TestLoadLogItemsUsesServerProperties pins the item log switch:
// server.properties turns it on, and a file without the key keeps the
// shipped default of logging nothing.
func TestLoadLogItemsUsesServerProperties(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bool
	}{
		{"LogItems = True\n", true},
		{"LogItems = False\n", false},
		{"LogChat = True\n", false},
	} {
		configPath := filepath.Join(t.TempDir(), "server.properties")
		if err := os.WriteFile(configPath, []byte(tc.body), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadLogItems(gameServerPaths{ConfigPath: configPath})
		if err != nil {
			t.Fatalf("loadLogItems(%q) error = %v", tc.body, err)
		}
		if bool(got) != tc.want {
			t.Fatalf("loadLogItems(%q) = %v, want %v", tc.body, got, tc.want)
		}
	}
}

// TestItemLoggerWritesOnlyWhenEnabled pins that LogItems = False leaves the
// item sink unwritten, and True hands it through.
func TestItemLoggerWritesOnlyWhenEnabled(t *testing.T) {
	var out bytes.Buffer
	sink := itemLogger(zerolog.New(&out))

	off := sink.enabled(false)
	off.Info().Msg("ADDED")
	if out.Len() != 0 {
		t.Fatalf("item log with LogItems off = %q, want nothing", out.String())
	}

	on := sink.enabled(true)
	on.Info().Msg("ADDED")
	if out.Len() == 0 {
		t.Fatal("item log with LogItems on wrote nothing")
	}
}
