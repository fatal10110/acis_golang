package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// Reference: PlayerStatus.reduceHp (PlayerStatus.java:140-151, 196-212),
// Playable.isInSameActiveDuel/canCastOffensiveSkillOnPlayable,
// Player.canCastBeneficialSkillOnPlayable and onKillUpdatePvPKarma.

// duelHitPair puts two combat characters, 1 and 2, in duel 1 in state, the
// first with 500 HP and no CP.
func duelHitPair(state duel.State) (a, b *Character) {
	a = liveCharacter(1, combatTemplate(), combatItems())
	b = liveCharacter(2, combatTemplate(), combatItems())
	a.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500})
	for _, c := range []*Character{a, b} {
		c.JoinDuel(1)
		c.SetDuelState(state)
	}
	return a, b
}

// TestDuelHitRules pins who may hit a duellist and which hits interrupt its
// duel: the opponent hits freely, a bystander's hit or one landing during
// the countdown still lands but interrupts it, and a duellist that already
// lost or won takes nothing more.
func TestDuelHitRules(t *testing.T) {
	cases := []struct {
		name      string
		state     duel.State
		bystander bool
		wantHP    float64
		wantState duel.State
	}{
		{name: "opponent hit", state: duel.Duelling, wantHP: 450, wantState: duel.Duelling},
		{name: "bystander hit", state: duel.Duelling, bystander: true, wantHP: 450, wantState: duel.Interrupted},
		{name: "opponent hit during the countdown", state: duel.Countdown, wantHP: 450, wantState: duel.Interrupted},
		{name: "hit on a defeated duellist", state: duel.Dead, wantHP: 500, wantState: duel.Dead},
		{name: "hit on a winner", state: duel.Winner, wantHP: 500, wantState: duel.Winner},
		{name: "bystander hit on a defeated duellist", state: duel.Dead, bystander: true, wantHP: 500, wantState: duel.Dead},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b := duelHitPair(tc.state)
			attacker := b
			if tc.bystander {
				attacker = liveCharacter(3, combatTemplate(), combatItems())
			}
			a.TakeDamage(50, attacker)
			if a.HP() != tc.wantHP {
				t.Fatalf("HP = %v, want %v", a.HP(), tc.wantHP)
			}
			if got := a.DuelState(); got != tc.wantState {
				t.Fatalf("duel state = %d, want %d", got, tc.wantState)
			}
		})
	}
}

// TestDuelLethalHitDefeats: a hit that would kill a fighting duellist
// leaves it at 1 HP, locks its skills and reports the defeat; the same hit
// during the countdown leaves 1 HP too but defeats nobody. Leaving the duel
// lifts a defeated duellist's lock.
func TestDuelLethalHitDefeats(t *testing.T) {
	a, b := duelHitPair(duel.Duelling)
	rec := recordEvents(a)
	a.TakeDamage(5000, b)
	if a.Dead() || a.HP() != 1 {
		t.Fatalf("after a lethal duel hit dead = %v, HP = %v; want alive at 1", a.Dead(), a.HP())
	}
	if !a.AllSkillsDisabled() {
		t.Fatal("a defeated duellist's skills are not locked")
	}
	if got := event.Count[event.DuelDefeated](rec); got != 1 {
		t.Fatalf("DuelDefeated events = %d, want 1", got)
	}
	a.SetDuelState(duel.Dead) // the manager's Defeat
	a.TakeDamage(5000, b)
	if a.HP() != 1 || event.Count[event.DuelDefeated](rec) != 1 {
		t.Fatalf("a defeated duellist took another hit: HP %v, defeats %d", a.HP(), event.Count[event.DuelDefeated](rec))
	}
	a.LeaveDuel()
	if a.AllSkillsDisabled() || a.InDuel() || a.DuelState() != duel.NoDuel {
		t.Fatal("leaving the duel kept the defeat lock or the duel")
	}

	c, d := duelHitPair(duel.Countdown)
	rec = recordEvents(c)
	c.TakeDamage(5000, d)
	if c.Dead() || c.HP() != 1 {
		t.Fatalf("after a lethal countdown hit dead = %v, HP = %v; want alive at 1", c.Dead(), c.HP())
	}
	if c.AllSkillsDisabled() || event.Count[event.DuelDefeated](rec) != 0 {
		t.Fatal("a lethal hit during the countdown defeated the duellist")
	}
	if c.DuelState() != duel.Interrupted {
		t.Fatalf("duel state = %d, want Interrupted", c.DuelState())
	}
}

