package questlog

import (
	"maps"
	"math"
	"strconv"
)

// Op is what one journal write does to the owner's character_quests rows.
type Op uint8

// The journal writes.
const (
	// OpSet inserts one variable's row, or replaces its value.
	OpSet Op = iota + 1
	// OpUnset deletes one variable's row.
	OpUnset
	// OpDelete deletes every row of the quest.
	OpDelete
	// OpComplete deletes every row of the quest but its <state>.
	OpComplete
)

// Write is one character_quests statement a journal change owes the
// database.
type Write struct {
	Op    Op
	Quest string
	// Var is the variable OpSet and OpUnset write.
	Var string
	// Value is the value OpSet writes.
	Value string
}

// State returns the first state of the quest named name, nil when there is
// none.
func (j *Journal) State(name string) *State {
	j.mu.Lock()
	defer j.mu.Unlock()
	return find(j.states, name)
}

// StateByID returns the first state whose quest has id, nil when there is
// none.
func (j *Journal) StateByID(id int32) *State {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, st := range j.states {
		if st.quest.ID == id {
			return st
		}
	}
	return nil
}

// Create adds a state of q, created, at the end of the journal and returns
// it. It writes nothing and, as in the reference, does not look for a state
// q already has.
func (j *Journal) Create(q Quest) *State {
	j.mu.Lock()
	defer j.mu.Unlock()
	st := newState(j, q)
	j.states = append(j.states, st)
	return st
}

// Seal refuses every later change to the journal and its states: the player
// is leaving, and the writes it owes are its last.
func (j *Journal) Seal() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.sealed = true
}

// Dirty reports whether the journal owes the database writes: writes no
// drain has taken, or writes a drain took and has not yet reported applied.
func (j *Journal) Dirty() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.pending) > 0 || j.inflight
}

// ScheduleDrain reports whether the caller must start a drain: the journal
// owes writes and no drain is owed for them yet. It then counts the caller's
// drain as owed until that drain takes the writes, so each change starts at
// most one.
func (j *Journal) ScheduleDrain() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(j.pending) == 0 || j.scheduled {
		return false
	}
	j.scheduled = true
	return true
}

// TakePending hands the writes the journal owes to a drain, in the order
// the changes were made, and clears the owed drain: a change made from here
// on asks for a drain of its own. The drain reports back with Applied.
func (j *Journal) TakePending() []Write {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := j.pending
	j.pending = nil
	j.scheduled = false
	j.inflight = len(out) > 0
	return out
}

// Applied reports how a drain ended with the writes TakePending gave it.
// Writes that were not applied go back ahead of any made since, so the next
// drain applies them first.
func (j *Journal) Applied(writes []Write, ok bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.inflight = false
	if !ok && len(writes) > 0 {
		j.pending = append(append([]Write(nil), writes...), j.pending...)
	}
}

// Quest returns the quest st belongs to.
func (st *State) Quest() Quest { return st.quest }

// Get returns st's variable key.
func (st *State) Get(key string) (string, bool) {
	st.j.mu.Lock()
	defer st.j.mu.Unlock()
	v, ok := st.vars[key]
	return v, ok
}

// Vars returns a copy of st's variables.
func (st *State) Vars() map[string]string {
	st.j.mu.Lock()
	defer st.j.mu.Unlock()
	return maps.Clone(st.vars)
}

// Status returns st's <state>; the zero Status when it names none.
func (st *State) Status() Status {
	st.j.mu.Lock()
	defer st.j.mu.Unlock()
	return st.status()
}

// Set sets st's variable key to value and writes it, even when the value is
// unchanged. It reports false, changing nothing, once the journal is sealed.
func (st *State) Set(key, value string) bool {
	st.j.mu.Lock()
	defer st.j.mu.Unlock()
	if st.j.sealed {
		return false
	}
	st.set(key, value)
	return true
}

// Unset removes st's variable key and deletes its row, even when st has no
// such variable. It reports false, changing nothing, once the journal is
// sealed.
func (st *State) Unset(key string) bool {
	st.j.mu.Lock()
	defer st.j.mu.Unlock()
	if st.j.sealed {
		return false
	}
	st.unset(key)
	return true
}

