// Package questlog holds a player's quest journal: one state per quest the
// player has touched, each a map of string variables saved as
// character_quests rows.
package questlog

import (
	"strconv"
	"sync"
)

// The reserved variables every quest state may carry.
const (
	// KeyState holds the quest's Status by name.
	KeyState = "<state>"
	// KeyCond holds the quest's condition: the step the client's quest
	// window shows.
	KeyCond = "<cond>"
	// KeyFlags holds explicit quest window flags, set when a quest skips
	// or goes back a step.
	KeyFlags = "<flags>"
)

// Status is where a player stands on a quest.
type Status uint8

// The quest statuses. The zero value is no valid status: a <state> value
// that names none of them.
const (
	StatusCreated Status = iota + 1
	StatusStarted
	StatusCompleted
)

// String returns the status as its <state> value is written.
func (s Status) String() string {
	switch s {
	case StatusCreated:
		return "CREATED"
	case StatusStarted:
		return "STARTED"
	case StatusCompleted:
		return "COMPLETED"
	default:
		return ""
	}
}

// parseStatus reads a <state> value, which must match a status name
// exactly.
func parseStatus(v string) Status {
	for _, s := range []Status{StatusCreated, StatusStarted, StatusCompleted} {
		if v == s.String() {
			return s
		}
	}
	return 0
}

// Quest identifies the script a quest state belongs to.
type Quest struct {
	// Name is the script's name, which keys its character_quests rows.
	Name string
	// ID is the quest id; only a positive id is a real quest, shown in the
	// client's quest window.
	ID int32
	// Items are the item ids the quest takes from the player when it ends;
	// shared with the script, never modified.
	Items []int32
}

// Real reports whether q is a real quest.
func (q Quest) Real() bool { return q.ID > 0 }

// Row is one saved character_quests row of a player.
type Row struct {
	// Quest is the row's quest name as saved.
	Quest string
	Var   string
	// Value is "" for a NULL value.
	Value string
}

// Entry is one line of the client's quest window.
type Entry struct {
	QuestID int32
	Flags   int32
}

// Journal is one player's quest states, in the order they were added, and
// the character_quests writes its changes still owe the database. The zero
// value is an empty journal. mu guards every field and every state's
// variables, so any goroutine may use it; it is a leaf lock.
type Journal struct {
	mu     sync.Mutex
	states []*State
	// pending are the writes not yet handed to a drain, in the order the
	// changes were made.
	pending []Write
	// scheduled is set while a drain is owed for pending: from the change
	// that asked for one until that drain takes the writes.
	scheduled bool
	// inflight is set while a drain applies the writes it took.
	inflight bool
	// sealed refuses every further change: the player is leaving.
	sealed bool
}

// State is one quest's variables in a journal. Its journal's lock guards
// them. A state an exit took out of its journal keeps working on its own:
// its changes are still written, as the reference writes them.
type State struct {
	j     *Journal
	quest Quest
	vars  map[string]string
}

func newState(j *Journal, q Quest) *State {
	return &State{j: j, quest: q, vars: map[string]string{KeyState: StatusCreated.String()}}
}

// Restore replaces the journal with the states rows describe, in row order.
// resolve names the quest a row's quest name belongs to; a row it does not
// resolve is skipped, and its quest name is returned, once per row, for the
// caller to report. The first row of a quest adds its state, created, ahead
// of the row's own variable. A row with an empty variable or value adds no
// variable, though it still adds its quest's state.
func (j *Journal) Restore(rows []Row, resolve func(name string) (Quest, bool)) (unknown []string) {
	var states []*State
	for _, r := range rows {
		q, ok := resolve(r.Quest)
		if !ok {
			unknown = append(unknown, r.Quest)
			continue
		}
		st := find(states, q.Name)
		if st == nil {
			st = newState(j, q)
			states = append(states, st)
		}
		if r.Var == "" || r.Value == "" {
			continue
		}
		st.vars[r.Var] = r.Value
	}
	j.mu.Lock()
	j.states = states
	j.mu.Unlock()
	return unknown
}

// find returns the state of the quest named name, nil when there is none.
func find(states []*State, name string) *State {
	for _, st := range states {
		if st.quest.Name == name {
			return st
		}
	}
	return nil
}

// List returns the client's quest window: every real quest that is started
// or completed, in journal order, with its flags.
func (j *Journal) List() []Entry {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.list()
}

// list is List with mu held.
func (j *Journal) list() []Entry {
	var out []Entry
	for _, st := range j.states {
		if !st.quest.Real() {
			continue
		}
		if s := st.status(); s != StatusStarted && s != StatusCompleted {
			continue
		}
		out = append(out, Entry{QuestID: st.quest.ID, Flags: st.flags()})
	}
	return out
}

// status is the state's <state>.
func (st *State) status() Status { return parseStatus(st.vars[KeyState]) }

// flags is the state's <flags> when set, otherwise the flags its <cond>
// gives.
func (st *State) flags() int32 {
	if v, ok := st.int(KeyFlags); ok {
		return v
	}
	cond, _ := st.int(KeyCond)
	return condFlags(cond)
}

// condFlags is the quest window flags condition cond gives: none for no
// condition, else every step up to it plus the high bit.
func condFlags(cond int32) int32 {
	if cond == 0 {
		return 0
	}
	return (int32(1)<<(uint32(cond)&31) - 1) | -0x80000000
}

// int reads the variable key as a 32-bit decimal integer; ok is false when
// it is unset or not such an integer.
func (st *State) int(key string) (int32, bool) {
	v, ok := st.vars[key]
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return 0, false
	}
	return int32(n), true
}
