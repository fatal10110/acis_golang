package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const partyRangeProbe = "PartyRangeProbe"

// TestScriptInPartyRangeFollowsConfig: a script compares a creature's
// distance to an NPC against the configured party range, strictly, and reads
// the range in force at each call: booted at 500, an NPC 499 away is within
// and one 1000 away is not; after //reload config reads 1500, the one 1000
// away is within, and one exactly 1500 away is not. A handle on nothing
// aborts the invocation.
func TestScriptInPartyRangeFollowsConfig(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel,
		gameservertest.WithPartyRange(500),
		gameservertest.WithScripts(
			[]script.Listing{{Path: "script.feature." + partyRangeProbe}},
			script.Catalog{"script.feature." + partyRangeProbe: func() script.Script { return script.Script{QuestID: -1} }}),
		gameservertest.WithDataReloads(fixedConfig(network.PlayerConfig{
			RespawnRestoreHP: 0.7, MaxBuffsAmount: 20, WeightLimitMultiplier: 1,
			KillRewards: &manager.KillRewardSettings{PartyRange: 1500, MultipleItemDrop: true},
		})))
	gm := srv.Client
	enterWorld(t, gm)
	drain(t, gm)

	x, y, z := srv.PlayerPosition(t, gmID)
	var npcs []*script.NPC
	for _, dx := range []int{499, 500, 1000, 1499, 1500} {
		h := srv.SpawnHostileNPCAt(t, location.Location{X: x + dx, Y: y, Z: z})
		if nx, ny, nz := h.Position(); nx != x+dx || ny != y || nz != z {
			t.Fatalf("NPC placed at %d,%d,%d, want %d,%d,%d", nx, ny, nz, x+dx, y, z)
		}
		npcs = append(npcs, script.NPCOf(h))
	}
	drain(t, gm)

	check := func(want ...bool) {
		t.Helper()
		var got []bool
		if !srv.RunScript(t, gmID, partyRangeProbe, func(sc *script.Script, p *script.Player) {
			for _, n := range npcs {
				got = append(got, sc.InPartyRange(p, n))
			}
		}) {
			t.Fatal("script invocation panicked")
		}
		for i, dx := range []int{499, 500, 1000, 1499, 1500} {
			if got[i] != want[i] {
				t.Errorf("player %d from the NPC: in party range %v, want %v", dx, got[i], want[i])
			}
		}
	}
	check(true, false, false, false, false)

	// An NPC against an NPC: 501 apart is out at 500.
	if !srv.RunScript(t, gmID, partyRangeProbe, func(sc *script.Script, _ *script.Player) {
		if sc.InPartyRange(npcs[0], npcs[2]) {
			t.Error("NPCs 501 apart read within a 500 party range")
		}
		if !sc.InPartyRange(npcs[0], npcs[1]) {
			t.Error("NPCs 1 apart read outside the party range")
		}
	}) {
		t.Fatal("script invocation panicked")
	}

	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload config")), configReloaded)
	check(true, true, true, true, false)

	if srv.RunScript(t, gmID, partyRangeProbe, func(sc *script.Script, p *script.Player) { sc.InPartyRange(p, nil) }) {
		t.Fatal("InPartyRange of a handle on nothing returned")
	}
}
