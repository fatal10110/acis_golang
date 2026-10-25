package scenario

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// renders are the opcodes a step compares: the dialog's, the quest's, the
// tutorial's and the item messages, the radar marks, the drops, the casts,
// the NPC chat lines a script makes and the player's own teleports. The rest
// (selection, approach, movement, NPC info, the deferred inventory and status
// updates) is not the script's.
var renders = []byte{
	serverpackets.OpcodeActionFailed, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeSystemMessage,
	serverpackets.OpcodePlaySound, serverpackets.OpcodeQuestList, 0xfe,
	serverpackets.OpcodeTutorialShowHTML, serverpackets.OpcodeTutorialShowQuestionMark,
	serverpackets.OpcodeTutorialEnableClientEvent, serverpackets.OpcodeTutorialCloseHTML,
	serverpackets.OpcodeRadarControl, serverpackets.OpcodeDropItem, serverpackets.OpcodeMagicSkillUse,
	serverpackets.OpcodeNpcSay, serverpackets.OpcodeTeleportToLocation,
}

// render renders a server packet payload as an S line, with roles naming
// object ids; false for a packet a step does not compare. A
// TeleportToLocation is compared only when it moves the player: where it
// lands. Another creature's teleport is the world's.
func render(payload []byte, roles map[int32]string) (string, bool, error) {
	if len(payload) == 0 || !slices.Contains(renders, payload[0]) {
		return "", false, nil
	}
	if payload[0] == serverpackets.OpcodeTeleportToLocation &&
		(len(payload) < 5 || roles[int32(binary.LittleEndian.Uint32(payload[1:5]))] != "player") {
		return "", false, nil
	}
	line, err := scriptcontract.Packet(payload, roles)
	if err != nil {
		if payload[0] == 0xfe {
			return "", false, nil // an extended packet other than the quest mark
		}
		return "", false, err
	}
	return strings.TrimPrefix(line, "S "), true, nil
}

// expandPage replaces a trailing "page=<path>" of an NpcHtmlMessage or a
// TutorialShowHtml line with the page's text, as the rendered packet writes
// it: in an NpcHtmlMessage, %objectId% filled in with the packet's NPC.
func expandPage(line string, pages map[string]string) (string, error) {
	head, path, ok := strings.Cut(line, " page=")
	npcPage := strings.HasPrefix(line, "NpcHtmlMessage obj=")
	if !ok || !npcPage && head != "TutorialShowHtml" {
		return line, nil
	}
	text, ok := pages[path]
	if !ok {
		return "", fmt.Errorf("page %s is not a page line of the scenario", path)
	}
	if npcPage {
		obj, _, _ := strings.Cut(strings.TrimPrefix(head, "NpcHtmlMessage obj="), " ")
		text = strings.ReplaceAll(text, "%objectId%", "{"+obj+"}")
	}
	return head + " html=" + scriptcontract.Quote(text), nil
}

