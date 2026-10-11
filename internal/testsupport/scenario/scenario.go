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
//	character <name> <level>                  the character the client plays, with the experience its level starts at
//	class <class id>                          optional: the character's class, and so its race (default 0, a human
//	                                          fighter); the class template keeps the fixture's stats
//	clan <level>                              optional: the character leads a clan of that level
//	quest <quest> <var>=<value> ...           character_quests rows of the character before it enters the world,
//	                                          one line per quest, as a rows check lists them
//	random <draw> ...                         optional: the script random draws, in order; each must be inside
//	                                          [0, n) of its call, and every one must be drawn
//	random always <draw>                      optional: every script random draw is that one
//	script <scripts.xml path>                 a script of the suite's catalog; script and stub lines in scripts.xml order
//	stub <scripts.xml path>                   a script not ported yet that a dialog lists: registered from the reference
//	                                          registration manifest (quest id, title, items, bindings) with no hook
//	item <id> ...                             shipped item templates added to the fixture catalog
//	page <path below data/html>               a shipped page the server serves; a path ending in / serves its directory
//	npc <role> <npc id> <type> <dx> <dy> <dz> a fixture NPC template of that instance type, spawned at that offset from the character;
//	                                          the templates a script may spawn more NPCs of
//
// Then the steps, each a verb line followed by its indented expectations:
//
//	action <role>          an Action packet on the NPC: the first selects it, the next talks
//	bypass <command>       a RequestBypassToServer; {role} stands for the NPC's object id
//	link <file>            a RequestLinkHtml
//	hit <role> <damage>    the character lands a hit of that damage on the NPC
//	kill <role> [xN]       the character lands a lethal hit on the NPC, N times; a dead NPC is replaced by a fresh one first
//	advance <duration>     time passes, as time.ParseDuration reads it
//	approach <role>        the character steps to 100 in front of the NPC, inside its see range, and one
//	                       AI tick runs, whose see-creature scan notices it
//	pickup <item id>       an Action packet on the one ground item of that template, then time passes
//	                       until the item has left the ground
//	mark <id>              a RequestTutorialQuestionMark: the character clicks that tutorial question mark
//	relog                  back to character select and into the world again
//	restart                a RequestRestart: back to character select
//	enter                  the character is selected and enters the world (the EnterWorld burst)
//	create                 the first step only: a RequestCharacterCreate of a human fighter named as the
//	                       character line says, instead of a seeded character; the character line's level
//	                       must be 1, and the class, clan and quest lines are left out (the creation
//	                       writes its own rows)
//
//	  S <packet>           the packets of the step, in order, as scriptcontract.Packet renders them;
//	                       "page=<path>" ends an NpcHtmlMessage whose text is that page, %objectId% filled in
//	  Q <statement>        the character_quests statements of the step, in order: kind quest | params
//	  items <id>=<n> ...   the character's count of each item after the step
//	  rows <quest> <var>=<value> ... | -    the quest's character_quests rows after the step, all of them
//	  state <quest> <status> | -            the quest's state in memory after the step
//
// A step other than create and enter acts in the world: a scenario starting
// with create, or a restart, is followed by an enter before any of them. The
// NPC lines' NPCs are spawned when the character first enters the world.
// The items and state checks read the character in the world, so a create
// or restart step takes rows checks only.
//
// The S and Q lists are exact: a step without S lines must send none of
// the rendered packets (relog excepted: its enter burst is not compared),
// and one without Q lines must write no statement. The rendered packets
// are the dialog's, the quest's, the tutorial's and the item messages:
// ActionFailed, NpcHtmlMessage, SystemMessage, PlaySound, QuestList,
// ExShowQuestMark, TutorialShowHtml, TutorialShowQuestionMark,
// TutorialEnableClientEvent, TutorialCloseHtml, RadarControl, DropItem and
// MagicSkillUse. "page=<path>" also ends a TutorialShowHtml line.
//
// Without a random line the script draws are random. A draw outside
// [0, n), or past the listed ones, fails the step that drew it.
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

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
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
	// Class is the character's class id.
	Class int
	// Clan is the level of the clan the character leads, -1 for none.
	Clan int
	// Quests are the character_quests rows seeded before the character
	// enters the world.
	Quests []QuestRows
	// Random is the script random source, nil for random draws.
	Random  *Random
	Scripts []ScriptLine
	Items   []int32
	Pages   []string
	NPCs    []NPC
	Steps   []Step
}

// QuestRows is one quest line: a quest's rows as var=value.
type QuestRows struct {
	Quest string
	Vars  [][2]string
}