// SetStatus sets st's <state> to s and writes it. It reports false, changing
// nothing, when st already has s or the journal is sealed.
func (st *State) SetStatus(s Status) bool {
	st.j.mu.Lock()
	defer st.j.mu.Unlock()
	if st.j.sealed || st.status() == s {
		return false
	}
	st.set(KeyState, s.String())
	return true
}

// SetCond sets st's <cond> to cond. When it skips steps forward, or the
// quest already carries <flags>, the quest window flags are kept in <flags>
// first: a step skipped forward adds its bit, a step back masks the higher
// bits off, and a step back to 2 or lower drops <flags>. It returns the
// quest window as the change leaves it. It reports false, changing nothing,
// when cond is st's condition already or the journal is sealed.
func (st *State) SetCond(cond int32) ([]Entry, bool) {
	st.j.mu.Lock()
	defer st.j.mu.Unlock()
	previous, _ := st.int(KeyCond)
	if st.j.sealed || cond == previous {
		return nil, false
	}
	// A <flags> of 0 is no flags. Shifts take the count mod 32, as int
	// shifts do in the reference.
	flags, _ := st.int(KeyFlags)
	switch {
	case flags == 0:
		if previous != 0 && cond > previous+1 {
			flags = condFlags(previous) | int32(1)<<(uint32(cond-1)&31)
			st.set(KeyFlags, strconv.Itoa(int(flags)))
		}
	case cond > previous:
		flags |= int32(1) << (uint32(cond-1) & 31)
		st.set(KeyFlags, strconv.Itoa(int(flags)))
	case cond > 2:
		flags &= int32(1)<<(uint32(cond)&31) - 1
		flags |= math.MinInt32
		st.set(KeyFlags, strconv.Itoa(int(flags)))
	default:
		st.unset(KeyFlags)
	}
	st.set(KeyCond, strconv.Itoa(int(cond)))
	return st.j.list(), true
}

// Exit ends a started quest: its variables are cleared, then a repeatable
// real quest leaves the journal, while any other is kept completed. It
// returns the quest window as the exit leaves it. It reports false,
// changing nothing, when st is not started or the journal is sealed.
//
// A repeatable exit takes out the first state of st's quest id, and only
// for a real quest: the reference matches states by quest id and never
// matches a script that is not a real quest, whose state stays in the
// journal with no variables while its rows are deleted.
func (st *State) Exit(repeatable bool) ([]Entry, bool) {
	st.j.mu.Lock()
	defer st.j.mu.Unlock()
	if st.j.sealed || st.status() != StatusStarted {
		return nil, false
	}
	clear(st.vars)
	if repeatable {
		st.j.removeQuest(st.quest)
		st.j.pending = append(st.j.pending, Write{Op: OpDelete, Quest: st.quest.Name})
	} else {
		st.set(KeyState, StatusCompleted.String())
		st.j.pending = append(st.j.pending, Write{Op: OpComplete, Quest: st.quest.Name})
	}
	return st.j.list(), true
}

// removeQuest takes the first state of q's quest id out of the journal,
// when q is a real quest.
func (j *Journal) removeQuest(q Quest) {
	if !q.Real() {
		return
	}
	for i, other := range j.states {
		if other.quest.ID == q.ID {
			j.states = append(j.states[:i:i], j.states[i+1:]...)
			return
		}
	}
}

// set stores and writes one variable; the journal's lock is held.
func (st *State) set(key, value string) {
	st.vars[key] = value
	st.j.pending = append(st.j.pending, Write{Op: OpSet, Quest: st.quest.Name, Var: key, Value: value})
}

// unset removes and deletes one variable; the journal's lock is held.
func (st *State) unset(key string) {
	delete(st.vars, key)
	st.j.pending = append(st.j.pending, Write{Op: OpUnset, Quest: st.quest.Name, Var: key})
}

// Started returns how many real quests of the journal are started.
func (j *Journal) Started() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	n := 0
	for _, st := range j.states {
		if st.quest.Real() && st.status() == StatusStarted {
			n++
		}
	}
	return n
}
