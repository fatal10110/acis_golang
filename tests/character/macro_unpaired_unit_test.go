package character

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/macro"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// encodeRawMacro is RequestMakeMacro with the name, description and the one
// line's text given as raw UTF-16 code units, so a test can send an
// unpaired surrogate.
func encodeRawMacro(name, desc []uint16, line macroLine, text []uint16) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMakeMacro)
	w.WriteInt32(0)
	w.WriteUTF16(name)
	w.WriteUTF16(desc)
	w.WriteString("M")
	w.WriteUint8(3)
	w.WriteUint8(1)
	w.WriteUint8(1)
	w.WriteUint8(uint8(line.typ))
	w.WriteInt32(line.d1)
	w.WriteUint8(uint8(line.d2))
	w.WriteUTF16(text)
	return w.Bytes()
}

func units(s string, extra ...uint16) []uint16 {
	var out []uint16
	for _, r := range s {
		out = append(out, uint16(r))
	}
	return append(out, extra...)
}

// TestMacroUnpairedSurrogateReadsAsReplacement pins client text holding an
// unpaired UTF-16 surrogate: the unit reads as U+FFFD, which the macro
// window echoes, which counts one unit toward the 32-unit description
// limit, and which the macro row stores as it is. Every unpaired surrogate
// reads the same, so a name differing from another only in which one it
// holds clashes with it.
func TestMacroUnpairedSurrogateReadsAsReplacement(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)
	drainQuiet(t, c)

	desc := units(strings.Repeat("d", 15), 0xDE00)
	desc = append(desc, units(strings.Repeat("d", 16))...) // 32 units, the limit
	line := macroLine{typ: macro.CommandShortcut, d1: 1, d2: 4}
	c.Send(encodeRawMacro(units("Moon", 0xD83D), desc, line, units("/say a", 0xDE00)))
	frame := c.Read()
	// The name reaches the client as "Moon", U+FFFD, terminator.
	if want := []byte{'M', 0, 'o', 0, 'o', 0, 'n', 0, 0xFD, 0xFF, 0, 0}; !bytes.Contains(frame, want) {
		t.Fatalf("SendMacroList % X lacks the name's units % X", frame, want)
	}
	line.text = "/say a�"
	if got := parseSendMacroList(t, frame); got.id != 1000 || got.name != "Moon�" || !reflect.DeepEqual(got.lines, []macroLine{line}) {
		t.Fatalf("SendMacroList = %+v, want macro 1000 %q with %+v", got, "Moon�", line)
	}

	// Another unpaired surrogate, in capitals: the same name.
	c.Send(encodeRawMacro(units("MOON", 0xD83E), nil, line, nil))
	if id := readSystemMessageID(t, c); id != 839 {
		t.Fatalf("name with another unpaired surrogate: system message %d, want 839", id)
	}

	srv.FlushPersistence(t)
	rows, err := gamesql.NewMacroStore(srv.DB).ListByOwner(context.Background(), objID)
	if err != nil {
		t.Fatalf("list macros: %v", err)
	}
	wantRows := []macro.Row{{
		ID: 1000, Icon: 3, Name: "Moon�", Description: strings.Repeat("d", 15) + "�" + strings.Repeat("d", 16),
		Acronym: "M", Commands: "4,1,4,/say a�;", CommandsValid: true,
	}}
	if !reflect.DeepEqual(rows, wantRows) {
		t.Fatalf("stored macros = %+v, want %+v", rows, wantRows)
	}
}
