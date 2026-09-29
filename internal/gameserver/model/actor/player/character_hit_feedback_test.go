package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// wrappedSelf is a character reached through a wrapper that embeds it, the
// shape of the session's live player: its interface value differs from the
// bare *Character's, but it is the same creature.
type wrappedSelf struct{ *Character }

func hitFeedbackVictim() *Character {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 200})
	return c
}

func hitFeedbackAttacker() *Character {
	a := liveCharacter(2, combatTemplate(), combatItems())
	a.Name = "attacker"
	return a
}

// TestDOTTickFromAnotherPlayerSendsNoDamageReport pins the isDOT exclusion of
// S1_GAVE_YOU_S2_DMG: a damage-over-time tick from another player still
// drains CP and reports the victim's status, but never names the attacker,
// while the same hit outside a DOT tick does.
func TestDOTTickFromAnotherPlayerSendsNoDamageReport(t *testing.T) {
	attacker := hitFeedbackAttacker()

	dot := hitFeedbackVictim()
	rec := recordEvents(dot)
	dot.ReduceHPByDOT(50, attacker, true)

	if got := event.Count[event.DamageReceived](rec); got != 0 {
		t.Fatalf("DOT tick DamageReceived events = %d, want 0", got)
	}
	if got := countVitals(rec); got != 1 {
		t.Fatalf("DOT tick VitalsChanged events = %d, want 1", got)
	}
	if dot.CP() != 150 || dot.HP() != 500 {
		t.Fatalf("DOT tick cp/hp = %v/%v, want 150/500 (CP absorbs a playable's tick)", dot.CP(), dot.HP())
	}

	hit := hitFeedbackVictim()
	hitRec := recordEvents(hit)
	hit.ReduceHPByDOT(50, attacker, false)

	got := event.Of[event.DamageReceived](hitRec)
	if len(got) != 1 || got[0].AttackerName != "attacker" || got[0].Amount != 50 {
		t.Fatalf("non-DOT hit DamageReceived = %+v, want one {attacker 50}", got)
	}
}

// TestSelfDamageSendsNoDamageReportAndDrainsNoCP pins the self exclusion of
// PlayerStatus.reduceHp's attacker block for a wrapped self attacker (the
// drowning shape) as well as the bare character: no S1_GAVE_YOU_S2_DMG, and
// the damage goes straight to HP without touching CP.
func TestSelfDamageSendsNoDamageReportAndDrainsNoCP(t *testing.T) {
	cases := []struct {
		name string
		hit  func(c *Character)
	}{
		{"wrapped self periodic", func(c *Character) { c.ReduceHPByDOT(50, wrappedSelf{c}, false) }},
		{"bare self periodic", func(c *Character) { c.ReduceHPByDOT(50, c, false) }},
		{"wrapped self skill hit", func(c *Character) { c.ReduceHP(50, wrappedSelf{c}, modelskill.Definition{}) }},
		{"wrapped self auto-attack", func(c *Character) { c.TakeDamage(50, wrappedSelf{c}) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := hitFeedbackVictim()
			c.SetRollSource(zeroRoll)
			rec := recordEvents(c)

			tc.hit(c)

			if got := event.Of[event.DamageReceived](rec); len(got) != 0 {
				t.Fatalf("DamageReceived = %+v, want none for self damage", got)
			}
			if c.CP() != 200 || c.HP() != 450 {
				t.Fatalf("cp/hp = %v/%v, want 200/450 (self damage skips CP)", c.CP(), c.HP())
			}
		})
	}
}
