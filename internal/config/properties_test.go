package config_test

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
)

func TestUnknownKeysInSurfacesKeyWithNoReader(t *testing.T) {
	props, err := config.ParseString("GameserverPort=7777\nTotallyUnreadKey=1\n")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}

	got := config.UnknownKeysIn("server.properties", props)

	want := []config.KeyRef{{File: "server.properties", Key: "TotallyUnreadKey"}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("UnknownKeysIn = %v, want %v", got, want)
	}
}

func TestUnknownKeysInEmptyForFullyReadFile(t *testing.T) {
	props, err := config.ParseString("GameserverPort=7777\n")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}

	if got := config.UnknownKeysIn("server.properties", props); len(got) != 0 {
		t.Fatalf("UnknownKeysIn = %v, want none", got)
	}
}
