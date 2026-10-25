package npcs

import (
	"fmt"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// A civilian master hears its private die: the dying hostile private's own
// party-died hook runs first, then its civilian master's, then the other
// civilian private's, each with the dying private as the caller.
func TestScriptCivilianMasterHearsItsPrivateDie(t *testing.T) {
	t.Parallel()
	log := &partyLog{}
	kinds := map[int32]script.NPCKind{
		spawnWolfID: script.KindHostile, spawnPupID: script.KindHostile,
		spawnTabletID: script.KindFolk, spawnClerkID: script.KindFolk,
	}
	recorder := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{spawnTabletID, spawnPupID, spawnClerkID}, Hooks: script.Hooks{
			OnPartyDied: func(_ *script.Script, e script.PartyDied) {
				log.add("PARTY_DIED caller=%d called=%d", e.Caller.NpcID(), e.Called.NpcID())
			},
		}}
	}
	w := bootScriptSpawns(t,
		gameservertest.WithNPCs(civilianPartyTemplates()),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: "ai.PartyDiedRecorder"}},
			script.Catalog{"ai.PartyDiedRecorder": recorder}),
	)
	master := w.spawner.AddSpawn(spawnTabletID, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
	tablet := spawnedNPC(t, w, spawnTabletID).(*npc.Folk)
	w.spawner.CreateOnePrivateEx(master, spawnPupID, script.Loc{X: w.at.X + 80, Y: w.at.Y, Z: w.at.Z}, 0, script.Params{})
	w.spawner.CreateOnePrivate(master, spawnClerkID, 0)
	pup := spawnedNPC(t, w, spawnPupID).(*npc.Hostile)
	clerk := spawnedNPC(t, w, spawnClerkID).(*npc.Folk)
	p := w.liveObject(t, w.player)

	onPlayerQueue(t, w, func() { pup.TakeDamage(1_000_000, p) })
	w.srv.Settle(t)
	want := []string{
		fmt.Sprintf("PARTY_DIED caller=%d called=%d", spawnPupID, spawnPupID),
		fmt.Sprintf("PARTY_DIED caller=%d called=%d", spawnPupID, spawnTabletID),
		fmt.Sprintf("PARTY_DIED caller=%d called=%d", spawnPupID, spawnClerkID),
	}
	if got := log.take(); !slices.Equal(got, want) {
		t.Fatalf("party-died hooks = %q, want %q", got, want)
	}
	if !pup.Dead() || tablet.Dead() || clerk.Master() != npc.PartyMember(tablet) {
		t.Fatalf("pup dead %v, tablet dead %v, clerk still the tablet's private %v", pup.Dead(), tablet.Dead(), clerk.Master() != nil)
	}
}
