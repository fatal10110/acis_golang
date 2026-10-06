package script

import (
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// TestHelperPanicAbortsTheStringHook calls helpers on a missing player in a
// first talk: a give of nothing returns before it reads the player, as the
// reference does, and the next give panics where the reference throws, so
// the hook stops there, the panic is logged, and its answer sends nothing.
func TestHelperPanicAbortsTheStringHook(t *testing.T) {
	var reached []string
	catalog := Catalog{
		"script.Giver": func() Script {
			return Script{Bind: Bindings{EventFirstTalk: {2}}, Hooks: Hooks{OnFirstTalk: func(s *Script, e FirstTalk) string {
				s.GiveItems(e.Player, 57, 0)
				reached = append(reached, "empty give")
				s.GiveItems(e.Player, 57, 1)
				reached = append(reached, "give")
				return "given.htm"
			}}}
		},
	}
	r, logs := build(t, listOf("script.Giver"), catalog)
	res, bound := r.FirstTalk(2, FirstTalk{})
	if !bound || res != (Result{Kind: ResultAborted}) {
		t.Fatalf("first talk = %+v, %v; want ResultAborted", res, bound)
	}
	if len(reached) != 1 || reached[0] != "empty give" {
		t.Fatalf("the hook reached %v, want only the empty give", reached)
	}
	if out := logs.String(); !strings.Contains(out, `"script":"script.Giver","hook":"onFirstTalk"`) || !strings.Contains(out, "helpers_test.go") {
		t.Fatalf("panic not logged with the hook's stack: %s", out)
	}
}

// TestInvokeRunsTheNamedScript runs a function as an invocation of a
// script found by name: any case finds it, an unknown name runs nothing,
// and a panic is recovered and logged.
func TestInvokeRunsTheNamedScript(t *testing.T) {
	env := &Env{}
	catalog := Catalog{"quest.Q001_LettersOfLove": func() Script { return Script{QuestID: 1, Items: []int32{687}} }}
	logs := &logBuffer{}
	r := Build(listOf("quest.Q001_LettersOfLove"), catalog, Config{KindOf: allTemplates, Log: zerolog.New(logs), Env: env})

	var got *Script
	if !r.Invoke("q001_lettersoflove", func(s *Script) { got = s }) {
		t.Fatal("Invoke of a registered script reported false")
	}
	if got == nil || got.Name != "Q001_LettersOfLove" || got.env != env || len(got.Items) != 1 {
		t.Fatalf("invoked %+v, want the registered script with its environment", got)
	}
	if r.Invoke("Q002_WhatWomenWant", func(*Script) { t.Fatal("ran an unknown script") }) {
		t.Fatal("Invoke of an unknown script reported true")
	}
	if r.Invoke("Q001_LettersOfLove", func(*Script) { panic("boom") }) {
		t.Fatal("Invoke of a panicking function reported true")
	}
	if !strings.Contains(logs.String(), `"panic":"boom"`) {
		t.Fatalf("panic not logged: %s", logs.String())
	}
}
