// Package macro holds a player's client macros: the ordered macro list, the
// rules a client macro edit must pass, and the persisted command format.
package macro

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// Command types a macro line can carry.
const (
	CommandSkill    = 1
	CommandAction   = 3
	CommandShortcut = 4
)

const (
	// maxCommandText is the most UTF-16 code units of command text, summed
	// over the decoded lines, an edit may carry.
	maxCommandText = 255
	// maxMacros is the macro count past which an edit is refused: the
	// refusal is for a list already holding more than this many, so a
	// list may grow to one past it.
	maxMacros = 24
	// maxDescription is the most UTF-16 code units a description may hold.
	maxDescription = 32
	// maxStoredCommands is the most UTF-16 code units the persisted command
	// column receives; a longer serialization is cut there.
	maxStoredCommands = 255
	// firstID is where new macro ids start.
	firstID = 1000
	// firstRevision is a new list's revision; every update the client is
	// sent advances it first.
	firstRevision = 1
)

// Command is one macro line. Lines are numbered by position: the line
// number a client edit carries is not kept.
type Command struct {
	Type int32
	// D1 is the skill id, or the shortcut page.
	D1 int32
	// D2 is the shortcut slot.
	D2   int32
	Text string
}

// Macro is one client macro.
type Macro struct {
	ID          int32
	Icon        int32
	Name        string
	Description string
	Acronym     string
	Commands    []Command
}

// Refusal is why a macro edit was refused.
type Refusal int

// Edit refusals, in the order they are checked.
const (
	Accepted Refusal = iota
	// CommandsTooLong: the decoded lines carry more than 255 code units of
	// command text.
	CommandsTooLong
	// TooMany: the list already holds more than 24 macros.
	TooMany
	// NameMissing: the macro has no name.
	NameMissing
	// NameTaken: another macro of the list has the same name, ignoring
	// case.
	NameTaken
	// DescriptionTooLong: the description is longer than 32 code units.
	DescriptionTooLong
)

// Row is one character_macroses row as read back.
type Row struct {
	ID          int32
	Icon        int32
	Name        string
	Description string
	Acronym     string
	// Commands is the persisted command text; Valid is false when the
	// column is NULL.
	Commands      string
	CommandsValid bool
}

// List is a player's macros in insertion order. It is owned by the player's
// queue; callers serialize access the same way they serialize the player's
// packet handling.
type List struct {
	order    []int32
	byID     map[int32]Macro
	revision int32
	nextID   int32
}

// NewList returns an empty list.
func NewList() *List {
	return &List{byID: make(map[int32]Macro), revision: firstRevision, nextID: firstID}
}

// Restore fills an empty list from rows, in row order. A row whose command
// text holds a non-numeric type or id stops the restore there: the rows
// before it are kept, that row and every later one are not, and the error
// says why.
func Restore(rows []Row) (*List, error) {
	l := NewList()
	for _, row := range rows {
		if !row.CommandsValid {
			return l, errNullCommands(row.ID)
		}
		cmds, err := DecodeCommands(row.Commands)
		if err != nil {
			return l, err
		}
		l.put(Macro{ID: row.ID, Icon: row.Icon, Name: row.Name, Description: row.Description, Acronym: row.Acronym, Commands: cmds})
	}
	return l, nil
}

// Len reports how many macros the list holds.
func (l *List) Len() int {
	if l == nil {
		return 0
	}
	return len(l.order)
}

// Has reports whether the list holds a macro with id.
func (l *List) Has(id int32) bool {
	if l == nil {
		return false
	}
	_, ok := l.byID[id]
	return ok
}

// All returns the macros in list order.
func (l *List) All() []Macro {
	if l == nil {
		return nil
	}
	out := make([]Macro, 0, len(l.order))
	for _, id := range l.order {
		out = append(out, l.byID[id])
	}
	return out
}

// Check reports whether m, decoded from a client edit whose lines carried
// commandText code units of command text, may be registered.
func (l *List) Check(m Macro, commandText int) Refusal {
	switch {
	case commandText > maxCommandText:
		return CommandsTooLong
	case l.Len() > maxMacros:
		return TooMany
	case m.Name == "":
		return NameMissing
	case l.nameTaken(m):
		return NameTaken
	case UTF16Len(m.Description) > maxDescription:
		return DescriptionTooLong
	}
	return Accepted
}

func (l *List) nameTaken(m Macro) bool {
	for _, id := range l.order {
		other := l.byID[id]
		if other.ID != m.ID && equalFoldJava(other.Name, m.Name) {
			return true
		}
	}
	return false
}

