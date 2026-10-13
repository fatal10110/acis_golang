package character

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/macro"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Strings below spell an unpaired UTF-16 surrogate in its three-byte form,
// which the test client's writer sends as that single unit:
// "\xED\xA0\xBD" is D83D, "\xED\xA0\xBE" is D83E, "\xED\xB8\x80" is DE00.

// TestMacroUnpairedSurrogatesEchoAndPersist pins client text holding an
// unpaired surrogate: the macro window echoes every unit as sent, the
// surrogate counts one unit toward the description limit, a different
// surrogate is a different name, and the stored row holds '?' for the unit
// ('c' for a leading surrogate ending the text), which is what a relog
// shows.
func TestMacroUnpairedSurrogatesEchoAndPersist(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	ctx := context.Background()

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)
	drainQuiet(t, c)

	name := "Moon\xED\xA0\xBD"
	desc := strings.Repeat("d", 15) + "\xED\xA0\xBD" + strings.Repeat("d", 16) // 32 units, the limit
	line := macroLine{typ: macro.CommandShortcut, d1: 1, d2: 4, text: "/say a\xED\xB8\x80"}
	c.Send(encodeRequestMakeMacro(0, name, desc, "M", 3, line))
	frame := c.Read()
	// The name reaches the client unit for unit: "Moon", D83D, terminator.
	if want := []byte{'M', 0, 'o', 0, 'o', 0, 'n', 0, 0x3D, 0xD8, 0, 0}; !bytes.Contains(frame, want) {
		t.Fatalf("SendMacroList % X lacks the name's units % X", frame, want)
	}
	got := parseSendMacroList(t, frame)
	if got.id != 1000 || got.name != name || !reflect.DeepEqual(got.lines, []macroLine{line}) {
		t.Fatalf("SendMacroList = %+v, want macro 1000 %q with %+v", got, name, line)
	}

	// The same name in another case clashes; another surrogate does not.
	c.Send(encodeRequestMakeMacro(0, "MOON\xED\xA0\xBD", "", "", 0))
	if id := readSystemMessageID(t, c); id != 839 {
		t.Fatalf("same name in capitals: system message %d, want 839", id)
	}
	other := "moon\xED\xA0\xBE"
	c.Send(encodeRequestMakeMacro(0, other, "", "", 0))
	list := readMacroList(t, c)
	if !reflect.DeepEqual(macroIDs(list), []int32{1000, 1001}) || list[1].name != other {
		t.Fatalf("after second macro: %+v, want 1000 and 1001 %q", list, other)
	}

	srv.FlushPersistence(t)
	rows, err := gamesql.NewMacroStore(srv.DB).ListByOwner(ctx, objID)
	if err != nil {
		t.Fatalf("list macros: %v", err)
	}
	wantRows := []macro.Row{
		{ID: 1000, Icon: 3, Name: "Moonc", Description: strings.Repeat("d", 15) + "?" + strings.Repeat("d", 16), Acronym: "M", Commands: "4,1,4,/say a?;", CommandsValid: true},
		{ID: 1001, Name: "moonc", CommandsValid: true},
	}
	if !reflect.DeepEqual(rows, wantRows) {
		t.Fatalf("stored macros = %+v, want %+v", rows, wantRows)
	}

	// A relog reads the stored text back; one macro keeps its list to one
	// packet.
	c.Send(encodeRequestDeleteMacro(1001))
	if list := readMacroList(t, c); !reflect.DeepEqual(macroIDs(list), []int32{1000}) {
		t.Fatalf("after delete: ids %v, want [1000]", macroIDs(list))
	}
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
	frames := readEnterWorldBurst(t, c)
	if got := parseSendMacroList(t, frames[0]); got.id != 1000 || got.name != "Moonc" ||
		!reflect.DeepEqual(got.lines, []macroLine{{typ: macro.CommandShortcut, d1: 1, d2: 4, text: "/say a?"}}) {
		t.Fatalf("relog SendMacroList = %+v, want macro 1000 Moonc", got)
	}
}
