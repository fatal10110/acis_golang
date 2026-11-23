package script

import (
	"maps"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// equalityRegistry registers stand-ins for every script the
// dialog.quest_equality golden names, plus a script whose name repeats the
// teleporter's in another directory.
func equalityRegistry(t *testing.T) *Registry {
	t.Helper()
	plain := func() Script { return Script{} }
	catalog := Catalog{
		"quest.Q001_LettersOfLove":           func() Script { return Script{QuestID: 1} },
		"quest.Q003_WillTheSealBeBroken":     func() Script { return Script{QuestID: 3} },
		"script.teleport.NoblesseTeleporter": plain,
		"script.feature.NoblesseTeleporter":  plain,
		"script.ai.boss.antharas.Antharas":   behavior(1),
		"script.ai.boss.baium.Archangel":     behavior(2),
	}
	r, logs := build(t, listOf(slices.Sorted(maps.Keys(catalog))...), catalog)
	for _, e := range r.entries {
		if e.script == nil {
			t.Fatalf("%s not registered; logs:\n%s", e.path, logs)
		}
	}
	return r
}

// equalityScript resolves a golden token: a script's path, else its name.
func equalityScript(t *testing.T, r *Registry, token string) *Script {
	t.Helper()
	for _, e := range r.entries {
		if e.script != nil && e.path == token {
			return e.script
		}
	}
	for _, e := range r.entries {
		if e.script != nil && e.script.Name == token {
			return e.script
		}
	}
	t.Fatalf("no stand-in script %q", token)
	return nil
}

// TestQuestEventScriptIdentity: a quest event runs only through the very
// script it names. The reference counts two scripts of one quest id with
// the same name, or of the same class, as one, as the golden pins; it also
// counts any two behaviors as one, which #5160 drops: a behavior bound to
// an NPC is not another behavior named by an event.
func TestQuestEventScriptIdentity(t *testing.T) {
	r := equalityRegistry(t)
	diverges := map[string]bool{"two-behaviors": true}
	scriptcontract.Run(t, "dialog.quest_equality", func(t *testing.T, row scriptcontract.Row) {
		a, b := equalityScript(t, r, row.Str(t, "a")), equalityScript(t, r, row.Str(t, "b"))
		want := row.Bool(t, "equal") != diverges[row.ID]
		if got := containsScript([]*Script{a}, b); got != want {
			t.Fatalf("%s names %s: one script = %v, want %v", a.path, b.path, got, want)
		}
	})
}

// TestQuestEventTellsSameNamesApart: two scripts named alike in different
// directories are not one script.
func TestQuestEventTellsSameNamesApart(t *testing.T) {
	r := equalityRegistry(t)
	a := equalityScript(t, r, "script.teleport.NoblesseTeleporter")
	b := equalityScript(t, r, "script.feature.NoblesseTeleporter")
	if a.Name != b.Name {
		t.Fatalf("names %q and %q differ", a.Name, b.Name)
	}
	if containsScript([]*Script{a}, b) {
		t.Fatal("same-named scripts in different directories are one script")
	}
}
