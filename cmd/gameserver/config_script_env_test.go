package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TestScriptEnvReadsQuestRates pins the server.properties quest rates into
// the script helpers, each defaulting to 1, beside the kill rewards' party
// range and multiple item drop.
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
		env, err := provideScriptEnv(props, manager.KillRewardConfig{PartyRange: 1500, MultipleItemDrop: true}, nil, nil)
		if err != nil {
			t.Fatalf("provideScriptEnv(%q) error = %v", tc.body, err)
		}
		if env.Rates != tc.want || env.PartyRange != 1500 || !env.MultipleItemDrop {
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
	if _, err := provideScriptEnv(props, manager.KillRewardConfig{}, nil, nil); err == nil {
		t.Fatal("a rate that is no number was accepted")
	}
}
