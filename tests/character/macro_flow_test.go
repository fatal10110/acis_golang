package character

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/macro"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/rs/zerolog"
)

type macroLine struct {
	typ, d1, d2 int32
	text        string
}

func encodeRequestMakeMacro(id int32, name, desc, acronym string, icon byte, lines ...macroLine) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMakeMacro)
	w.WriteInt32(id)
	w.WriteString(name)
	w.WriteString(desc)
	w.WriteString(acronym)
	w.WriteUint8(icon)
	w.WriteUint8(uint8(len(lines)))
	for i, l := range lines {
		w.WriteUint8(uint8(i + 1))
		w.WriteUint8(uint8(l.typ))
		w.WriteInt32(l.d1)
		w.WriteUint8(uint8(l.d2))
		w.WriteString(l.text)
	}
	return w.Bytes()
}

func encodeRequestDeleteMacro(id int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestDeleteMacro)
	w.WriteInt32(id)
	return w.Bytes()
}

// sentMacro is one parsed SendMacroList packet.
type sentMacro struct {
	revision int32
	count    int
	id       int32 // 0 for the empty list's packet
	name     string
	lines    []macroLine
}

func parseSendMacroList(t *testing.T, frame []byte) sentMacro {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSendMacroList {
		t.Fatalf("opcode = %#x, want SendMacroList (%#x)", frame[0], serverpackets.OpcodeSendMacroList)
	}
	r := wire.NewReader(frame[1:])
	var m sentMacro
	m.revision = r.ReadInt32()
	r.ReadUint8()
	m.count = int(r.ReadUint8())
	if r.ReadUint8() == 1 {
		m.id = r.ReadInt32()
		m.name = r.ReadString()
		r.ReadString() // description
		r.ReadString() // acronym
		r.ReadUint8()  // icon
		for i := range int(r.ReadUint8()) {
			if n := r.ReadUint8(); int(n) != i+1 {
				t.Fatalf("line %d numbered %d", i, n)
			}
			var l macroLine
			l.typ, l.d1, l.d2, l.text = int32(r.ReadUint8()), r.ReadInt32(), int32(r.ReadUint8()), r.ReadString()
			m.lines = append(m.lines, l)
		}
	}
	if err := r.Err(); err != nil {
		t.Fatalf("parse SendMacroList: %v", err)
	}
	return m
}

// readMacroList reads one macro window refresh: a SendMacroList per macro,
// all of one revision.
func readMacroList(t *testing.T, c *testsupport.ScriptedClient) []sentMacro {
	t.Helper()
	first := parseSendMacroList(t, c.Read())
	out := []sentMacro{first}
	for len(out) < first.count {
		next := parseSendMacroList(t, c.Read())
		if next.revision != first.revision || next.count != first.count {
			t.Fatalf("macro list packet %+v does not continue %+v", next, first)
		}
		out = append(out, next)
	}
	return out
}

func readSystemMessageID(t *testing.T, c *testsupport.ScriptedClient) int32 {
	t.Helper()
	frame := c.Read()
	if frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want SystemMessage", frame[0])
	}
	return wire.NewReader(frame[1:]).ReadInt32()
}

func macroIDs(list []sentMacro) []int32 {
	var ids []int32
	for _, m := range list {
		ids = append(ids, m.id)
	}
	return ids
}

