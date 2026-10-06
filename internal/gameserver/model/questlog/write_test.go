package questlog

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// seeded returns a journal holding one state of q with vars, put there
// without writing anything, as a load leaves it.
func seeded(q Quest, vars map[string]string) (*Journal, *State) {
	j := &Journal{}
	st := j.Create(q)
	clear(st.vars)
	for k, v := range vars {
		st.vars[k] = v
	}
	return j, st
}

// TestSetCondMatchesReferenceFlags runs the journal.cond_flags golden: the
// <cond> and <flags> a condition change leaves, and the flags the quest
// window then shows.
func TestSetCondMatchesReferenceFlags(t *testing.T) {
	scriptcontract.Run(t, "journal.cond_flags", func(t *testing.T, r scriptcontract.Row) {
		vars := map[string]string{KeyState: "STARTED"}
		if c := r.Str(t, "from_cond"); c != "0" {
			vars[KeyCond] = c
		}
		if f := r.Str(t, "from_flags"); f != "-" {
			vars[KeyFlags] = f
		}
		j, st := seeded(quests[0], vars)
		_, changed := st.SetCond(int32(r.Int(t, "to")))
		if want := r.Str(t, "from_cond") != r.Str(t, "to"); changed != want {
			t.Fatalf("changed = %v, want %v", changed, want)
		}
		got := func(key string) string {
			if v, ok := st.vars[key]; ok {
				return v
			}
			return "-"
		}
		if got(KeyCond) != r.Str(t, "cond") || got(KeyFlags) != r.Str(t, "flags_var") {
			t.Fatalf("<cond>=%s <flags>=%s, want %s %s", got(KeyCond), got(KeyFlags), r.Str(t, "cond"), r.Str(t, "flags_var"))
		}
		if flags := fmt.Sprintf("0x%08x", uint32(j.states[0].flags())); flags != r.Str(t, "get_flags") {
			t.Fatalf("flags = %s, want %s", flags, r.Str(t, "get_flags"))
		}
	})
}

// TestFlagsFromCondMatchReference runs the journal.get_flags golden: the
// flags a condition gives when no <flags> is kept.
func TestFlagsFromCondMatchReference(t *testing.T) {
	scriptcontract.Run(t, "journal.get_flags", func(t *testing.T, r scriptcontract.Row) {
		_, st := seeded(quests[0], map[string]string{KeyState: "STARTED", KeyCond: r.Str(t, "cond")})
		if got := fmt.Sprintf("0x%08x", uint32(st.flags())); got != r.Str(t, "get_flags") {
			t.Fatalf("flags = %s, want %s", got, r.Str(t, "get_flags"))
		}
	})
}

// TestListMatchesQuestWindowContents runs the questlist.packet golden: the
// quest window a seeded journal shows lists real quests started or
// completed, in journal order, with explicit or computed flags.
func TestListMatchesQuestWindowContents(t *testing.T) {
	scriptcontract.Run(t, "questlist.packet", func(t *testing.T, r scriptcontract.Row) {
		var rows []Row
		if seed := r.Str(t, "seed"); seed != "-" {
			for _, s := range strings.Split(seed, ",") {
				f := strings.Split(s, ":")
				rows = append(rows, Row{Quest: f[0], Var: KeyState, Value: f[1]}, Row{Quest: f[0], Var: KeyCond, Value: f[2]})
				if len(f) > 3 {
					rows = append(rows, Row{Quest: f[0], Var: KeyFlags, Value: f[3]})
				}
			}
		}
		var j Journal
		if unknown := j.Restore(rows, resolve); len(unknown) != 0 {
			t.Fatalf("unknown = %v", unknown)
		}
		if got, want := questListLine(j.List()), r.Lines[0]; got != want {
			t.Fatalf("list = %q, want %q", got, want)
		}
	})
}

