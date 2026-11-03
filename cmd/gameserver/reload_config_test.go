package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/rs/zerolog"
)

// TestDataReloadsReadConfig requires the config reload hook to read the
// server, players, NPC and events properties anew into the player
// settings, the kill reward, event, quest rate, PvP flag time, NPC limit
// and ShowNpcLevel settings among them, and to fail on a value boot would
// refuse.
func TestDataReloadsReadConfig(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	paths := gameServerPaths{
		ConfigPath:        write("server.properties", "AllowDiscardItem = True\n"),
		HexIDPath:         write("hexid.txt", "ServerID = 3\nHexID = -7fff\n"),
		PlayersConfigPath: write("players.properties", "KarmaPlayerCanTrade = True\n"),
		NpcsConfigPath:    write("npcs.properties", ""),
		GeoConfigPath:     write("geoengine.properties", ""),
		EventsConfigPath:  write("events.properties", ""),
	}
	reloads := provideDataReloads(paths, gameServerConfig{}, &gameData{}, nil, nil, nil, nil, gameplayConfig{}, zerolog.Nop())

	settings, err := reloads.Config()
	if err != nil {
		t.Fatalf("config reload: %v", err)
	}
	if e := settings.Events; e == nil || *e != script.DefaultEventRules() {
		t.Fatalf("reloaded event settings = %+v; want the shipped ones", e)
	}
	if settings.DiscardItemDisabled || !settings.KarmaPlayerCanTrade {
		t.Fatalf("reloaded settings = discard disabled %v, karma trade %v; want false, true",
			settings.DiscardItemDisabled, settings.KarmaPlayerCanTrade)
	}
	if r := settings.QuestRates; r == nil || *r != script.DefaultRates() {
		t.Fatalf("reloaded quest rates = %+v; want the shipped ones", r)
	}
	if p := settings.PvPFlagTimes; p == nil || *p != (task.PvPFlagTimes{Normal: 40 * time.Second, Flagged: 20 * time.Second}) {
		t.Fatalf("reloaded PvP flag times = %+v; want 40s and 20s", p)
	}
	if l := settings.NPCLimits; l == nil || *l != (npc.Limits{MaxBuffs: 20, DriftRange: 200}) || settings.ShowNPCLevel {
		t.Fatalf("reloaded NPC limits = %+v, show level %v; want 20 buffs, drift 200, no level", l, settings.ShowNPCLevel)
	}

	write("server.properties", "AllowDiscardItem = False\nServerBypassTime = 2500\nMultipleItemDrop = False\nRateXp = 3\nRateQuestDrop = 2\nRateQuestRewardAdena = 3\n")
	write("players.properties", "KarmaPlayerCanTrade = False\nPartyRange = 900\nPvPVsNormalTime = 5000\nPvPVsPvPTime = 7000\nMaxBuffsAmount = 24\n")
	write("npcs.properties", "MaxDriftRange = 350\nShowNpcLevel = True\n")
	write("events.properties", "NeededPartyMembers = 7\nOfficerCost = 5\n")
	settings, err = reloads.Config()
	if err != nil {
		t.Fatalf("second config reload: %v", err)
	}
	if !settings.DiscardItemDisabled || settings.KarmaPlayerCanTrade || settings.ServerBypassDelay != 2500*time.Millisecond {
		t.Fatalf("reloaded settings = discard disabled %v, karma trade %v, bypass delay %s; want the edited files' values",
			settings.DiscardItemDisabled, settings.KarmaPlayerCanTrade, settings.ServerBypassDelay)
	}
	if r := settings.KillRewards; r == nil || r.PartyRange != 900 || r.MultipleItemDrop || r.RateXP != 3 {
		t.Fatalf("reloaded kill rewards = %+v; want party range 900, single item drop, exp rate 3", r)
	}

	if r := settings.QuestRates; r == nil || *r != (script.Rates{Drop: 2, Reward: 1, RewardAdena: 3, XP: 1, SP: 1}) {
		t.Fatalf("reloaded quest rates = %+v; want drop 2, adena 3, the rest 1", r)
	}
	if p := settings.PvPFlagTimes; p == nil || *p != (task.PvPFlagTimes{Normal: 5 * time.Second, Flagged: 7 * time.Second}) {
		t.Fatalf("reloaded PvP flag times = %+v; want 5s and 7s", p)
	}
	if l := settings.NPCLimits; l == nil || *l != (npc.Limits{MaxBuffs: 24, DriftRange: 350}) || !settings.ShowNPCLevel || settings.MaxBuffsAmount != 24 {
		t.Fatalf("reloaded NPC limits = %+v, show level %v, player buffs %d; want 24 buffs, drift 350, level shown", l, settings.ShowNPCLevel, settings.MaxBuffsAmount)
	}
	if e := settings.Events; e == nil || e.FourGobletsParty != 7 || e.Rift.Costs[2] != 5 {
		t.Fatalf("reloaded event settings = %+v; want party 7, officer cost 5", e)
	}

	write("events.properties", "RiftMinPartySize = two\n")
	if _, err := reloads.Config(); err == nil {
		t.Fatal("config reload of a malformed events value succeeded")
	}
	write("events.properties", "")
	write("server.properties", "RateQuestReward = lots\n")
	if _, err := reloads.Config(); err == nil {
		t.Fatal("config reload of a malformed quest rate succeeded")
	}
	write("server.properties", "")
	write("players.properties", "PartyRange = far\n")
	if _, err := reloads.Config(); err == nil {
		t.Fatal("config reload of a malformed value succeeded")
	}
}
