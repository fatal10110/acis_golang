// Package scenario reads and runs scenario files: a character's play
// through real client packets against a booted server and MariaDB, step by
// step, with the packets, quest journal statements, inventory, quest rows
// and quest states each step must leave. One generic test per suite
// (tests/quest, tests/ai) runs every scenario file of its testdata
// directory; a scenario needs no Go code of its own.
//
// # Format
//
// Line oriented. Blank lines and lines starting with # are comments. The
// header comes first:
//
//	trace <path below the module root>        optional: the reference trace the first steps replay
//	character <name> <level>                  the character the client plays
//	script <scripts.xml path>                 a script of the suite's catalog; script and stub lines in scripts.xml order
//	stub <scripts.xml path>                   a script not ported yet that a dialog lists: registered from the reference
//	                                          registration manifest (quest id, title, items, bindings) with no hook
//	item <id> ...                             shipped item templates added to the fixture catalog
//	page <path below data/html>               a shipped page the server serves; a path ending in / serves its directory
//	npc <role> <npc id> <type> <dx> <dy> <dz> a fixture NPC template of that instance type, spawned at that offset from the character
//
// Then the steps, each a verb line followed by its indented expectations:
//
//	action <role>          an Action packet on the NPC: the first selects it, the next talks
//	bypass <command>       a RequestBypassToServer; {role} stands for the NPC's object id
//	link <file>            a RequestLinkHtml
//	hit <role> <damage>    the character lands a hit of that damage on the NPC
//	kill <role> [xN]       the character lands a lethal hit on the NPC, N times; a dead NPC is replaced by a fresh one first
//	advance <duration>     time passes, as time.ParseDuration reads it
//	relog                  back to character select and into the world again
//
//	  S <packet>           the packets of the step, in order, as scriptcontract.Packet renders them;
//	                       "page=<path>" ends an NpcHtmlMessage whose text is that page, %objectId% filled in
//	  Q <statement>        the character_quests statements of the step, in order: kind quest | params
//	  items <id>=<n> ...   the character's count of each item after the step
//	  rows <quest> <var>=<value> ... | -    the quest's character_quests rows after the step, all of them
//	  state <quest> <status> | -            the quest's state in memory after the step
//
// The S and Q lists are exact: a step without S lines must send none of
// the rendered packets (relog excepted: its enter burst is not compared),
// and one without Q lines must write no statement. The rendered packets
// are the dialog's, the quest's and the item messages: ActionFailed,
// NpcHtmlMessage, SystemMessage, PlaySound, QuestList and ExShowQuestMark.
//
// A trace header ties the scenario to the reference server: before the run
// the trace's client packets must be the scenario's first steps, and their
// rendered packets and statements must be those steps' S and Q lines.
package scenario

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Scenario is one parsed scenario file.
type Scenario struct {
	// Name is the file name, for messages.
	Name string
	// Trace is the reference trace path below the module root, empty for
	// none.
	Trace     string
	Character string
	Level     int
	Scripts   []ScriptLine
	Items     []int32
	Pages     []string
	NPCs      []NPC
	Steps     []Step
}

// ScriptLine is one script or stub line.
type ScriptLine struct {
	Path string
	Stub bool
}

// NPC is one NPC the scenario spawns.
type NPC struct {
	Role       string
	ID         int32
	Type       string
	DX, DY, DZ int
}

// Step is one client action and what it must leave.
type Step struct {
	// Line is the verb line's number in the file.
	Line int
	Verb string
	Args []string
	// Packets and Statements are the S and Q lines without their prefix.
	Packets    []string
	Statements []string
	Checks     []Check
}

// String is the step as the file writes it.
func (s Step) String() string { return strings.Join(append([]string{s.Verb}, s.Args...), " ") }

// Check is one items, rows or state line.
type Check struct {
	Line int
	Kind string
	Args []string
}

