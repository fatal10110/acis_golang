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

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// renders are the opcodes a step compares: the dialog's, the quest's and
// the item messages. The rest (selection, approach, animation, NPC info,
// the deferred inventory and status updates) is not the script's.
var renders = []byte{
	serverpackets.OpcodeActionFailed, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeSystemMessage,
	serverpackets.OpcodePlaySound, serverpackets.OpcodeQuestList, 0xfe,
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

// expandPage replaces a trailing "page=<path>" of an NpcHtmlMessage line
// with the page's text, %objectId% filled in with the packet's NPC, as the
// rendered packet writes it.
func expandPage(line string, pages map[string]string) (string, error) {
	head, path, ok := strings.Cut(line, " page=")
	if !ok || !strings.HasPrefix(line, "NpcHtmlMessage obj=") {
		return line, nil
	}
	text, ok := pages[path]
	if !ok {
		return "", fmt.Errorf("page %s is not a page line of the scenario", path)
	}
	obj, _, _ := strings.Cut(strings.TrimPrefix(head, "NpcHtmlMessage obj="), " ")
	return head + " html=" + scriptcontract.Quote(strings.ReplaceAll(text, "%objectId%", "{"+obj+"}")), nil
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
// server answered: the rendered packets and the journal statements.
type traceStep struct {
	client string
	sent   []string
	stmts  []string
}

var (
	traceAction = regexp.MustCompile(`^C Action (\d+) \(object (\d+)\)$`)
	traceKill   = regexp.MustCompile(`^# kill (\d+) \(object (\d+)\)$`)
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
		case (strings.HasPrefix(line, "S ") || strings.HasPrefix(line, "Q ")) && len(raw) > 0:
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
			if strings.HasPrefix(line, "Q ") {
				st, err := scriptcontract.ParseStatement(line)
				if err != nil {
					return nil, nil, err
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
// packets and statements.
func checkTrace(sc *Scenario, data []byte, pages map[string]string) error {
	steps, objs, err := readTrace(sc, data)
	if err != nil {
		return err
	}
	for i, ts := range steps {
		if i >= len(sc.Steps) {
			return fmt.Errorf("the trace has %d client packets, the scenario %d steps", len(steps), len(sc.Steps))
		}
		st := sc.Steps[i]
		where := fmt.Sprintf("step %d (line %d) %q against trace %q", i+1, st.Line, st, ts.client)
		if client, err := traceClient(st, objs, sc); err != nil {
			return fmt.Errorf("%s: %w", where, err)
		} else if client != ts.client {
			return fmt.Errorf("%s: the step sends %q", where, client)
		}
		want, err := expandPackets(st.Packets, pages)
		if err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		if d := FirstDiff("packet", want, ts.sent); d != "" {
			return fmt.Errorf("%s: scenario %s (got: the scenario, want: the trace)", where, d)
		}
		if d := FirstDiff("statement", st.Statements, ts.stmts); d != "" {
			return fmt.Errorf("%s: scenario %s (got: the scenario, want: the trace)", where, d)
		}
	}
	return nil
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
	case "kill":
		// A trace records each kill alone.
		if len(st.Args) == 1 || st.Args[1] == "x1" {
			for _, n := range sc.NPCs {
				if n.Role == st.Args[0] {
					return fmt.Sprintf("kill %d", n.ID), nil
				}
			}
		}
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