// expandPackets is expandPage over a step's packets.
func expandPackets(lines []string, pages map[string]string) ([]string, error) {
	out := make([]string, len(lines))
	for i, l := range lines {
		var err error
		if out[i], err = expandPage(l, pages); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// traceStep is one client packet (or probe action) of a reference trace
// with what the server answered: the rendered packets, the journal
// statements, the memo writes and the pinned script draws, each
// "<bound>:<value>".
type traceStep struct {
	client string
	tasks  []traceTask
	stmts  []string
	memos  []string
	draws  []string
}

// traceTask is the rendered packets one task of a trace sent, in the
// reference's order: the client packet's (or probe action's) own run, or a
// tick's. own reports a client packet's own run, which runs on the player's
// own queue.
type traceTask struct {
	sent []string
	own  bool
}

var (
	traceAction   = regexp.MustCompile(`^C Action (\d+) \(object (\d+)\)$`)
	tracePickup   = regexp.MustCompile(`^C Action item (\d+) \(object \d+\)$`)
	traceKill     = regexp.MustCompile(`^# kill (\d+) \(object (\d+)\)$`)
	traceHit      = regexp.MustCompile(`^# hit (\d+) \(object (\d+)\) (\d+)$`)
	traceAdvance  = regexp.MustCompile(`^# advance (\d+)$`)
	traceApproach = regexp.MustCompile(`^# approach (\d+) \(object (\d+)\)$`)
	traceNPC      = regexp.MustCompile(`^# npc (\S+) (\d+) \(object (\d+)\)$`)
	traceDraw     = regexp.MustCompile(`^# draw (\d+:\d+) \S+$`)
)

// probeTraces is the directory, below the module root, of the reference
// probe's plays of the scenarios themselves.
const probeTraces = "internal/gameserver/script/testdata/oracle/scenarios/"

// Player memo statements, as the reference writes them.
const (
	memoSet   = "INSERT INTO character_memo (charId, var, val) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE val = VALUES(val)"
	memoUnset = "DELETE FROM character_memo WHERE charId = ? AND var = ?"
)

// traceStatement sorts one Q line of a trace: a journal statement, as a
// Q line writes it, or a memo write, as an M line does; neither for a
// read or a statement on another table, an error for an unknown write on
// the journal or memo table.
func traceStatement(line string) (stmt, memo string, err error) {
	sql, params, _ := strings.Cut(strings.TrimPrefix(line, "Q "), " | ")
	if strings.HasPrefix(strings.ToUpper(sql), "SELECT ") {
		return "", "", nil
	}
	st, err := scriptcontract.ParseStatement(line)
	if err == nil {
		return string(st.Kind) + " " + strings.Join(st.Params[1:], " | "), "", nil
	}
	switch {
	case strings.Contains(sql, "character_quests"):
		// A journal statement must be one the format knows.
		return "", "", err
	case sql == memoSet:
		// charId | var | val
		if p := strings.SplitN(params, " | ", 3); len(p) == 3 {
			return "", "set " + p[1] + " | " + p[2], nil
		}
	case sql == memoUnset:
		if p := strings.SplitN(params, " | ", 2); len(p) == 2 {
			return "", "unset " + p[1], nil
		}
	case strings.Contains(sql, "character_memo"):
	default:
		// Another table's statement (raid points, the character's own
		// rows) is not compared.
		return "", "", nil
	}
	return "", "", fmt.Errorf("statement %q is not a memo write", sql)
}

// readTrace reads a reference trace (format: the oracle directory's
// README), naming the player "player" and each NPC by the role its spawn
// line names, else by the scenario role of its NPC id.
func readTrace(sc *Scenario, data []byte) (steps []traceStep, npcObjs map[string]int32, err error) {
	type rawStep struct {
		client string
		lines  []string
	}
	var (
		raw       []rawStep
		playerObj int32
	)
	// seen pairs each NPC object of the trace with its NPC id; actioned
	// holds those the client acted on, which a bypass can name; spawned
	// names the NPCs the probe spawned for an NPC line by its role.
	seen, actioned, spawned := map[int32]int32{}, map[int32]bool{}, map[int32]string{}
	sb := bufio.NewScanner(bytes.NewReader(data))
	sb.Buffer(make([]byte, 1<<20), 1<<20)
	for sb.Scan() {
		line := sb.Text()
		switch {
		case strings.HasPrefix(line, "# player object "):
			obj, _, _ := strings.Cut(strings.TrimPrefix(line, "# player object "), ";")
			n, err := strconv.ParseInt(obj, 10, 32)
			if err != nil {
				return nil, nil, fmt.Errorf("trace player object %q: %w", obj, err)
			}
			playerObj = int32(n)
		case tracePickup.MatchString(line):
			raw = append(raw, rawStep{client: "pickup " + tracePickup.FindStringSubmatch(line)[1]})
		case strings.HasPrefix(line, "C "):
			if m := traceAction.FindStringSubmatch(line); m != nil {
				seen[atoi32(m[2])] = atoi32(m[1])
				actioned[atoi32(m[2])] = true
			}
			raw = append(raw, rawStep{client: line})
		case traceApproach.MatchString(line):
			m := traceApproach.FindStringSubmatch(line)
			seen[atoi32(m[2])] = atoi32(m[1])
			raw = append(raw, rawStep{client: "approach " + m[1]})
		case line == "# tick":
			raw = append(raw, rawStep{client: "tick"})
		case traceNPC.MatchString(line):
			m := traceNPC.FindStringSubmatch(line)
			if !slices.ContainsFunc(sc.NPCs, func(n NPC) bool { return n.Role == m[1] && n.ID == atoi32(m[2]) }) {
				return nil, nil, fmt.Errorf("trace line %q names no NPC line of the scenario", line)
			}
			seen[atoi32(m[3])] = atoi32(m[2])
			spawned[atoi32(m[3])] = m[1]
		case traceKill.MatchString(line):
			m := traceKill.FindStringSubmatch(line)
			seen[atoi32(m[2])] = atoi32(m[1])
			raw = append(raw, rawStep{client: "kill " + m[1]})
		case traceHit.MatchString(line):
			m := traceHit.FindStringSubmatch(line)
			seen[atoi32(m[2])] = atoi32(m[1])
			raw = append(raw, rawStep{client: "hit " + m[1] + " " + m[3]})
		case traceAdvance.MatchString(line):
			raw = append(raw, rawStep{client: "advance " + traceAdvance.FindStringSubmatch(line)[1]})
		case (strings.HasPrefix(line, "S ") || strings.HasPrefix(line, "Q ") || strings.HasPrefix(line, "T ")) && len(raw) > 0:
			// A T line ends the packets one task sent: the client
			// packet's own run, then each tick's.
			raw[len(raw)-1].lines = append(raw[len(raw)-1].lines, line)
		case strings.HasPrefix(line, "# draw "):
			if !traceDraw.MatchString(line) {
				return nil, nil, fmt.Errorf("trace line %q is not a draw <bound>:<value> <class>", line)
			}
			if len(raw) == 0 {
				return nil, nil, fmt.Errorf("trace line %q comes before the first client packet", line)
			}
			raw[len(raw)-1].lines = append(raw[len(raw)-1].lines, line)
		}
	}
	if err := sb.Err(); err != nil {
		return nil, nil, err
	}
	roles := map[int32]string{playerObj: "player"}
	npcObjs = map[string]int32{}
	for obj, id := range seen {
		var named []string
		if role, ok := spawned[obj]; ok {
			named = []string{role}
		} else {
			for _, n := range sc.NPCs {
				if n.ID == id {
					named = append(named, n.Role)
				}
			}
		}
		switch len(named) {
		case 0:
			continue
		case 1:
		default:
			return nil, nil, fmt.Errorf("NPC id %d has roles %v; a traced scenario names one NPC per id", id, named)
		}
		roles[obj] = named[0]
		if actioned[obj] {
			npcObjs[named[0]] = obj
		}
	}
	for _, r := range raw {
		step := traceStep{client: r.client}
		// task holds the rendered packets of the task running now; own
		// reports that it is a client packet's own run, which runs on the
		// player's own queue.
		var task []string
		own := strings.HasPrefix(r.client, "C ")
		endTask := func() {
			if len(task) > 0 {
				step.tasks = append(step.tasks, traceTask{sent: task, own: own})
			}
			task, own = nil, false
		}
		for _, line := range r.lines {
			if strings.HasPrefix(line, "T ") {
				endTask()
				continue
			}
			if m := traceDraw.FindStringSubmatch(line); m != nil {
				step.draws = append(step.draws, m[1])
				continue
			}
			if strings.HasPrefix(line, "Q ") {
				stmt, memo, err := traceStatement(line)
				switch {
				case err != nil:
					return nil, nil, fmt.Errorf("trace line %q: %w", line, err)
				case stmt != "":
					step.stmts = append(step.stmts, stmt)
				case memo != "":
					step.memos = append(step.memos, memo)
				}
				continue
			}
			fields := strings.Fields(line)
			payload, err := hex.DecodeString(fields[len(fields)-1])
			if err != nil {
				return nil, nil, fmt.Errorf("trace line %q: %w", line, err)
			}
			rendered, ok, err := render(payload, roles)
			if err != nil {
				return nil, nil, fmt.Errorf("trace line %q: %w", line, err)
			}
			if ok {
				task = append(task, rendered)
			}
		}
		endTask()
		steps = append(steps, step)
	}
	return steps, npcObjs, nil
}

// stepPackets joins the rendered packets of tasks, the tasks one step of
// the trace ran, in the order the Go run sends them. A script's teleport of
// the player, or its cast with the player as caster, from a hook on
// another queue than the player's runs on the player's queue once the
// hook's task is done, so the packets the rest of the hook sends go out
// before it (script engine plan, section 9, item 4); from a hook on the
// player's own queue it runs inside the call, as in the reference. queues
// names, in order, the queue of each task that teleports or casts with the
// player ("own" or "other", the step's queue line); nil leaves a client
// packet's own run on the player's queue and every other task on another.
func stepPackets(tasks []traceTask, queues []string) ([]string, error) {
	moving := 0
	for _, t := range tasks {
		if slices.ContainsFunc(t.sent, movesPlayer) {
			moving++
		}
	}
	if queues != nil && len(queues) != moving {
		return nil, fmt.Errorf("the queue line names %d tasks, the trace has %d that teleport the player or make it cast", len(queues), moving)
	}
	var sent []string
	for _, t := range tasks {
		lines := t.sent
		if slices.ContainsFunc(lines, movesPlayer) {
			own := t.own
			if queues != nil {
				own, queues = queues[0] == "own", queues[1:]
			}
			if !own {
				lines = deferPlayerActions(lines)
			}
		}
		sent = append(sent, lines...)
	}
	return sent, nil
}

// movesPlayer reports whether a rendered packet is a script's teleport of
// the player or its cast with the player as caster.
func movesPlayer(line string) bool {
	return isPlayerTeleport(line) || strings.HasPrefix(line, "MagicSkillUse caster=player ")
}

func isPlayerTeleport(line string) bool {
	return strings.HasPrefix(line, "TeleportToLocation obj=player ")
}

// deferPlayerActions moves each teleport of the player among task, the
// rendered packets of a task whose hook ran on another queue than the
// player's, to the end of task, with the ActionFailed lines right before it
// (its aborts'), and each cast with the player as caster with the teleport
// that follows it (the skill's, after its aborts), in their order.
func deferPlayerActions(task []string) []string {
	var rest, moved []string
	for i := 0; i < len(task); i++ {
		line := task[i]
		switch {
		case isPlayerTeleport(line):
			start := len(rest)
			for start > 0 && rest[start-1] == "ActionFailed" {
				start--
			}
			moved = append(moved, rest[start:]...)
			moved = append(moved, line)
			rest = rest[:start]
		case strings.HasPrefix(line, "MagicSkillUse caster=player "):
			end := i + 1
			for end < len(task) && task[end] == "ActionFailed" {
				end++
			}
			if end == len(task) || !isPlayerTeleport(task[end]) {
				end = i
			}
			moved = append(moved, task[i:end+1]...)
			i = end
		default:
			rest = append(rest, line)
		}
	}
	return append(rest, moved...)
}

// checkTrace requires the scenario's steps to replay the trace: the same
// client packets, and S, Q and M lines equal to the trace's rendered
// packets, journal statements and memo writes. A step that is several
// client packets or probe actions of the trace (a kill of N kills, N ticks,
// a relog, an enter) replays them all, its lines theirs in order; a relog's
// packets are not compared, as the run does not compare them. It returns
// the script draws the reference pinned in each traced step, each
// "<bound>:<value>", for the run to compare with its own; a trace that pins
// draws needs a random line. A trace of the reference probe's own play of
// the scenario (under probeTraces) covers every step.
func checkTrace(sc *Scenario, data []byte, pages map[string]string) (draws [][]string, err error) {
	steps, objs, err := readTrace(sc, data)
	if err != nil {
		return nil, err
	}
	for i, next := 0, 0; next < len(steps); i++ {
		if i >= len(sc.Steps) {
			return nil, fmt.Errorf("the trace has %d client packets, the scenario %d steps", len(steps), len(sc.Steps))
		}
		st := sc.Steps[i]
		clients, err := traceClients(st, objs, sc)
		where := fmt.Sprintf("step %d (line %d) %q against trace %q", i+1, st.Line, st, steps[next].client)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", where, err)
		}
		var ts traceStep
		for j, client := range clients {
			if next+j >= len(steps) {
				return nil, fmt.Errorf("%s: the trace ends after %d of its %d client packets", where, j, len(clients))
			}
			s := steps[next+j]
			if client != s.client {
				return nil, fmt.Errorf("%s: the step sends %q", where, client)
			}
			ts.tasks = append(ts.tasks, s.tasks...)
			ts.stmts = append(ts.stmts, s.stmts...)
			ts.memos = append(ts.memos, s.memos...)
			ts.draws = append(ts.draws, s.draws...)
		}
		next += len(clients)
		sent, err := stepPackets(ts.tasks, st.Queues)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", where, err)
		}
		if st.Verb != "relog" {
			want, err := expandPackets(st.Packets, pages)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", where, err)
			}
			if d := FirstDiff("packet", want, sent); d != "" {
				return nil, fmt.Errorf("%s: scenario %s (got: the scenario, want: the trace)", where, d)
			}
		}
		if d := FirstDiff("statement", st.Statements, ts.stmts); d != "" {
			return nil, fmt.Errorf("%s: scenario %s (got: the scenario, want: the trace)", where, d)
		}
		if d := FirstDiff("memo", st.Memos, ts.memos); d != "" {
			return nil, fmt.Errorf("%s: scenario %s (got: the scenario, want: the trace)", where, d)
		}
		draws = append(draws, ts.draws)
	}
	// The probe plays every step of a scenario it traces.
	if strings.HasPrefix(sc.Trace, probeTraces) && len(draws) < len(sc.Steps) {
		st := sc.Steps[len(draws)]
		return nil, fmt.Errorf("the trace ends before step %d (line %d) %q", len(draws)+1, st.Line, st)
	}
	// The run pins and records its script draws only with a random line.
	if sc.Random == nil && slices.ContainsFunc(draws, func(d []string) bool { return len(d) > 0 }) {
		return nil, fmt.Errorf("the trace pins script draws and the scenario has no random line")
	}
	return draws, nil
}

