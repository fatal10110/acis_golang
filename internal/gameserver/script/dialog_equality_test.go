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

// TestQuestEqualityMatchesReferenceGolden: two behaviors are one script, two
// scripts of one quest id are one when their names match, and any other two
// are one only when their paths match.
func TestQuestEqualityMatchesReferenceGolden(t *testing.T) {
	r := equalityRegistry(t)
	scriptcontract.Run(t, "dialog.quest_equality", func(t *testing.T, row scriptcontract.Row) {
		a, b := equalityScript(t, r, row.Str(t, "a")), equalityScript(t, r, row.Str(t, "b"))
		if got, want := equal(a, b), row.Bool(t, "equal"); got != want {
			t.Fatalf("equal(%s, %s) = %v, want %v", a.path, b.path, got, want)
		}
	})
}

// TestQuestEqualityTellsSameNamesApartByPath: two scripts named alike in
// different directories are not one script.
func TestQuestEqualityTellsSameNamesApartByPath(t *testing.T) {
	r := equalityRegistry(t)
	a := equalityScript(t, r, "script.teleport.NoblesseTeleporter")
	b := equalityScript(t, r, "script.feature.NoblesseTeleporter")
	if a.Name != b.Name {
		t.Fatalf("names %q and %q differ", a.Name, b.Name)
	}
	if equal(a, b) {
		t.Fatal("same-named scripts in different directories are equal")
	}
	if !equal(a, a) {
		t.Fatal("a script is not equal to itself")
	}
}