// inDuel puts each character in duel id, in state.
func inDuel(id int32, state duel.State, cs ...*Character) {
	for _, c := range cs {
		c.JoinDuel(id)
		c.SetDuelState(state)
	}
}

// TestDuelSameDuelSocialRules: two players fighting each other in one duel
// attack and cast offensive skills on each other without force, even as
// party mates, and help each other without CTRL even when flagged. A
// player of another duel, or an opponent already out of the fight, gets
// the ordinary rules.
func TestDuelSameDuelSocialRules(t *testing.T) {
	t.Run("party mates fight freely", func(t *testing.T) {
		c, tgt := policyPair(t, samePartyGraph(), policySide{}, policySide{})
		inDuel(1, duel.Duelling, c, tgt)
		if allowed, decided := c.SocialWithoutForce(c, tgt); !allowed || !decided {
			t.Fatalf("SocialWithoutForce = (%v, %v), want (true, true)", allowed, decided)
		}
		if !c.AttackableWithoutForceBy(tgt) {
			t.Fatal("a duel opponent needs force")
		}
		if !c.CanCastOnPlayable(tgt, policyDebuff, false, true, true) {
			t.Fatal("a debuff on a duel opponent party mate was refused")
		}
	})
	t.Run("another duel keeps the party rule", func(t *testing.T) {
		c, tgt := policyPair(t, samePartyGraph(), policySide{}, policySide{})
		inDuel(1, duel.Duelling, c)
		inDuel(2, duel.Duelling, tgt)
		if allowed, _ := c.SocialWithoutForce(c, tgt); allowed {
			t.Fatal("a player of another duel is attacked without force")
		}
		if c.CanCastOnPlayable(tgt, policyDebuff, true, true, true) {
			t.Fatal("a debuff on a party mate of another duel was allowed")
		}
	})
	t.Run("a defeated opponent keeps the party rule", func(t *testing.T) {
		c, tgt := policyPair(t, samePartyGraph(), policySide{}, policySide{})
		inDuel(1, duel.Duelling, c)
		inDuel(1, duel.Dead, tgt)
		if c.CanCastOnPlayable(tgt, policyDebuff, true, true, true) {
			t.Fatal("a debuff on a defeated duel opponent was allowed")
		}
	})
	t.Run("flagged duel opponent helped without ctrl", func(t *testing.T) {
		c, tgt := policyPair(t, policyGraph{}, policySide{}, policySide{flag: task.PvPFlagOn})
		inDuel(1, duel.Duelling, c, tgt)
		if !c.CanCastOnPlayable(tgt, policyDebuff, false, false, true) {
			t.Fatal("a buff on a flagged player of the same duel needed CTRL")
		}
		inDuel(2, duel.Duelling, tgt)
		if c.CanCastOnPlayable(tgt, policyDebuff, false, false, true) {
			t.Fatal("a buff on a flagged player of another duel went without CTRL")
		}
	})
}

// TestDuelSummonKillGivesNoKarma: killing the summon of a duel opponent
// leaves the killer's karma alone; the same kill outside a duel gives it.
func TestDuelSummonKillGivesNoKarma(t *testing.T) {
	owner, killer := policyPair(t, policyGraph{}, policySide{}, policySide{})
	inDuel(1, duel.Duelling, owner, killer)
	owner.AwardSummonKillKarma(killer)
	if killer.Karma() != 0 {
		t.Fatalf("karma after a duel summon kill = %d, want 0", killer.Karma())
	}
	owner.LeaveDuel()
	killer.LeaveDuel()
	owner.AwardSummonKillKarma(killer)
	if killer.Karma() == 0 {
		t.Fatal("a summon kill outside a duel gave no karma, the baseline this test relies on")
	}
}