// traceDrawDiff compares the script draws step i made, each
// "<bound>:<value>", with those the reference made in that step: a traced
// step draws the same bounds in the same order. A step past the trace is
// not compared.
func traceDrawDiff(traceDraws [][]string, i int, drawn []string) string {
	if i >= len(traceDraws) {
		return ""
	}
	return FirstDiff("script draw", drawn, traceDraws[i])
}

// traceClients writes the client packets (and probe actions) of st the
// way a trace records them, with the trace's object ids.
func traceClients(st Step, objs map[string]int32, sc *Scenario) ([]string, error) {
	npcID := func(role string) (int32, bool) {
		for _, n := range sc.NPCs {
			if n.Role == role {
				return n.ID, true
			}
		}
		return 0, false
	}
	// times is a kill's or tick's xN: N of them, 1 without.
	times := func(args []string) int {
		if len(args) == 1 {
			n, _ := killCount(args[0])
			return n
		}
		return 1
	}
	enter := []string{"C RequestGameStart 0", "C EnterWorld"}
	switch st.Verb {
	case "action":
		if id, ok := npcID(st.Args[0]); ok {
			return []string{fmt.Sprintf("C Action %d (object %d)", id, objs[st.Args[0]])}, nil
		}
	case "bypass":
		return []string{"C RequestBypassToServer " + expandRoles(st.Args[0], objs)}, nil
	case "link":
		return []string{"C RequestLinkHtml " + st.Args[0]}, nil
	case "hit":
		if id, ok := npcID(st.Args[0]); ok {
			return []string{fmt.Sprintf("hit %d %s", id, st.Args[1])}, nil
		}
	case "kill":
		// A trace records each kill alone: a kill step of N kills is N of
		// them.
		if id, ok := npcID(st.Args[0]); ok {
			return slices.Repeat([]string{fmt.Sprintf("kill %d", id)}, times(st.Args[1:])), nil
		}
	case "approach":
		if id, ok := npcID(st.Args[0]); ok {
			return []string{fmt.Sprintf("approach %d", id)}, nil
		}
	case "tick":
		return slices.Repeat([]string{"tick"}, times(st.Args)), nil
	case "advance":
		d, _ := time.ParseDuration(st.Args[0])
		return []string{"advance " + strconv.FormatInt(d.Milliseconds(), 10)}, nil
	case "pickup":
		return []string{"pickup " + st.Args[0]}, nil
	case "mark":
		return []string{"C RequestTutorialQuestionMark " + st.Args[0]}, nil
	case "create":
		return []string{"C RequestCharacterCreate " + sc.Character}, nil
	case "restart":
		return []string{"C RequestRestart"}, nil
	case "enter":
		return enter, nil
	case "relog":
		return append([]string{"C RequestRestart"}, enter...), nil
	}
	return nil, fmt.Errorf("a %s step is not a client packet a trace records", st.Verb)
}

// expandRoles replaces each {role} of s with that role's object id.
func expandRoles(s string, objs map[string]int32) string {
	for role, obj := range objs {
		s = strings.ReplaceAll(s, "{"+role+"}", strconv.Itoa(int(obj)))
	}
	return s
}

func atoi32(s string) int32 {
	n, _ := strconv.ParseInt(s, 10, 32)
	return int32(n)
}
