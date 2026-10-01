package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/bbs"
)

// TestLoadBoardConfig pins the community board keys of server.properties
// and their shipped defaults: EnableCommunityBoard (False), BBSDefault
// (_bbshome) and ShowServerNews (False).
func TestLoadBoardConfig(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bbs.Config
		news bool
	}{
		{"", bbs.Config{Home: "_bbshome"}, false},
		{"EnableCommunityBoard = True\nBBSDefault = _bbsmail\nShowServerNews = True\n", bbs.Config{Enabled: true, Home: "_bbsmail"}, true},
	} {
		path := filepath.Join(t.TempDir(), "server.properties")
		if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
			t.Fatal(err)
		}
		got, news, err := loadBoardConfig(gameServerPaths{ConfigPath: path})
		if err != nil {
			t.Fatalf("loadBoardConfig(%q) error = %v", tc.body, err)
		}
		if got != tc.want || news != tc.news {
			t.Fatalf("loadBoardConfig(%q) = %+v, news %v; want %+v, news %v", tc.body, got, news, tc.want, tc.news)
		}
	}
}
