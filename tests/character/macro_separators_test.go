package character

import (
	"context"
	"reflect"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/macro"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestMacroSeparatorsSurviveRelogAndBadRowsAreSkipped covers the two
// owner-approved divergences of #5061: macro text holding ',' or ';' is
// stored escaped and restores unchanged after a relog, and a stored row
// that does not decode is left out of the login restore without dropping
// the macros after it. Text without separators keeps its stored form.
func TestMacroSeparatorsSurviveRelogAndBadRowsAreSkipped(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	ctx := context.Background()
	macros := gamesql.NewMacroStore(srv.DB)

	// An undecodable row ahead of a good one.
	if _, err := srv.DB.ExecContext(ctx,
		`INSERT INTO character_macroses (char_obj_id, id, icon, name, descr, acronym, commands) VALUES (?, 2000, 1, 'Broken', '', '', '1,z,3;')`,
		objID); err != nil {
		t.Fatalf("seed broken macro: %v", err)
	}
	if err := macros.Save(ctx, objID, macro.Macro{ID: 2001, Icon: 1, Name: "Later", Commands: []macro.Command{{Type: macro.CommandAction, D1: 2}}}); err != nil {
		t.Fatalf("seed macro: %v", err)
	}

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	frames := readEnterWorldBurst(t, c)
	if got := parseSendMacroList(t, frames[0]); got.count != 1 || got.id != 2001 || got.name != "Later" {
		t.Fatalf("login SendMacroList = %+v, want only Later: the broken row is skipped, not the rows after it", got)
	}
	drainQuiet(t, c)

	created := []struct {
		name  string
		lines []macroLine
	}{
		{"One", []macroLine{{typ: macro.CommandAction, text: "/say one"}}},
		{"Two", []macroLine{
			{typ: macro.CommandAction, text: "/say hello, world"},
			{typ: macro.CommandShortcut, d1: 1, d2: 4, text: "/say hi;oops,1,2"},
		}},
		{"Three", []macroLine{{typ: macro.CommandAction, text: `/say three \ end\`}}},
	}
	for _, m := range created {
		c.Send(encodeRequestMakeMacro(0, m.name, "", "", 1, m.lines...))
		readMacroList(t, c)
	}

	srv.FlushPersistence(t)
	rows, err := macros.ListByOwner(ctx, objID)
	if err != nil {
		t.Fatalf("list macros: %v", err)
	}
	wantRows := map[int32]string{
		1000: "3,0,0,/say one;",
		1001: `3,0,0,/say hello\, world;4,1,4,/say hi\;oops\,1\,2;`,
		1002: `3,0,0,/say three \ end\\;`,
		2000: "1,z,3;",
		2001: "3,2,0;",
	}
	gotRows := map[int32]string{}
	for _, row := range rows {
		gotRows[row.ID] = row.Commands
	}
	if !reflect.DeepEqual(gotRows, wantRows) {
		t.Fatalf("stored commands = %q, want %q", gotRows, wantRows)
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
	list := readMacroList(t, c)
	drainQuiet(t, c)
	if want := []int32{1000, 1001, 1002, 2001}; !reflect.DeepEqual(macroIDs(list), want) {
		t.Fatalf("relog macro ids %v, want %v", macroIDs(list), want)
	}
	for i, m := range created {
		if got := list[i]; got.name != m.name || !reflect.DeepEqual(got.lines, m.lines) {
			t.Errorf("relog macro %d = %+v, want %s with lines %+v", got.id, got, m.name, m.lines)
		}
	}
}
