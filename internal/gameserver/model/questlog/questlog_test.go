package questlog

import (
	"reflect"
	"strings"
	"testing"
)

// quests are the quests resolve knows, by case-insensitive name.
var quests = []Quest{
	{Name: "Q001_LettersOfLove", ID: 1},
	{Name: "Q002_WhatWomenWant", ID: 2},
	{Name: "Q003_WillTheSealBeBroken", ID: 3},
	{Name: "Q006_StepIntoTheFuture", ID: 6},
	{Name: "NoblesseTeleporter", ID: -1},
}

func resolve(name string) (Quest, bool) {
	for _, q := range quests {
		if strings.EqualFold(q.Name, name) {
			return q, true
		}
	}
	return Quest{}, false
}

// seed is one quest state as name:state:cond[:flags], written as rows.
func seed(name, state, cond string, flags ...string) []Row {
	rows := []Row{{Quest: name, Var: KeyState, Value: state}, {Quest: name, Var: KeyCond, Value: cond}}
	for _, f := range flags {
		rows = append(rows, Row{Quest: name, Var: KeyFlags, Value: f})
	}
	return rows
}

func join(parts ...[]Row) []Row {
	var out []Row
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// TestRestoreResolvesNamesAndSkipsUnknown pins the load rules: a row's name
// resolves case-insensitively to the first matching quest and its state is
// that quest's, an unknown name is reported once per row and skipped while
// the rest loads, a state without a <state> row is created, and a row with
// an empty variable or value adds its quest's state but no variable.
func TestRestoreResolvesNamesAndSkipsUnknown(t *testing.T) {
	var j Journal
	unknown := j.Restore([]Row{
		{Quest: "Gone", Var: KeyCond, Value: "1"},
		{Quest: "Gone", Var: KeyState, Value: "STARTED"},
		{Quest: "q002_whatwomenwant", Var: KeyCond, Value: "2"},
		{Quest: "Q002_WhatWomenWant", Var: KeyState, Value: "STARTED"},
		{Quest: "Q003_WillTheSealBeBroken", Var: KeyCond, Value: "1"},
		{Quest: "Q006_StepIntoTheFuture", Var: KeyState, Value: ""},
		{Quest: "Q006_StepIntoTheFuture", Var: "", Value: "x"},
	}, resolve)
	if want := []string{"Gone", "Gone"}; !reflect.DeepEqual(unknown, want) {
		t.Fatalf("unknown = %v, want %v", unknown, want)
	}
	want := []struct {
		quest Quest
		vars  map[string]string
	}{
		{quests[1], map[string]string{KeyState: "STARTED", KeyCond: "2"}},
		{quests[2], map[string]string{KeyState: "CREATED", KeyCond: "1"}},
		{quests[3], map[string]string{KeyState: "CREATED"}},
	}
	if len(j.states) != len(want) {
		t.Fatalf("states = %d, want %d", len(j.states), len(want))
	}
	for i, w := range want {
		if st := j.states[i]; !reflect.DeepEqual(st.quest, w.quest) || !reflect.DeepEqual(st.vars, w.vars) {
			t.Fatalf("state %d = %+v %v, want %+v %v", i, st.quest, st.vars, w.quest, w.vars)
		}
	}
	if got := j.List(); !reflect.DeepEqual(got, []Entry{{2, -0x7ffffffd}}) {
		t.Fatalf("List() = %v", got)
	}
}

// TestRestoreReplacesJournal pins that a restore starts from an empty
// journal.
func TestRestoreReplacesJournal(t *testing.T) {
	var j Journal
	j.Restore(seed("Q001_LettersOfLove", "STARTED", "1"), resolve)
	j.Restore(nil, resolve)
	if got := j.List(); got != nil {
		t.Fatalf("List() after an empty restore = %v, want none", got)
	}
}

// TestUnreadableReservedValues pins how values the server never writes
// read: a <state> naming no status is neither started nor completed, a
// <cond> that is not an int reads 0, and a <flags> that is not an int
// gives way to the flags the condition gives.
func TestUnreadableReservedValues(t *testing.T) {
	var j Journal
	j.Restore(join(
		seed("Q001_LettersOfLove", "started", "1"),
		seed("Q002_WhatWomenWant", "STARTED", "x"),
		seed("Q003_WillTheSealBeBroken", "STARTED", "2", "y"),
	), resolve)
	if got, want := j.List(), []Entry{{2, 0}, {3, -0x7ffffffd}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %v, want %v", got, want)
	}
}
