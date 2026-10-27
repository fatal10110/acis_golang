package scriptcontract

import (
	"encoding/binary"
	"errors"
	"fmt"
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
// NpcHtmlMessage, SystemMessage, PlaySound, ExShowQuestMark, QuestList,
// ActionFailed, the tutorial window's packets (TutorialShowHtml,
// TutorialShowQuestionMark, TutorialEnableClientEvent, TutorialCloseHtml),
// RadarControl, DropItem, MagicSkillUse, NpcSay, TeleportToLocation,
// SpecialCamera, SocialAction, NpcInfo and DeleteObject are decoded; any
// other packet is an error, because the goldens write those with the reference
// class name and raw bytes.
//
// DropItem leaves out the dropped object's id and place, and MagicSkillUse
// the caster's and the target's places: the world draws those (the object id
// sequence, the drop scatter, where each creature stands), not the script.
// NpcInfo keeps the NPC's object and template id only: its place, speeds,
// equipment and name are the world's too.
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
		html = roleIDs(html, roles)
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
	case 0xa0:
		html := d.str()
		html = roleIDs(html, roles)
		out = "S TutorialShowHtml html=" + Quote(html)
	case 0xa1:
		out = fmt.Sprintf("S TutorialShowQuestionMark id=%d", d.int32())
	case 0xa2:
		out = fmt.Sprintf("S TutorialEnableClientEvent id=%d", d.int32())
	case 0xa3:
		out = "S TutorialCloseHtml"
	case 0xeb:
		show, typ := d.int32(), d.int32()
		out = fmt.Sprintf("S RadarControl show=%d type=%d loc=%d,%d,%d", show, typ, d.int32(), d.int32(), d.int32())
	case 0x0c:
		by := role(d.int32(), roles)
		d.int32() // the dropped object
		item := d.int32()
		d.int32() // x, y, z
		d.int32()
		d.int32()
		stackable, count := d.int32(), d.int32()
		out = fmt.Sprintf("S DropItem by=%s item=%d count=%d stackable=%d animate=%d", by, item, count, stackable, d.int32())
	case 0x48:
		caster, target := role(d.int32(), roles), role(d.int32(), roles)
		skill, level, hit, reuse := d.int32(), d.int32(), d.int32(), d.int32()
		d.int32() // the caster's x, y, z
		d.int32()
		d.int32()
		success := d.int32()
		if success != 0 {
			d.uint16()
		}
		d.int32() // the target's x, y, z
		d.int32()
		d.int32()
		out = fmt.Sprintf("S MagicSkillUse caster=%s target=%s skill=%d-%d hit=%d reuse=%d success=%d", caster, target, skill, level, hit, reuse, success)
	case 0x02:
		// The speaker's npc field is the client's NPC name id: 1000000 plus
		// the template id.
		speaker, typ, npcID := role(d.int32(), roles), d.int32(), d.int32()
		out = fmt.Sprintf("S NpcSay obj=%s type=%d npc=%d text=%s", speaker, typ, npcID, Quote(d.str()))
	case 0x28:
		obj := role(d.int32(), roles)
		x, y, z := d.int32(), d.int32(), d.int32()
		out = fmt.Sprintf("S TeleportToLocation obj=%s loc=%d,%d,%d fast=%d", obj, x, y, z, d.int32())
	case 0xc7:
		obj := role(d.int32(), roles)
		var shot [9]int32
		for i := range shot {
			shot[i] = d.int32()
		}
		out = fmt.Sprintf("S SpecialCamera obj=%s distance=%d yaw=%d pitch=%d time=%d duration=%d turn=%d rise=%d widescreen=%d unknown=%d",
			obj, shot[0], shot[1], shot[2], shot[3], shot[4], shot[5], shot[6], shot[7], shot[8])
	case 0x2d:
		obj := role(d.int32(), roles)
		out = fmt.Sprintf("S SocialAction obj=%s action=%d", obj, d.int32())
	case 0x16:
		// The npc field is the client's NPC name id: 1000000 plus the
		// template id.
		obj, npcID := role(d.int32(), roles), d.int32()
		out = fmt.Sprintf("S NpcInfo obj=%s npc=%d", obj, npcID-1000000)
		d.b = nil
	case 0x12:
		obj := role(d.int32(), roles)
		d.int32() // how the client removes it
		out = "S DeleteObject obj=" + obj
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

// roleIDs replaces, in text, every run of digits that is a role's object id
// with the role name in braces. A longer number holding an id's digits, such
// as a system message id in a link, is left whole, and so is a number that
// ends a word, such as the quest id in a quest name (Q101_...).
func roleIDs(text string, roles map[int32]string) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		if text[i] < '0' || text[i] > '9' {
			b.WriteByte(text[i])
			i++
			continue
		}
		j := i
		for j < len(text) && text[j] >= '0' && text[j] <= '9' {
			j++
		}
		run := text[i:j]
		inWord := i > 0 && isLetter(text[i-1])
		if id, err := strconv.ParseInt(run, 10, 32); err == nil && strconv.FormatInt(id, 10) == run {
			if name, ok := roles[int32(id)]; ok && !inWord {
				run = "{" + name + "}"
			}
		}
		b.WriteString(run)
		i = j
	}
	return b.String()
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

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