// Register adds m, or replaces the macro with its id, and returns it as
// stored. A macro with id 0 gets the next free id from 1000 upward.
func (l *List) Register(m Macro) Macro {
	if m.ID == 0 {
		m.ID = l.nextID
		l.nextID++
		for l.Has(m.ID) {
			m.ID = l.nextID
			l.nextID++
		}
	}
	l.put(m)
	return m
}

func (l *List) put(m Macro) {
	if _, ok := l.byID[m.ID]; !ok {
		l.order = append(l.order, m.ID)
	}
	l.byID[m.ID] = m
}

// Delete removes the macro with id and reports whether there was one.
func (l *List) Delete(id int32) bool {
	if !l.Has(id) {
		return false
	}
	delete(l.byID, id)
	for i, have := range l.order {
		if have == id {
			l.order = append(l.order[:i], l.order[i+1:]...)
			break
		}
	}
	return true
}

// NextRevision advances the list's revision and returns it: every macro
// list the client is sent carries a fresh one.
func (l *List) NextRevision() int32 {
	l.revision++
	return l.revision
}

// EncodeCommands serializes cmds into the persisted command text: each line
// as type,d1,d2 then ",text" when it has text, closed by ';', and the whole
// cut to 255 UTF-16 code units. A cut through a surrogate pair drops its
// leading half.
func EncodeCommands(cmds []Command) string {
	var sb strings.Builder
	for _, c := range cmds {
		sb.WriteString(strconv.Itoa(int(c.Type)))
		sb.WriteByte(',')
		sb.WriteString(strconv.Itoa(int(c.D1)))
		sb.WriteByte(',')
		sb.WriteString(strconv.Itoa(int(c.D2)))
		if c.Text != "" {
			sb.WriteByte(',')
			sb.WriteString(c.Text)
		}
		sb.WriteByte(';')
	}
	return truncateUTF16(sb.String(), maxStoredCommands)
}

// DecodeCommands parses persisted command text. Lines are split on ';' and
// fields on ','; empty pieces are skipped either way. A line with fewer
// than three fields is skipped; its fourth field, if any, is its text and
// any later field is lost. A type, d1 or d2 that is not a decimal int32
// fails the whole decode.
func DecodeCommands(s string) ([]Command, error) {
	var out []Command
	for _, line := range tokens(s, ';') {
		fields := tokens(line, ',')
		if len(fields) < 3 {
			continue
		}
		var nums [3]int32
		for i := range nums {
			n, err := commons.ParseInt(fields[i], 32)
			if err != nil {
				return nil, errBadCommand(line, err)
			}
			nums[i] = int32(n)
		}
		c := Command{Type: nums[0], D1: nums[1], D2: nums[2]}
		if len(fields) > 3 {
			c.Text = fields[3]
		}
		out = append(out, c)
	}
	return out, nil
}

func errNullCommands(id int32) error {
	return fmt.Errorf("macro %d: no command text", id)
}

func errBadCommand(line string, err error) error {
	return fmt.Errorf("macro command %q: %w", line, err)
}

// tokens splits s on sep and drops the empty pieces.
func tokens(s string, sep byte) []string {
	var out []string
	for piece := range strings.SplitSeq(s, string(sep)) {
		if piece != "" {
			out = append(out, piece)
		}
	}
	return out
}

// UTF16Len reports how many UTF-16 code units s encodes to: the length the
// client counts in.
func UTF16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

func truncateUTF16(s string, limit int) string {
	n := 0
	for i, r := range s {
		w := utf16.RuneLen(r)
		if n+w > limit {
			return s[:i]
		}
		n += w
	}
	return s
}

// equalFoldJava compares a and b ignoring case code point by code point,
// matching on equal code points, equal upper cases, or equal lower cases
// of the upper cases; strings of different UTF-16 length never match.
func equalFoldJava(a, b string) bool {
	if UTF16Len(a) != UTF16Len(b) {
		return false
	}
	ra, rb := []rune(a), []rune(b)
	if len(ra) != len(rb) {
		return false
	}
	for i := range ra {
		c1, c2 := ra[i], rb[i]
		if c1 == c2 {
			continue
		}
		u1, u2 := unicode.ToUpper(c1), unicode.ToUpper(c2)
		if u1 == u2 || unicode.ToLower(u1) == unicode.ToLower(u2) {
			continue
		}
		return false
	}
	return true
}