// questListLine renders a quest window as the goldens write a QuestList.
func questListLine(entries []Entry) string {
	var sb strings.Builder
	sb.WriteString("S QuestList")
	for _, e := range entries {
		fmt.Fprintf(&sb, " %d:0x%08x", e.QuestID, uint32(e.Flags))
	}
	return sb.String()
}

// TestWritesMatchReferenceStatements runs the journal.write_order golden at
// the journal: each operation owes the statements the reference executes,
// in order, leaves the state the reference leaves, and returns the quest
// window the reference's QuestList then shows. Which packets go out, and
// when, is the engine's (tests/quest).
func TestWritesMatchReferenceStatements(t *testing.T) {
	scriptcontract.Run(t, "journal.write_order", func(t *testing.T, r scriptcontract.Row) {
		q := Quest{Name: "Q001_LettersOfLove", ID: 1}
		if r.Str(t, "quest") == "script" {
			q = Quest{Name: "NoblesseTeleporter", ID: -1}
		}
		j := &Journal{}
		var st *State
		if seedState := r.Str(t, "seed_state"); seedState != "-" {
			vars := map[string]string{KeyState: seedState}
			if c := r.Str(t, "seed_cond"); c != "0" {
				vars[KeyCond] = c
			}
			if f := r.Str(t, "seed_flags"); f != "-" {
				vars[KeyFlags] = f
			}
			if seedState == "STARTED" {
				vars["ex"] = "1"
			}
			j, st = seeded(q, vars)
		}

		var list []Entry
		listed := false
		op := strings.Fields(r.Str(t, "op"))
		switch op[0] {
		case "set":
			st.Set(op[1], op[2])
		case "unset":
			st.Unset(op[1])
		case "setState":
			st.SetStatus(parseStatus(op[1]))
		case "setCond":
			n, _ := strconv.Atoi(op[1])
			list, listed = st.SetCond(int32(n))
		case "exitQuest":
			list, listed = st.Exit(op[1] == "true")
		case "newQuestState":
			j.Create(q)
		default:
			t.Fatalf("op %q", op)
		}

		var wantQ, wantList []string
		for _, line := range r.Lines {
			switch {
			case strings.HasPrefix(line, "Q "):
				s, err := scriptcontract.ParseStatement(line)
				if err != nil {
					t.Fatal(err)
				}
				wantQ = append(wantQ, string(s.Kind)+" "+strings.Join(s.Params[1:], " | "))
			case strings.HasPrefix(line, "S QuestList"):
				wantList = append(wantList, line)
			}
		}
		var gotQ []string
		for _, w := range j.TakePending() {
			gotQ = append(gotQ, statement(w))
		}
		if !slices.Equal(gotQ, wantQ) {
			t.Fatalf("statements:\n got %q\nwant %q", gotQ, wantQ)
		}
		// The reference sends QuestList only for a real quest; the window
		// the change leaves is the same either way.
		if listed && q.Real() {
			if got := []string{questListLine(list)}; !slices.Equal(got, wantList) {
				t.Fatalf("quest window = %q, want %q", got, wantList)
			}
		} else if len(wantList) != 0 {
			t.Fatalf("no quest window returned, want %q", wantList)
		}

		after := j.State(q.Name)
		state, vars := "-", "-"
		if after != nil {
			if v, ok := after.vars[KeyState]; ok {
				state = v
			}
			var kv []string
			for k, v := range after.vars {
				kv = append(kv, k+":"+v)
			}
			slices.Sort(kv)
			if len(kv) > 0 {
				vars = strings.Join(kv, ",")
			}
		}
		if state != r.Str(t, "state") || vars != r.Str(t, "vars") {
			t.Fatalf("after: state=%s vars=%s, want %s %s", state, vars, r.Str(t, "state"), r.Str(t, "vars"))
		}
	})
}

