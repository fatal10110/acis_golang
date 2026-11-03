package admin

import (
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// rateQuest is the quest the rate tests reward through.
const rateQuest = "Q001_LettersOfLove"

// rateScripts registers rateQuest.
func rateScripts() gameservertest.Option {
	return gameservertest.WithScripts([]script.Listing{{Path: "quest." + rateQuest}},
		script.Catalog{"quest." + rateQuest: func() script.Script { return script.Script{QuestID: 1} }})
}

// rewardGains rewards objID, srv.Client's character, through rateQuest
// with 100 adena, 1 exp and 10 sp, and returns the adena, exp and sp it
// gained, reading what the client was sent.
func rewardGains(t *testing.T, srv *gameservertest.Server, objID int32) (adena int, exp int64, sp int) {
	t.Helper()
	progression := func() player.Progression {
		var out player.Progression
		srv.RunQuest(t, objID, rateQuest, func(_ *script.Quests, c *player.Character, _ *script.Script) { out = c.ProgressionValues() })
		return out
	}
	held, before := srv.PlayerItemCount(t, objID, item.AdenaID), progression()
	if !srv.RunScript(t, objID, rateQuest, func(sc *script.Script, p *script.Player) {
		sc.RewardItems(p, item.AdenaID, 100)
		sc.RewardExpAndSp(p, 1, 10)
	}) {
		t.Fatal("reward invocation panicked")
	}
	after := progression()
	drain(t, srv.Client)
	return srv.PlayerItemCount(t, objID, item.AdenaID) - held, after.Exp - before.Exp, int(after.SP - before.SP)
}

// TestAdminReloadConfigQuestRatesTakeReloadedValues: a quest reward after
// //reload config pays by the reloaded RateQuestRewardAdena,
// RateQuestRewardXP and RateQuestRewardSP, as the reference reads them on
// each reward. At the boot rates of 1 the reward pays 100 adena, 1 exp and
// 10 sp; at 3, 2 and 4 it pays 300, 2 and 40.
func TestAdminReloadConfigQuestRatesTakeReloadedValues(t *testing.T) {
	t.Parallel()
	reloaded := liveSettings()
	reloaded.QuestRates = &script.Rates{Drop: 1, Reward: 1, RewardAdena: 3, XP: 2, SP: 4}
	srv, gmID := bootAdmin(t, adminLevel, rateScripts(), gameservertest.WithDataReloads(fixedConfig(reloaded)))
	gm := srv.Client
	enterWorld(t, gm)
	drain(t, gm)

	if adena, exp, sp := rewardGains(t, srv, gmID); adena != 100 || exp != 1 || sp != 10 {
		t.Fatalf("boot reward paid %d adena, %d exp, %d sp; want 100, 1, 10", adena, exp, sp)
	}
	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload config")), configReloaded)
	if adena, exp, sp := rewardGains(t, srv, gmID); adena != 300 || exp != 2 || sp != 40 {
		t.Fatalf("reloaded reward paid %d adena, %d exp, %d sp; want 300, 2, 40", adena, exp, sp)
	}
}

// TestAdminReloadConfigQuestRatesRaceFree: rewards while another game
// master's //reload config swaps the quest rates back and forth each pay by
// one set whole per helper call, never its exp from one and its sp from the
// other.
func TestAdminReloadConfigQuestRatesRaceFree(t *testing.T) {
	t.Parallel()
	off, on := liveSettings(), liveSettings()
	off.QuestRates = &script.Rates{Drop: 1, Reward: 1, RewardAdena: 1, XP: 1, SP: 1}
	on.QuestRates = &script.Rates{Drop: 1, Reward: 1, RewardAdena: 2, XP: 3, SP: 2}
	var calls atomic.Int32
	srv, gmID := bootAdmin(t, adminLevel, rateScripts(), flipReloads(&calls, off, on))
	enterWorld(t, srv.Client)
	drain(t, srv.Client)
	gm2, _ := addPlayer(t, srv, "gm2", "Reloader", adminLevel)

	const rounds = 20
	sent := reloadsInBackground(gm2, rounds)
	for i := range 6 {
		// The item reward and the exp reward are two helper calls, each
		// reading the rates once, so only each call's own rates must agree.
		adena, exp, sp := rewardGains(t, srv, gmID)
		if (exp != 1 || sp != 10) && (exp != 3 || sp != 20) || adena != 100 && adena != 200 {
			t.Fatalf("reward %d paid %d adena, %d exp, %d sp; want 100 or 200 adena and 1, 10 or 3, 20", i, adena, exp, sp)
		}
	}
	awaitReloads(t, srv, &calls, rounds, sent)
}