// Random is the random line: the draws in order, or one draw always.
type Random struct {
	Draws  []int
	Always bool
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

// maxClanLevel is the highest clan level.
const maxClanLevel = 8

// argCount is, per verb, the arguments it takes: min and max.
var argCount = map[string][2]int{
	"action":   {1, 1},
	"bypass":   {1, -1},
	"link":     {1, 1},
	"hit":      {2, 2},
	"kill":     {1, 2},
	"advance":  {1, 1},
	"approach": {1, 1},
	"pickup":   {1, 1},
	"mark":     {1, 1},
	"relog":    {0, 0},
	"restart":  {0, 0},
	"enter":    {0, 0},
	"create":   {0, 0},
}

// Parse reads a scenario file.
func Parse(name string, data []byte) (*Scenario, error) {
	sc := &Scenario{Name: name, Clan: -1}
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
	if err := sc.checkFlow(); err != nil {
		return nil, fmt.Errorf("%s:%w", name, err)
	}
	return sc, nil
}

// Creates reports whether the scenario creates its character through the
// client: its first step is create.
func (sc *Scenario) Creates() bool { return sc.Steps[0].Verb == "create" }

// checkFlow requires each step to find the character where it acts: create
// first and only on a character no precondition line seeds, enter at the
// character select, every other step in the world.
func (sc *Scenario) checkFlow() error {
	inWorld := true
	for i, st := range sc.Steps {
		fail := func(format string, args ...any) error {
			return fmt.Errorf("%d: %s", st.Line, fmt.Sprintf(format, args...))
		}
		switch st.Verb {
		case "create":
			if i != 0 {
				return fail("create is the first step only")
			}
			switch {
			case sc.Level != 1:
				return fail("a created character starts at level 1, not %d", sc.Level)
			case sc.Class != 0 || sc.Clan >= 0 || len(sc.Quests) > 0:
				return fail("a created character takes no class, clan or quest line")
			}
			inWorld = false
		case "enter":
			if inWorld {
				return fail("enter needs the character select: after create or restart")
			}
			inWorld = true
		default:
			if !inWorld {
				return fail("%s needs the character in the world: enter first", st.Verb)
			}
			if st.Verb == "restart" {
				inWorld = false
			}
		}
		for _, ck := range st.Checks {
			if !inWorld && ck.Kind != "rows" {
				return fmt.Errorf("%d: a %s check reads the character in the world: after %s, only rows", ck.Line, ck.Kind, st.Verb)
			}
		}
	}
	return nil
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
		level, err := strconv.ParseInt(args[1], 10, 32)
		if err != nil {
			return fmt.Errorf("level %q: %w", args[1], err)
		}
		sc.Character, sc.Level = args[0], int(level)
	case "class":
		if len(args) != 1 || sc.Class != 0 {
			return fmt.Errorf("one class line: class <class id>")
		}
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("class %q: %w", args[0], err)
		}
		if id == 0 {
			return fmt.Errorf("class 0 is the default: leave the line out")
		}
		if _, ok := player.ClassRace(id); !ok {
			return fmt.Errorf("class %d has no race", id)
		}
		sc.Class = id
	case "clan":
		if len(args) != 1 || sc.Clan >= 0 {
			return fmt.Errorf("one clan line: clan <level>")
		}
		level, err := strconv.Atoi(args[0])
		if err != nil || level < 0 || level > maxClanLevel {
			return fmt.Errorf("clan level %q is not 0 to %d", args[0], maxClanLevel)
		}
		sc.Clan = level
	case "quest":
		if len(args) < 2 {
			return fmt.Errorf("quest <quest> <var>=<value>, one or more")
		}
		q := QuestRows{Quest: args[0]}
		for _, l := range sc.Quests {
			if l.Quest == q.Quest {
				return fmt.Errorf("quest %s listed twice", q.Quest)
			}
		}
		for _, a := range args[1:] {
			k, v, ok := strings.Cut(a, "=")
			if !ok || k == "" {
				return fmt.Errorf("row %q is not <var>=<value>", a)
			}
			q.Vars = append(q.Vars, [2]string{k, v})
		}
		sc.Quests = append(sc.Quests, q)
	case "random":
		if sc.Random != nil {
			return fmt.Errorf("one random line")
		}
		rd := &Random{}
		if len(args) > 0 && args[0] == "always" {
			if len(args) != 2 {
				return fmt.Errorf("random always <draw>")
			}
			rd.Always, args = true, args[1:]
		}
		if len(args) == 0 {
			return fmt.Errorf("random <draw> ... | random always <draw>")
		}
		for _, a := range args {
			v, err := strconv.Atoi(a)
			if err != nil || v < 0 {
				return fmt.Errorf("draw %q is not a number of 0 or more", a)
			}
			rd.Draws = append(rd.Draws, v)
		}
		sc.Random = rd
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
			v, err := strconv.ParseInt(a, 10, 32)
			if err != nil {
				return fmt.Errorf("npc number %q: %w", a, err)
			}
			offsets[i] = int(v)
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
	case "action", "hit", "kill", "approach":
		if !roles[args[0]] {
			return fmt.Errorf("unknown role %q", args[0])
		}
	}
	switch verb {
	case "hit":
		if _, err := strconv.ParseInt(args[1], 10, 32); err != nil {
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
	case "pickup", "mark":
		if _, err := strconv.ParseInt(args[0], 10, 32); err != nil {
			return fmt.Errorf("%s number %q: %w", verb, args[0], err)
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
