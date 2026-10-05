package scriptcontract

import (
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Packet renders one server packet payload (opcode first, as the test client
// and FrameCapture return it) the way the goldens write their "S" lines, so a
// test compares its captured packets with a golden row's lines directly.
// roles maps object ids to the role names the golden uses; they replace the
// ids in object fields and, as {role}, inside page text.
//
// NpcHtmlMessage, SystemMessage, PlaySound, ExShowQuestMark, QuestList and
// ActionFailed are decoded; any other packet is an error, because the goldens
// write those with the reference class name and raw bytes.
func Packet(payload []byte, roles map[int32]string) (string, error) {
	if len(payload) == 0 {
		return "", errors.New("empty packet")
	}
	d := decoder{b: payload[1:]}
	var out string
	switch payload[0] {
	case 0x25:
		out = "S ActionFailed"
	case 0x0f:
		obj := role(d.int32(), roles)
		html := d.str()
		for _, id := range sortedIDs(roles) {
			html = strings.ReplaceAll(html, strconv.Itoa(int(id)), "{"+roles[id]+"}")
		}
		out = fmt.Sprintf("S NpcHtmlMessage obj=%s item=%d html=%s", obj, d.int32(), Quote(html))
	case 0x64:
		var sb strings.Builder
		fmt.Fprintf(&sb, "S SystemMessage id=%d", d.int32())
		n := d.int32()
		for range n {
			typ := d.int32()
			fmt.Fprintf(&sb, " %d:", typ)
			switch typ {
			case 0:
				sb.WriteString(Quote(d.str()))
			case 4:
				fmt.Fprintf(&sb, "%d,%d", d.int32(), d.int32())
			case 7:
				fmt.Fprintf(&sb, "%d,%d,%d", d.int32(), d.int32(), d.int32())
			default:
				fmt.Fprintf(&sb, "%d", d.int32())
			}
		}
		out = sb.String()
	case 0x98:
		typ := d.int32()
		file := d.str()
		bind := d.int32()
		obj := role(d.int32(), roles)
		x, y, z := d.int32(), d.int32(), d.int32()
		out = fmt.Sprintf("S PlaySound type=%d file=%s bind=%d obj=%s loc=%d,%d,%d delay=%d", typ, Value(file), bind, obj, x, y, z, d.int32())
	case 0x80:
		var sb strings.Builder
		sb.WriteString("S QuestList")
		n := d.uint16()
		for range n {
			fmt.Fprintf(&sb, " %d:0x%08x", d.int32(), uint32(d.int32()))
		}
		out = sb.String()
	case 0xfe:
		if sub := d.uint16(); sub != 0x1a {
			return "", fmt.Errorf("extended packet 0x%04x is not rendered", sub)
		}
		out = fmt.Sprintf("S ExShowQuestMark quest=%d", d.int32())
	default:
		return "", fmt.Errorf("packet 0x%02x is not rendered", payload[0])
	}
	if d.err != nil {
		return "", d.err
	}
	// Every rendered packet ends at its last field; a trailing byte is an
	// encoder writing a field the client does not read.
	if len(d.b) != 0 {
		return "", fmt.Errorf("%d trailing bytes", len(d.b))
	}
	return out, nil
}

// Quote writes s the way the goldens quote values: double quotes, with
// backslash escapes for backslash, quote, newline, carriage return and tab,
// and \u00XX for any other control character.
func Quote(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, c := range s {
		switch c {
		case '\\':
			sb.WriteString(`\\`)
		case '"':
			sb.WriteString(`\"`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			if c < 0x20 || c == 0x7f {
				fmt.Fprintf(&sb, `\u%04x`, c)
			} else {
				sb.WriteRune(c)
			}
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

// Value writes s the way the goldens write a row value: bare when it is
// printable ASCII without space, quote, backslash or '=', quoted otherwise.
func Value(s string) string {
	if s == "" {
		return Quote(s)
	}
	for _, c := range s {
		if c <= 0x20 || c >= 0x7f || c == '"' || c == '\\' || c == '=' {
			return Quote(s)
		}
	}
	return s
}

// Statement is one "Q" line of a golden: a reference SQL statement and its
// bound parameters, with role ids already replaced ({player}).
type Statement struct {
	Kind   StatementKind
	Params []string
}

// StatementKind names the journal statements the reference executes.
type StatementKind string

// The journal statements.
const (
	// Upsert sets one variable: charId, quest name, var, value.
	Upsert StatementKind = "upsert"
	// DeleteVar removes one variable: charId, quest name, var.
	DeleteVar StatementKind = "delete-var"
	// DeleteQuest removes every row of a quest: charId, quest name.
	DeleteQuest StatementKind = "delete-quest"
	// DeleteExceptState removes every row of a quest but <state>: charId, quest name.
	DeleteExceptState StatementKind = "delete-except-state"
)

var statementKinds = map[string]StatementKind{
	"INSERT INTO character_quests (charId,name,var,value) VALUES (?,?,?,?) ON DUPLICATE KEY UPDATE value=VALUES(value)": Upsert,
	"DELETE FROM character_quests WHERE charId=? AND name=? AND var=?":                                                  DeleteVar,
	"DELETE FROM character_quests WHERE charId=? AND name=?":                                                            DeleteQuest,
	"DELETE FROM character_quests WHERE charId=? AND name=? AND var<>'<state>'":                                         DeleteExceptState,
}

// ParseStatement reads a golden "Q" line into its journal statement kind, so a
// test compares the Go store's writes with the reference order without
// depending on the SQL text.
func ParseStatement(line string) (Statement, error) {
	rest, ok := strings.CutPrefix(line, "Q ")
	if !ok {
		return Statement{}, fmt.Errorf("%q is not a statement line", line)
	}
	parts := strings.Split(rest, " | ")
	kind, ok := statementKinds[parts[0]]
	if !ok {
		return Statement{}, fmt.Errorf("statement %q is not a journal statement", parts[0])
	}
	return Statement{Kind: kind, Params: parts[1:]}, nil
}

func role(id int32, roles map[int32]string) string {
	if r, ok := roles[id]; ok {
		return r
	}
	return strconv.Itoa(int(id))
}

// sortedIDs orders role ids longest decimal first, so no id is replaced inside
// a longer one.
func sortedIDs(roles map[int32]string) []int32 {
	ids := make([]int32, 0, len(roles))
	for id := range roles {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b int32) int {
		if c := cmp.Compare(len(strconv.Itoa(int(b))), len(strconv.Itoa(int(a)))); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
	return ids
}

type decoder struct {
	b   []byte
	err error
}

func (d *decoder) take(n int) []byte {
	if d.err != nil {
		return make([]byte, n)
	}
	if len(d.b) < n {
		d.err = errors.New("packet ends early")
		return make([]byte, n)
	}
	v := d.b[:n]
	d.b = d.b[n:]
	return v
}

func (d *decoder) int32() int32 { return int32(binary.LittleEndian.Uint32(d.take(4))) }

func (d *decoder) uint16() uint16 { return binary.LittleEndian.Uint16(d.take(2)) }

func (d *decoder) str() string {
	var units []uint16
	for d.err == nil {
		u := binary.LittleEndian.Uint16(d.take(2))
		if u == 0 {
			break
		}
		units = append(units, u)
	}
	return string(utf16.Decode(units))
}
