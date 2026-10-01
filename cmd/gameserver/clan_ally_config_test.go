package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

// Reference: Config.java loads the alliance penalties and size from
// clans.properties: DaysBeforeJoinAllyWhenLeaved, ...WhenDismissed and
// DaysBeforeAcceptNewClanWhenDismissed default to 1,
// DaysBeforeCreateNewAllyWhenDissolved to 10, MaxNumOfClansInAlly to 3.
func TestLoadClanConfigAlliance(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clans.properties")
	props := "DaysBeforeJoinAllyWhenLeaved = 2\nDaysBeforeJoinAllyWhenDismissed = 3\n" +
		"DaysBeforeAcceptNewClanWhenDismissed = 4\nDaysBeforeCreateNewAllyWhenDissolved = 5\nMaxNumOfClansInAlly = 6\n"
	if err := os.WriteFile(path, []byte(props), 0o600); err != nil {
		t.Fatal(err)
	}
	players := filepath.Join(dir, "players.properties")
	if err := os.WriteFile(players, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadClanConfig(gameServerPaths{ClansConfigPath: path, PlayersConfigPath: players}, zerolog.Nop())
	if err != nil {
		t.Fatalf("loadClanConfig error = %v", err)
	}
	want := clanConfigDefaults(5, false)
	want.AllyJoinDaysWhenLeft, want.AllyJoinDaysWhenDismissed, want.AcceptClanDaysWhenDismissed = 2, 3, 4
	want.CreateAllyDaysWhenDissolved, want.MaxClansInAlly = 5, 6
	if got != want {
		t.Fatalf("loadClanConfig = %+v, want %+v", got, want)
	}
}
