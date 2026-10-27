package admin

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const eventRulesProbe = "EventRulesProbe"

// eventRulesProbeScripts registers the probe feature script the event
// rules tests run.
func eventRulesProbeScripts() gameservertest.Option {
	return gameservertest.WithScripts(
		[]script.Listing{{Path: "script.feature." + eventRulesProbe}},
		script.Catalog{"script.feature." + eventRulesProbe: func() script.Script { return script.Script{QuestID: -1} }})
}

// TestScriptEventRulesFollowConfig: a script reads the events.properties
// event quest settings in force at each call. Booted with a Four Sepulchers
// party of 6 and rift settings off the shipped ones, a party of 5 falls
// short of the entry and the soldier entry costs 7 fragments; after
// //reload config reads a party of 3 and a soldier cost of 30, the same
// party of 5 is enough and the soldier entry costs 30.
func TestScriptEventRulesFollowConfig(t *testing.T) {
	t.Parallel()
	booted := script.EventRules{
		FourGobletsParty: 6,
		Rift: script.RiftRules{
			MinParty: 4, Costs: [6]int{5, 7, 9, 11, 13, 15},
			JumpDelayMin: 3, JumpDelayRnd: 2, AnakazelPortChance: 60,
		},
	}
	reloaded := script.EventRules{
		FourGobletsParty: 3,
		Rift: script.RiftRules{
			MinParty: 5, Costs: [6]int{20, 30, 40, 50, 60, 70},
			JumpDelayMin: 12, JumpDelayRnd: 0, AnakazelPortChance: 1,
		},
	}
	srv, gmID := bootAdmin(t, adminLevel,
		gameservertest.WithEventRules(booted),
		eventRulesProbeScripts(),
		gameservertest.WithDataReloads(fixedConfig(network.PlayerConfig{
			RespawnRestoreHP: 0.7, MaxBuffsAmount: 20, WeightLimitMultiplier: 1,
			Events: &reloaded,
		})))
	gm := srv.Client
	enterWorld(t, gm)
	drain(t, gm)

	const party = 5
	check := func(want script.EventRules, enough bool, soldierCost int) {
		t.Helper()
		var got script.EventRules
		if !srv.RunScript(t, gmID, eventRulesProbe, func(sc *script.Script, _ *script.Player) { got = sc.EventRules() }) {
			t.Fatal("script invocation panicked")
		}
		if got != want {
			t.Fatalf("EventRules() = %+v, want %+v", got, want)
		}
		if ok := party >= got.FourGobletsParty; ok != enough {
			t.Errorf("a party of %d enters the Four Sepulchers: %v, want %v", party, ok, enough)
		}
		if c := got.Rift.Costs[1]; c != soldierCost {
			t.Errorf("soldier rift entry costs %d fragments, want %d", c, soldierCost)
		}
	}
	check(booted, false, 7)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload config")), configReloaded)
	check(reloaded, true, 30)
}

// TestScriptNPCCompareAndExchangeIntIsAtomic: many scripts claiming one
// NPC's AVQuest0 slot from 0 to 1 at once see exactly one claim succeed,
// the rest reading the 1 it stored; the AVQuest1 slot is left as it was,
// and a release from 1 to 0 frees the slot for the next claim.
func TestScriptNPCCompareAndExchangeIntIsAtomic(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	drain(t, gm)

	x, y, z := srv.PlayerPosition(t, gmID)
	n := script.NPCOf(srv.SpawnHostileNPCAt(t, location.Location{X: x + 100, Y: y, Z: z}))
	n.SetInt(script.SlotAVQuest1, 7)

	const claimers = 32
	var (
		wg    sync.WaitGroup
		start = make(chan struct{})
		olds  = make([]int32, claimers)
	)
	for i := range claimers {
		wg.Go(func() {
			<-start
			olds[i] = n.CompareAndExchangeInt(script.SlotAVQuest0, 0, 1)
		})
	}
	close(start)
	wg.Wait()

	won := 0
	for i, old := range olds {
		switch old {
		case 0:
			won++
		case 1:
		default:
			t.Errorf("claim %d read %d before it, want 0 or 1", i, old)
		}
	}
	if won != 1 {
		t.Fatalf("%d claims of a free slot succeeded, want 1", won)
	}
	if got := n.Int(script.SlotAVQuest0); got != 1 {
		t.Fatalf("AVQuest0 after the claims = %d, want 1", got)
	}
	if got := n.Int(script.SlotAVQuest1); got != 7 {
		t.Fatalf("AVQuest1 after the claims = %d, want 7", got)
	}
	if old := n.CompareAndExchangeInt(script.SlotAVQuest0, 0, 1); old != 1 {
		t.Fatalf("a claim of a held slot read %d, want 1", old)
	}
	if old := n.CompareAndExchangeInt(script.SlotAVQuest0, 1, 0); old != 1 {
		t.Fatalf("a release read %d, want 1", old)
	}
	if old := n.CompareAndExchangeInt(script.SlotAVQuest0, 0, 1); old != 0 {
		t.Fatalf("a claim after the release read %d, want 0", old)
	}
}
