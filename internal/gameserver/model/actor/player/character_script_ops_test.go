package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// TestIsNewbieMatchesReference pins Player.isNewbie: at most one class
// change and level 25 or lower, and with checkLowLevel level 6 or higher.
func TestIsNewbieMatchesReference(t *testing.T) {
	c, err := NewCharacter(1, humanFighterTemplate(), "acct", "newbie", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() error: %v", err)
	}
	for _, tc := range []struct {
		class, level int
		low, any     bool
	}{
		{class: 0, level: 1, low: false, any: true},
		{class: 0, level: 5, low: false, any: true},
		{class: 0, level: 6, low: true, any: true},
		{class: 0, level: 25, low: true, any: true},
		{class: 0, level: 26, low: false, any: false},
		{class: 1, level: 20, low: true, any: true},   // warrior: first class change
		{class: 2, level: 20, low: false, any: false}, // gladiator: second class change
	} {
		c.SetClassID(tc.class)
		c.CharLevel = tc.level
		if got := c.IsNewbie(true); got != tc.low {
			t.Errorf("class %d level %d: IsNewbie(true) = %v, want %v", tc.class, tc.level, got, tc.low)
		}
		if got := c.IsNewbie(false); got != tc.any {
			t.Errorf("class %d level %d: IsNewbie(false) = %v, want %v", tc.class, tc.level, got, tc.any)
		}
	}
}

// TestDetachingCharacterGetsNoScriptClanReputation: a script's reputation
// reward is a give, which a detaching character never gets.
func TestDetachingCharacterGetsNoScriptClanReputation(t *testing.T) {
	c, err := NewCharacter(1, humanFighterTemplate(), "acct", "leader", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() error: %v", err)
	}
	rec := recordEvents(c)
	c.AddScriptClanReputation(10)
	if got := event.Count[event.ClanReputationAdded](rec); got != 1 {
		t.Fatalf("reputation requests = %d, want 1", got)
	}
	c.MarkDetaching()
	c.AddScriptClanReputation(10)
	if got := event.Count[event.ClanReputationAdded](rec); got != 1 {
		t.Fatalf("reputation requests after detach = %d, want still 1", got)
	}
}
