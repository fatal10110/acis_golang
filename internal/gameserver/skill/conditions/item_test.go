package conditions

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

func itemLeaf(kind string, attrs map[string]string, children ...item.Condition) item.Condition {
	return item.Condition{Kind: kind, Attrs: attrs, Children: children}
}

// TestEvaluateItemFollowsTheConditionReader pins item use conditions to the
// same reader as skill conditions: an unrecognized attribute beside a
// recognized one is ignored, <and>/<or> drop a child holding no condition, a
// <not> reads only its first child, and a <not> around no condition aborts
// the use with no feedback. A clause holding no condition is skipped.
func TestEvaluateItemFollowsTheConditionReader(t *testing.T) {
	gameChance := itemLeaf("game", map[string]string{"chance": "50"})
	level := func(n string) item.Condition { return itemLeaf("player", map[string]string{"level": n}) }
	low := &player{creature: creature{level: 10}}
	for _, tt := range []struct {
		name    string
		root    item.Condition
		held    bool
		aborted bool
	}{
		{"and drops game chance", itemLeaf("and", nil, gameChance, level("1")), true, false},
		{"and drops game chance, level too high", itemLeaf("and", nil, gameChance, level("11")), false, false},
		{"unrecognized attribute beside level", itemLeaf("player", map[string]string{"bogus": "1", "level": "1"}), true, false},
		{"or drops an empty player", itemLeaf("or", nil, itemLeaf("player", nil), level("99")), false, false},
		{"not reads its first child only", itemLeaf("not", nil, level("99"), level("1")), true, false},
		{"not of game chance", itemLeaf("not", nil, gameChance), false, true},
		{"whole clause holds no condition", gameChance, true, false},
		{"target is the user", itemLeaf("target", map[string]string{"hp_min_max": "0,100"}), true, false},
	} {
		uc := item.UseCondition{Root: tt.root, MessageID: 113, AddName: true}
		failed, held, aborted := EvaluateItem([]item.UseCondition{uc}, low, low)
		if held != tt.held || aborted != tt.aborted {
			t.Errorf("%s: (held, aborted) = (%v, %v), want (%v, %v)", tt.name, held, aborted, tt.held, tt.aborted)
			continue
		}
		switch {
		case held && failed.MessageID != 0:
			t.Errorf("%s: held but reported %+v", tt.name, failed)
		case aborted && (failed.MessageID != 0 || failed.AddName || failed.Message != ""):
			t.Errorf("%s: aborted with feedback %+v, want none", tt.name, failed)
		case !held && !aborted && failed.MessageID != 113:
			t.Errorf("%s: failed clause %+v, want the clause's feedback", tt.name, failed)
		}
	}
}

// TestEvaluateItemHasNoSkillOrPlayerForAPet: a use condition is tested with
// no skill, so <skill stat> fails, and a player-only check fails for a
// non-player user while a level check reads its level.
func TestEvaluateItemHasNoSkillOrPlayerForAPet(t *testing.T) {
	pet := &npc{creature: creature{level: 50}}
	for _, tt := range []struct {
		name string
		root item.Condition
		user Actor
		held bool
	}{
		{"skill stat", itemLeaf("skill", map[string]string{"stat": "maxHp"}), &player{}, false},
		{"pet level", itemLeaf("player", map[string]string{"level": "50"}), pet, true},
		{"pet level too high", itemLeaf("player", map[string]string{"level": "51"}), pet, false},
		{"pet sex", itemLeaf("player", map[string]string{"sex": "0"}), pet, false},
	} {
		_, held, _ := EvaluateItem([]item.UseCondition{{Root: tt.root}}, tt.user, tt.user)
		if held != tt.held {
			t.Errorf("%s: held = %v, want %v", tt.name, held, tt.held)
		}
	}
}