// argCount is, per verb, the arguments it takes: min and max.
var argCount = map[string][2]int{
	"action":  {1, 1},
	"bypass":  {1, -1},
	"link":    {1, 1},
	"hit":     {2, 2},
	"kill":    {1, 2},
	"advance": {1, 1},
	"relog":   {0, 0},
}

// Parse reads a scenario file.
func Parse(name string, data []byte) (*Scenario, error) {
	sc := &Scenario{Name: name}
	roles := map[string]bool{}
	sb := bufio.NewScanner(bytes.NewReader(data))
	sb.Buffer(make([]byte, 1<<20), 1<<20)
	for n := 1; sb.Scan(); n++ {
		raw := sb.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fail := func(format string, args ...any) error {
			return fmt.Errorf("%s:%d: %s", name, n, fmt.Sprintf(format, args...))
		}
		if raw[0] == ' ' || raw[0] == '\t' {
			if len(sc.Steps) == 0 {
				return nil, fail("expectation %q before any step", line)
			}
			if err := sc.Steps[len(sc.Steps)-1].expect(n, line); err != nil {
				return nil, fail("%v", err)
			}
			continue
		}
		fields := strings.Fields(line)
		verb, args := fields[0], fields[1:]
		if _, isStep := argCount[verb]; isStep || len(sc.Steps) > 0 {
			c, ok := argCount[verb]
			if !ok {
				return nil, fail("unknown step %q", verb)
			}
			if len(args) < c[0] || (c[1] >= 0 && len(args) > c[1]) {
				return nil, fail("%s takes %d to %d arguments, got %d", verb, c[0], c[1], len(args))
			}
			if err := checkStep(verb, args, roles); err != nil {
				return nil, fail("%v", err)
			}
			if verb == "bypass" {
				// The command keeps its own spacing.
				args = []string{strings.TrimSpace(strings.TrimPrefix(line, verb))}
			}
			sc.Steps = append(sc.Steps, Step{Line: n, Verb: verb, Args: args})
			continue
		}
		if err := sc.header(verb, args, roles); err != nil {
			return nil, fail("%v", err)
		}
	}
	if err := sb.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if sc.Character == "" {
		return nil, fmt.Errorf("%s: no character line", name)
	}
	if len(sc.Steps) == 0 {
		return nil, fmt.Errorf("%s: no step", name)
	}
	return sc, nil
}

func (sc *Scenario) header(key string, args []string, roles map[string]bool) error {
	switch key {
	case "trace":
		if len(args) != 1 || sc.Trace != "" {
			return fmt.Errorf("one trace line with one path")
		}
		sc.Trace = args[0]
	case "character":
		if len(args) != 2 || sc.Character != "" {
			return fmt.Errorf("one character line: character <name> <level>")
		}
		level, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("level %q: %w", args[1], err)
		}
		sc.Character, sc.Level = args[0], level
	case "script", "stub":
		if len(args) != 1 {
			return fmt.Errorf("%s <path>", key)
		}
		for _, l := range sc.Scripts {
			if l.Path == args[0] {
				return fmt.Errorf("script %s listed twice", args[0])
			}
		}
		sc.Scripts = append(sc.Scripts, ScriptLine{Path: args[0], Stub: key == "stub"})
	case "item":
		if len(args) == 0 {
			return fmt.Errorf("item takes one or more ids")
		}
		for _, a := range args {
			id, err := strconv.ParseInt(a, 10, 32)
			if err != nil {
				return fmt.Errorf("item id %q: %w", a, err)
			}
			sc.Items = append(sc.Items, int32(id))
		}
	case "page":
		if len(args) != 1 {
			return fmt.Errorf("page <path>")
		}
		sc.Pages = append(sc.Pages, args[0])
	case "npc":
		if len(args) != 6 {
			return fmt.Errorf("npc <role> <npc id> <type> <dx> <dy> <dz>")
		}
		id, err := strconv.ParseInt(args[1], 10, 32)
		if err != nil {
			return fmt.Errorf("npc number %q: %w", args[1], err)
		}
		var offsets [3]int
		for i, a := range args[3:] {
			v, err := strconv.Atoi(a)
			if err != nil {
				return fmt.Errorf("npc number %q: %w", a, err)
			}
			offsets[i] = v
		}
		role := args[0]
		if roles[role] || role == "player" {
			return fmt.Errorf("role %q named twice", role)
		}
		roles[role] = true
		sc.NPCs = append(sc.NPCs, NPC{Role: role, ID: int32(id), Type: args[2], DX: offsets[0], DY: offsets[1], DZ: offsets[2]})
	default:
		return fmt.Errorf("unknown header line %q", key)
	}
	return nil
}

