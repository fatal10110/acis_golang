package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// competing puts c in stadium game's match, fighting when started.
func competing(c *Character, game int, started bool) {
	c.SetOlympiadMode(true)
	c.SetOlympiadGameID(game)
	c.SetOlympiadStarted(started)
}

// TestOlympiadMatchSocialPolicy pins the same-match rules the reference
// checks before the duel, party and clan ones (Playable
// isInSameActiveOlympiadMatch, canCastOffensiveSkillOnPlayable,
// isAttackableWithoutForceBy; Player's beneficial cast check): two clanmates
// fighting each other in the same started match need no force and may cast
// anything offensive; a match not yet started, or another stadium's, leaves
// the clan rule in force; a beneficial cast on a flagged competitor of the
// same match, started or not, needs no CTRL.
func TestOlympiadMatchSocialPolicy(t *testing.T) {
	cases := []struct {
		name                   string
		casterGame, targetGame int
		started                bool
		allowed                bool
	}{
		{name: "same started match", casterGame: 3, targetGame: 3, started: true, allowed: true},
		{name: "same match not started", casterGame: 3, targetGame: 3},
		{name: "other stadium", casterGame: 3, targetGame: 4, started: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, tgt := policyPair(t, policyGraph{}, policySide{clan: 10}, policySide{clan: 10})
			competing(c, tc.casterGame, tc.started)
			competing(tgt, tc.targetGame, tc.started)
			if got := tgt.AttackableWithoutForceBy(c); got != tc.allowed {
				t.Errorf("attackable without force = %v, want %v", got, tc.allowed)
			}
			if got := c.CanCastOnPlayable(tgt, policyDebuff, false, true, true); got != tc.allowed {
				t.Errorf("offensive debuff allowed = %v, want %v", got, tc.allowed)
			}
		})
	}

	for _, tc := range []struct {
		name    string
		game    int
		allowed bool
	}{
		{name: "same match", game: 3, allowed: true},
		{name: "other stadium", game: 4},
	} {
		t.Run("beneficial "+tc.name, func(t *testing.T) {
			c, tgt := policyPair(t, policyGraph{}, policySide{}, policySide{flag: task.PvPFlagOn})
			competing(c, 3, false)
			competing(tgt, tc.game, false)
			if got := c.CanCastOnPlayable(tgt, policyDebuff, false, false, true); got != tc.allowed {
				t.Fatalf("beneficial cast allowed = %v, want %v", got, tc.allowed)
			}
		})
	}
}
