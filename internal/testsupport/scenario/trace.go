package scenario

import (
	"bufio"
	"bytes"
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
// tutorial's and the item messages, the radar marks, the drops, the casts
// and the NPC chat lines a script makes. The rest (selection, approach, movement, NPC info, the
// deferred inventory and status updates) is not the script's.
var renders = []byte{
	serverpackets.OpcodeActionFailed, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeSystemMessage,
	serverpackets.OpcodePlaySound, serverpackets.OpcodeQuestList, 0xfe,
	serverpackets.OpcodeTutorialShowHTML, serverpackets.OpcodeTutorialShowQuestionMark,
	serverpackets.OpcodeTutorialEnableClientEvent, serverpackets.OpcodeTutorialCloseHTML,
	serverpackets.OpcodeRadarControl, serverpackets.OpcodeDropItem, serverpackets.OpcodeMagicSkillUse,
	serverpackets.OpcodeNpcSay,
}

// render renders a server packet payload as an S line, with roles naming
// object ids; false for a packet a step does not compare.
func render(payload []byte, roles map[int32]string) (string, bool, error) {
	if len(payload) == 0 || !slices.Contains(renders, payload[0]) {
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

// traceStep is one client packet of a reference trace with what the
// server answered: the rendered packets, the journal statements and the
// pinned script draws, each "<bound>:<value>".
type traceStep struct {
	client string
	sent   []string
	stmts  []string
	draws  []string
}

var (
	traceAction  = regexp.MustCompile(`^C Action (\d+) \(object (\d+)\)$`)
	traceKill    = regexp.MustCompile(`^# kill (\d+) \(object (\d+)\)$`)
	traceHit     = regexp.MustCompile(`^# hit (\d+) \(object (\d+)\) (\d+)$`)
	traceAdvance = regexp.MustCompile(`^# advance (\d+)$`)
	traceDraw    = regexp.MustCompile(`^# draw (\d+:\d+) \S+$`)
)

// readTrace reads a reference trace (format: the oracle directory's
// README), naming the player "player" and each NPC by the scenario role
// of its NPC id.
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
	// holds those the client acted on, which a bypass can name.
	seen, actioned := map[int32]int32{}, map[int32]bool{}
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
		case strings.HasPrefix(line, "C "):
			if m := traceAction.FindStringSubmatch(line); m != nil {
				seen[atoi32(m[2])] = atoi32(m[1])
				actioned[atoi32(m[2])] = true
			}
			raw = append(raw, rawStep{client: line})
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
		case (strings.HasPrefix(line, "S ") || strings.HasPrefix(line, "Q ")) && len(raw) > 0:
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
		for _, n := range sc.NPCs {
			if n.ID == id {
				named = append(named, n.Role)
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
		for _, line := range r.lines {
			if m := traceDraw.FindStringSubmatch(line); m != nil {
				step.draws = append(step.draws, m[1])
				continue
			}
			if strings.HasPrefix(line, "Q ") {
				st, err := scriptcontract.ParseStatement(line)
				if err != nil {
					// Another table's statement (a player memo, raid points)
					// is not compared: the Q lines are the quest journal's.
					// A journal statement must be one the format knows.
					if sql, _, _ := strings.Cut(line, " | "); strings.Contains(sql, "character_quests") {
						return nil, nil, fmt.Errorf("trace line %q: %w", line, err)
					}
					continue
				}
				step.stmts = append(step.stmts, string(st.Kind)+" "+strings.Join(st.Params[1:], " | "))
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
				step.sent = append(step.sent, rendered)
			}
		}
		steps = append(steps, step)
	}
	return steps, npcObjs, nil
}

// checkTrace requires the scenario's first steps to replay the trace:
// the same client packets, and S and Q lines equal to the trace's rendered
// packets and statements. A kill step of N kills replays N kills of the
// trace, its lines theirs in order. It returns the script draws the
// reference pinned in each traced step, each "<bound>:<value>", for the run
// to compare with its own; a trace that pins draws needs a random line.
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
		n := 1
		if st.Verb == "kill" && len(st.Args) == 2 {
			n, _ = killCount(st.Args[1])
		}
		ts := traceStep{client: steps[next].client}
		where := fmt.Sprintf("step %d (line %d) %q against trace %q", i+1, st.Line, st, ts.client)
		if next+n > len(steps) {
			return nil, fmt.Errorf("%s: the trace ends after %d of its %d kills", where, len(steps)-next, n)
		}
		client, err := traceClient(st, objs, sc)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", where, err)
		}
		for _, s := range steps[next : next+n] {
			if client != s.client {
				return nil, fmt.Errorf("%s: the step sends %q", where, client)
			}
			ts.sent = append(ts.sent, s.sent...)
			ts.stmts = append(ts.stmts, s.stmts...)
			ts.draws = append(ts.draws, s.draws...)
		}
		next += n
		want, err := expandPackets(st.Packets, pages)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", where, err)
		}
		if d := FirstDiff("packet", want, ts.sent); d != "" {
			return nil, fmt.Errorf("%s: scenario %s (got: the scenario, want: the trace)", where, d)
		}
		if d := FirstDiff("statement", st.Statements, ts.stmts); d != "" {
			return nil, fmt.Errorf("%s: scenario %s (got: the scenario, want: the trace)", where, d)
		}
		draws = append(draws, ts.draws)
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

// traceClient writes the client packet of st the way a trace does, with
// the trace's object ids.
func traceClient(st Step, objs map[string]int32, sc *Scenario) (string, error) {
	switch st.Verb {
	case "action":
		for _, n := range sc.NPCs {
			if n.Role == st.Args[0] {
				return fmt.Sprintf("C Action %d (object %d)", n.ID, objs[n.Role]), nil
			}
		}
	case "bypass":
		return "C RequestBypassToServer " + expandRoles(st.Args[0], objs), nil
	case "link":
		return "C RequestLinkHtml " + st.Args[0], nil
	case "kill", "hit":
		// A trace records each kill alone: a kill step of N kills is N of
		// them.
		for _, n := range sc.NPCs {
			if n.Role == st.Args[0] {
				if st.Verb == "hit" {
					return fmt.Sprintf("hit %d %s", n.ID, st.Args[1]), nil
				}
				return fmt.Sprintf("kill %d", n.ID), nil
			}
		}
	case "advance":
		d, _ := time.ParseDuration(st.Args[0])
		return "advance " + strconv.FormatInt(d.Milliseconds(), 10), nil
	}
	return "", fmt.Errorf("a %s step is not a client packet a trace records", st.Verb)
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