// statement renders a write as the golden's statement kind and parameters
// after the player id.
func statement(w Write) string {
	switch w.Op {
	case OpSet:
		return fmt.Sprintf("upsert %s | %s | %s", w.Quest, w.Var, w.Value)
	case OpUnset:
		return fmt.Sprintf("delete-var %s | %s", w.Quest, w.Var)
	case OpDelete:
		return "delete-quest " + w.Quest
	case OpComplete:
		return "delete-except-state " + w.Quest
	}
	return fmt.Sprintf("op %d", w.Op)
}

// TestRepeatableExitRemovesOnlyRealQuests pins how a repeatable exit leaves
// the journal: a real quest's first state of its id leaves it, while a
// script that is not a real quest keeps its state, with no variables.
func TestRepeatableExitRemovesOnlyRealQuests(t *testing.T) {
	j := &Journal{}
	real := j.Create(quests[0])
	script := j.Create(quests[4])
	real.vars[KeyState], script.vars[KeyState] = "STARTED", "STARTED"
	if _, ok := script.Exit(true); !ok {
		t.Fatal("script exit refused")
	}
	if j.State(quests[4].Name) != script || len(script.vars) != 0 {
		t.Fatalf("script state after exit: %v, vars %v", j.State(quests[4].Name), script.vars)
	}
	if _, ok := real.Exit(true); !ok {
		t.Fatal("quest exit refused")
	}
	if j.State(quests[0].Name) != nil {
		t.Fatal("real quest still in the journal")
	}
	// An exited state still writes what a script sets on it.
	real.Set("ex", "2")
	if got := j.TakePending(); !reflect.DeepEqual(got[len(got)-1], Write{Op: OpSet, Quest: quests[0].Name, Var: "ex", Value: "2"}) {
		t.Fatalf("last write = %+v", got[len(got)-1])
	}
}

// TestSealRefusesChanges pins that a sealed journal takes no change: the
// player is leaving.
func TestSealRefusesChanges(t *testing.T) {
	j, st := seeded(quests[0], map[string]string{KeyState: "STARTED", KeyCond: "1"})
	j.Seal()
	if st.Set("ex", "1") || st.Unset(KeyCond) || st.SetStatus(StatusCompleted) {
		t.Fatal("sealed journal took a change")
	}
	if _, ok := st.SetCond(2); ok {
		t.Fatal("sealed journal took a condition")
	}
	if _, ok := st.Exit(false); ok {
		t.Fatal("sealed journal took an exit")
	}
	if j.Dirty() || !reflect.DeepEqual(st.vars, map[string]string{KeyState: "STARTED", KeyCond: "1"}) {
		t.Fatalf("dirty=%v vars=%v", j.Dirty(), st.vars)
	}
}

// TestDrainBookkeeping pins the pending writes' life: one drain is owed per
// batch of changes, a drain takes them in order, writes it failed to apply
// go back ahead of later ones, and the journal is dirty until a drain
// reports them applied.
func TestDrainBookkeeping(t *testing.T) {
	j, st := seeded(quests[0], map[string]string{KeyState: "STARTED"})
	if j.ScheduleDrain() || j.Dirty() {
		t.Fatal("clean journal asked for a drain")
	}
	st.Set("a", "1")
	st.Set("b", "2")
	if !j.ScheduleDrain() || j.ScheduleDrain() {
		t.Fatal("want exactly one drain owed")
	}
	first := j.TakePending()
	if len(first) != 2 || !j.Dirty() {
		t.Fatalf("took %v, dirty=%v", first, j.Dirty())
	}
	st.Set("c", "3")
	if !j.ScheduleDrain() {
		t.Fatal("a change after the take must ask for its own drain")
	}
	j.Applied(first, false)
	got := j.TakePending()
	want := []Write{first[0], first[1], {Op: OpSet, Quest: quests[0].Name, Var: "c", Value: "3"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("after a failed drain: %+v, want %+v", got, want)
	}
	j.Applied(got, true)
	if j.Dirty() {
		t.Fatal("dirty after every write applied")
	}
}