// TestMacroFlowRestoresEditsDeletesAndReloads drives the macro window over
// the wire: a stored macro comes back in the login burst, a create and an
// edit are answered with the whole list under a new revision, every refused
// edit gets its message, deleting a macro drops its shortcuts first, and
// the result survives a relog.
func TestMacroFlowRestoresEditsDeletesAndReloads(t *testing.T) {
	t.Parallel()
	logs := &logBuffer{}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithLog(zerolog.New(logs)),
		gameservertest.WithReuseDelays(0, 0),
	)
	t.Cleanup(func() {
		if t.Failed() {
			logs.mu.Lock()
			defer logs.mu.Unlock()
			t.Logf("server log:\n%s", logs.buf.String())
		}
	})
	c := srv.Client
	objID := srv.SoleObjectID(t)
	ctx := context.Background()
	macros := gamesql.NewMacroStore(srv.DB)
	if err := macros.Save(ctx, objID, macro.Macro{ID: 1000, Icon: 1, Name: "Stored", Commands: []macro.Command{{Type: macro.CommandAction, D1: 2}}}); err != nil {
		t.Fatalf("seed macro: %v", err)
	}
	macroShortcut := shortcut.Shortcut{Slot: 1, Page: 2, Type: shortcut.Macro, ID: 1000, Level: -1, CharacterType: 1}
	if err := srv.Shortcuts.Save(ctx, objID, 0, macroShortcut); err != nil {
		t.Fatalf("seed macro shortcut: %v", err)
	}

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	frames := readEnterWorldBurst(t, c)
	if got := parseSendMacroList(t, frames[0]); got.revision != 2 || got.count != 1 || got.id != 1000 || got.name != "Stored" ||
		!reflect.DeepEqual(got.lines, []macroLine{{typ: macro.CommandAction, d1: 2}}) {
		t.Fatalf("login SendMacroList = %+v, want revision 2 with the stored macro", got)
	}
	if e := findShortCut(parseShortCutInit(t, frames[11]), serverpackets.ShortcutMacro, 1000); e == nil {
		t.Fatal("login ShortCutInit lacks the macro shortcut")
	}
	drainQuiet(t, c)

	// A new macro takes the first free id from 1000 up and the whole list
	// is resent in insertion order under revision 3.
	c.Send(encodeRequestMakeMacro(0, "Fresh", "d", "F", 3, macroLine{typ: macro.CommandSkill, d1: 1177}, macroLine{typ: macro.CommandShortcut, d1: 1, d2: 4, text: "/attack"}))
	list := readMacroList(t, c)
	if list[0].revision != 3 || !reflect.DeepEqual(macroIDs(list), []int32{1000, 1001}) {
		t.Fatalf("after create: revision %d ids %v, want revision 3 ids [1000 1001]", list[0].revision, macroIDs(list))
	}

	// Editing the stored macro keeps its place; reusing its own name with
	// another case is not a clash.
	c.Send(encodeRequestMakeMacro(1000, "STORED", "", "", 1))
	list = readMacroList(t, c)
	if list[0].revision != 4 || list[0].id != 1000 || list[0].name != "STORED" || len(list[0].lines) != 0 {
		t.Fatalf("after edit: %+v, want revision 4 with macro 1000 renamed first", list)
	}

	for _, tc := range []struct {
		name    string
		payload []byte
		want    int32
	}{
		{"command text over 255", encodeRequestMakeMacro(0, "Long", "", "", 0, macroLine{typ: 3, text: strings.Repeat("x", 200)}, macroLine{typ: 3, text: strings.Repeat("y", 56)}), 810},
		{"no name", encodeRequestMakeMacro(0, "", "", "", 0), 838},
		{"name of another macro", encodeRequestMakeMacro(0, "fresh", "", "", 0), 839},
		{"description over 32", encodeRequestMakeMacro(0, "Desc", strings.Repeat("d", 33), "", 0), 837},
	} {
		c.Send(tc.payload)
		if got := readSystemMessageID(t, c); got != tc.want {
			t.Fatalf("%s: system message %d, want %d", tc.name, got, tc.want)
		}
	}

	// A shortcut for a macro the list lacks is answered, then dropped.
	c.Send(encodeRequestShortCutReg(int32(serverpackets.ShortcutMacro), wireShortcutSlot(3, 0), 4242, 1))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeShortCutRegister {
		t.Fatalf("missing-macro shortcut answer = %#x, want ShortCutRegister", reply[0])
	}

	// Deleting an unknown id answers nothing.
	c.Send(encodeRequestDeleteMacro(4242))
	if frame := c.ReadWithTimeout(rejectSilenceWindow); frame != nil {
		t.Fatalf("delete of an unknown macro answered %#x", frame[0])
	}

	// Deleting the stored macro drops its shortcut, then resends the list.
	c.Send(encodeRequestDeleteMacro(1000))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeShortCutDelete {
		t.Fatalf("delete answer = %#x, want ShortCutDelete first", reply[0])
	} else if slot := wire.NewReader(reply[1:]).ReadInt32(); slot != wireShortcutSlot(2, 1) {
		t.Fatalf("ShortCutDelete slot %d, want %d", slot, wireShortcutSlot(2, 1))
	}
	list = readMacroList(t, c)
	if list[0].revision != 5 || !reflect.DeepEqual(macroIDs(list), []int32{1001}) {
		t.Fatalf("after delete: revision %d ids %v, want revision 5 ids [1001]", list[0].revision, macroIDs(list))
	}

	srv.FlushPersistence(t)
	rows, err := macros.ListByOwner(ctx, objID)
	if err != nil {
		t.Fatalf("list macros: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != 1001 || rows[0].Name != "Fresh" || rows[0].Commands != "1,1177,0;4,1,4,/attack;" {
		t.Fatalf("stored macros = %+v, want only Fresh", rows)
	}
	scRows, err := srv.Shortcuts.ListByOwner(ctx, objID, 0)
	if err != nil {
		t.Fatalf("list shortcuts: %v", err)
	}
	for _, sc := range scRows {
		if sc.Type == shortcut.Macro {
			t.Fatalf("macro shortcut %+v survived its macro's deletion", sc)
		}
	}

	// A relog restores the stored macro under a fresh list's revision.
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeRestartResponse {
		t.Fatalf("restart opcode = %#x, want RestartResponse", reply[0])
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("post-restart opcode = %#x, want CharSelectInfo", reply[0])
	}
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	frames = readEnterWorldBurst(t, c)
	if got := parseSendMacroList(t, frames[0]); got.revision != 2 || got.count != 1 || got.id != 1001 ||
		!reflect.DeepEqual(got.lines, []macroLine{{typ: 1, d1: 1177}, {typ: 4, d1: 1, d2: 4, text: "/attack"}}) {
		t.Fatalf("relog SendMacroList = %+v, want revision 2 with Fresh", got)
	}
}
