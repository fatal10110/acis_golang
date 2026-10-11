package ai

import (
	"fmt"
	"slices"
	"testing"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TestFolkBehaviorReadsAndStopsItsHate drives the script handle's hate
// reads and controls on a civilian NPC from its bound behavior, each hit's
// damage choosing the step. The first hit queues an attack on the attacker
// and a light do-nothing, and the most hated creature reads back as the
// attacker. The second stops that hate: the NPC then hates no one and its
// attack desire on the attacker is gone, the do-nothing kept. The third
// clears every desire the NPC holds.
func TestFolkBehaviorReadsAndStopsItsHate(t *testing.T) {
	t.Parallel()
	log := &folkLog{}
	hated := func(n *script.NPC) string {
		if c := n.MostHated(); c != nil {
			return fmt.Sprintf("%d", c.ObjectID())
		}
		return "none"
	}
	guard := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{folkPatrolID}, Hooks: script.Hooks{
			OnAttacked: func(_ *script.Script, e script.Attacked) {
				switch e.Damage {
				case 10:
					e.NPC.AddAttackDesire(e.Attacker, 2000)
					e.NPC.AddDoNothingDesire(40, 100)
				case 20:
					e.NPC.StopHate(e.Attacker)
				case 30:
					e.NPC.ClearDesires()
				}
				log.add("ATTACKED damage=%d hated=%s", e.Damage, hated(e.NPC))
			},
		}}
	}
	srv, at := bootFolkDesires(t, script.Catalog{"ai.FolkGuardHate": guard})
	player := characterOf(t, srv)
	f := srv.SpawnFolkNPCAt(t, folkFighterTemplate(folkPatrolID), location.Location{X: at.X + 40, Y: at.Y, Z: at.Z})
	srv.ReadQueued(t, srv.Client)
	onPlayer := &actorai.Desire{Kind: actorai.IntentionAttack, FinalTarget: player}

	hitFolk(t, srv, f, player, 10)
	if got, want := log.take(), []string{fmt.Sprintf("ATTACKED damage=10 hated=%d", player.ObjectID())}; !slices.Equal(got, want) {
		t.Fatalf("hooks = %q, want %q", got, want)
	}
	if !f.Desires().Has(onPlayer) || f.Desires().Len() != 2 {
		t.Fatalf("desires = %d, attack on the character %v; want it and the do-nothing", f.Desires().Len(), f.Desires().Has(onPlayer))
	}

	hitFolk(t, srv, f, player, 20)
	if got, want := log.take(), []string{"ATTACKED damage=20 hated=none"}; !slices.Equal(got, want) {
		t.Fatalf("hooks = %q, want %q", got, want)
	}
	if c, ok := f.MostHated(); ok {
		t.Fatalf("most hated = %v after StopHate, want none", c)
	}
	if f.Desires().Has(onPlayer) || !f.Desires().HasKind(actorai.IntentionNothing) || f.Desires().Len() != 1 {
		t.Fatalf("desires = %d, attack on the character %v; want only the do-nothing", f.Desires().Len(), f.Desires().Has(onPlayer))
	}

	hitFolk(t, srv, f, player, 30)
	if got, want := log.take(), []string{"ATTACKED damage=30 hated=none"}; !slices.Equal(got, want) {
		t.Fatalf("hooks = %q, want %q", got, want)
	}
	if n := f.Desires().Len(); n != 0 {
		t.Fatalf("desires = %d after ClearDesires, want none", n)
	}
}
