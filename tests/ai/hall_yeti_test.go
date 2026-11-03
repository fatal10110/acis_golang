package ai

import (
	"slices"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/hall"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestCrazyYetiShoutsAsItAppearsThenAttacks ports AzitWateringCrazyYeti on
// its civilian id: as it appears through the live population the enraged
// yeti shouts NpcString 1010627, and nothing else. Seen, the character is
// attacked: one swing at it, the yeti's intention an attack. The swing
// ends and no attack-finished hook runs, a civilian's attack reporting no
// end; the hook is wrapped to log once it is reached.
func TestCrazyYetiShoutsAsItAppearsThenAttacks(t *testing.T) {
	t.Parallel()
	log := &folkLog{}
	const path = "script.ai.siegablehall.AzitWateringCrazyYeti"
	yeti := func() script.Script {
		s := hall.CrazyYeti()
		base := s.Hooks
		s.Hooks = base.With(script.Hooks{
			OnAttackFinished: func(sc *script.Script, e script.AttackFinished) {
				log.add("ATTACK_FINISHED")
				base.AttackFinished(sc, e)
			},
		})
		return s
	}
	catalog := script.Catalog{"script.ai.individual.DefaultNpc": defaultnpc.New, path: yeti}
	list := []script.Listing{{Path: "script.ai.individual.DefaultNpc"}, {Path: path}}
	kinds := map[int32]script.NPCKind{enragedYetiID: script.KindFolk}
	scripts := raidBoot{opts: []gameservertest.Option{gameservertest.WithNPCScripts(kinds, list, catalog)}}
	w := bootHalls(t, 0, 0, location.Location{}, scripts, folkFighterTemplate(enragedYetiID))

	n, err := w.srv.NpcSpawns.AddSpawn(enragedYetiID, w.at.X+40, w.at.Y, w.at.Z, 0, false, 0)
	if err != nil {
		t.Fatalf("spawn %d: %v", enragedYetiID, err)
	}
	w.srv.Settle(t)
	f := n.(*npc.Folk)
	id := f.ObjectID()
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got, want := says(frames, id), []string{script.NpcString(1010627)}; !slices.Equal(got, want) {
		t.Fatalf("lines as it appears = %q, want %q", got, want)
	}

	swings := framesOf(cycle(t, w.srv), serverpackets.OpcodeAttack, id)
	if len(swings) != 1 || attackTarget(swings[0]) != w.player.ObjectID() {
		t.Fatalf("swings = %v, want one at the character", swings)
	}
	if got := f.CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("intention = %v, want attack", got)
	}
	w.srv.Advance(t, 3*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	if got := log.take(); len(got) != 0 {
		t.Fatalf("hooks = %q after the swings ended, want none", got)
	}
}
