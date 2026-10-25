package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestRootedWarriorKeepsItsHateMidSwing runs the Warrior's attacked hook
// under the root 1201 while its own swing at its most hated creature is in
// flight. The script's attack query has no busy check: the swing in flight
// does not make the character unattackable, so a hit from more than 40 away
// keeps the Warrior's hate of the character and adds the hit's hate twice,
// 125 from the Warrior's own hook and 125 from its root branch.
func TestRootedWarriorKeepsItsHateMidSwing(t *testing.T) {
	t.Parallel()
	tmpl := warriorTemplate(warriorID, nil)
	// A reach of 40 + both radii + 10 holds the character 45 away.
	tmpl.BaseAttackRange = 40
	w := bootIndividual1b(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptSkills(dryadRoot())}}, tmpl)
	n := w.spawn(t, warriorID, 45, 0)
	w.land(t, dryadRootID, n)
	w.hit(t, n, 10)
	w.srv.AdvanceUntil(t, "the Warrior's swing at the character", func() bool {
		w.ticks(t, 1)
		return n.AttackingNow()
	})

	before := n.AggroHate(w.player)
	if before <= 0 {
		t.Fatalf("setup: hate of the character = %v, want it hated", before)
	}
	if !script.NPCOf(n).CanAttack(w.charOf(t)) {
		t.Fatal("CanAttack(character) = false while the Warrior's swing is in flight, want true")
	}
	w.hit(t, n, 10)
	if !n.AttackingNow() {
		t.Fatal("setup: the Warrior's swing ended before the hit")
	}
	if got := n.AggroHate(w.player) - before; got != 250 {
		t.Fatalf("hate gained from the hit mid-swing = %v, want 250 (the hate of the character kept)", got)
	}
}
