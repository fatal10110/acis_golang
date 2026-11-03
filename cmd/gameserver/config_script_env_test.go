package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// TestScriptEnvReadsQuestRates pins the server.properties quest rates into
// the book the script helpers read, each defaulting to 1, beside the kill
// rewards' party range and multiple item drop.
func TestScriptEnvReadsQuestRates(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		body string
		want script.Rates
	}{
		{"", script.Rates{Drop: 1, Reward: 1, RewardAdena: 1, XP: 1, SP: 1}},
		{
			"RateQuestDrop = 1.5\nRateQuestReward = 2\nRateQuestRewardXP = 3\nRateQuestRewardSP = 4\nRateQuestRewardAdena = 0.5\n",
			script.Rates{Drop: 1.5, Reward: 2, RewardAdena: 0.5, XP: 3, SP: 4},
		},
	} {
		server := filepath.Join(dir, "server.properties")
		if err := os.WriteFile(server, []byte(tc.body), 0o600); err != nil {
			t.Fatal(err)
		}
		props, err := config.LoadFile(server)
		if err != nil {
			t.Fatal(err)
		}
		rates, err := provideQuestRates(props)
		if err != nil {
			t.Fatalf("provideQuestRates(%q) error = %v", tc.body, err)
		}
		env := provideScriptEnv(manager.KillRewardConfig{Settings: manager.NewKillRewardBook(manager.KillRewardSettings{PartyRange: 1500, MultipleItemDrop: true})}, nil, nil, nil, nil, &gameData{}, task.NewGameClock(nil), nil, nil, gameplayConfig{}, nil, nil, nil, nil, nil, nil, rates)
		if env.Rates.Rates() != tc.want || env.Drops.PartyRange() != 1500 || !env.Drops.MultipleItemDrop() {
			t.Fatalf("provideScriptEnv(%q) = %+v, want rates %+v", tc.body, env, tc.want)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "server.properties"), []byte("RateQuestDrop = fast\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	props, err := config.LoadFile(filepath.Join(dir, "server.properties"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provideQuestRates(props); err == nil {
		t.Fatal("a rate that is no number was accepted")
	}
}