// checkStep validates a step's arguments against the declared roles.
func checkStep(verb string, args []string, roles map[string]bool) error {
	switch verb {
	case "action", "hit", "kill":
		if !roles[args[0]] {
			return fmt.Errorf("unknown role %q", args[0])
		}
	}
	switch verb {
	case "hit":
		if _, err := strconv.Atoi(args[1]); err != nil {
			return fmt.Errorf("damage %q: %w", args[1], err)
		}
	case "kill":
		if len(args) == 2 {
			if _, err := killCount(args[1]); err != nil {
				return err
			}
		}
	case "advance":
		if _, err := time.ParseDuration(args[0]); err != nil {
			return err
		}
	}
	return nil
}

// killCount reads a kill step's xN.
func killCount(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(s, "x"))
	if err != nil || !strings.HasPrefix(s, "x") || n < 1 {
		return 0, fmt.Errorf("kill count %q is not xN", s)
	}
	return n, nil
}

// expect adds one indented line to the step.
func (s *Step) expect(n int, line string) error {
	kind, rest, _ := strings.Cut(line, " ")
	switch kind {
	case "S":
		s.Packets = append(s.Packets, rest)
	case "Q":
		s.Statements = append(s.Statements, rest)
	case "items", "rows", "state":
		args := strings.Fields(rest)
		if err := checkCheck(kind, args); err != nil {
			return err
		}
		s.Checks = append(s.Checks, Check{Line: n, Kind: kind, Args: args})
	default:
		return fmt.Errorf("unknown expectation %q", kind)
	}
	return nil
}

func checkCheck(kind string, args []string) error {
	switch kind {
	case "items":
		if len(args) == 0 {
			return fmt.Errorf("items takes one or more <id>=<count>")
		}
		for _, a := range args {
			id, count, ok := strings.Cut(a, "=")
			if _, err := strconv.ParseInt(id, 10, 32); !ok || err != nil {
				return fmt.Errorf("item %q is not <id>=<count>", a)
			}
			if _, err := strconv.Atoi(count); err != nil {
				return fmt.Errorf("item %q is not <id>=<count>", a)
			}
		}
	case "rows":
		if len(args) < 2 {
			return fmt.Errorf("rows <quest> <var>=<value> ... | -")
		}
		if len(args) == 2 && args[1] == "-" {
			return nil
		}
		for _, a := range args[1:] {
			if _, _, ok := strings.Cut(a, "="); !ok {
				return fmt.Errorf("row %q is not <var>=<value>", a)
			}
		}
	case "state":
		if len(args) != 2 {
			return fmt.Errorf("state <quest> <status> | -")
		}
	}
	return nil
}

// FirstDiff compares a step's got lines with its wanted ones and describes
// the first difference: the first line that differs, or the first one
// missing or extra; "" when they are equal.
func FirstDiff(what string, got, want []string) string {
	for i := range max(len(got), len(want)) {
		switch {
		case i >= len(got):
			return fmt.Sprintf("%s %d missing:\n want %s", what, i+1, want[i])
		case i >= len(want):
			return fmt.Sprintf("%s %d not expected:\n  got %s", what, i+1, got[i])
		case got[i] != want[i]:
			return fmt.Sprintf("%s %d differs:\n  got %s\n want %s", what, i+1, got[i], want[i])
		}
	}
	return ""
}
