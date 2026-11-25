package npcs

import (
	"context"
	"os"
	"slices"
	"strconv"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// A scheme created, edited or deleted is written as it changes, so a
// server stopped with no shutdown save restarts with the schemes as last
// changed; a refused create leaves them as they were.
// Deleting a player's last scheme removes its rows (owner-approved
// divergence, #4699).
func TestSchemeBufferChangesOutliveAStopWithoutTheShutdownSave(t *testing.T) {
	t.Parallel()
	w, f := storedSchemeBufferWorld(t, true)
	w.talkTo(t, f)
	for _, command := range []string{
		"support", "createscheme Fight", "createscheme Tank",
		"editschemes Buffs Fight 1", "skillselect Buffs Fight 1035 1",
		"editschemes Buffs Fight 2", "skillselect Buffs Fight 1068 2",
		"editschemes Buffs Fight 1", "skillunselect Buffs Fight 1035 1",
		"support", "editschemes Buffs Tank 1", "skillselect Buffs Tank 1036 1",
		"support",
	} {
		if frames := w.bypass(t, npcCommand(f, command)); len(frames) == 0 {
			t.Fatalf("%s: no answer", command)
		}
	}
	assertFrames(t, "createscheme again", w.bypass(t, npcCommand(f, "createscheme fIGHT")),
		noticeFrame("The scheme name already exists."), []byte{serverpackets.OpcodeActionFailed})
	assertFrames(t, "createscheme too long", w.bypass(t, npcCommand(f, "createscheme abcdefghijklmno")),
		noticeFrame("Scheme's name must contain up to 14 chars. Spaces are trimmed."), []byte{serverpackets.OpcodeActionFailed})
	w.bypass(t, npcCommand(f, "deletescheme Tank"))
	w.srv.Halt(t)
	assertStoredSchemes(t, w, []schemebuffer.Row{{OwnerID: w.player, Name: "Fight", Skills: "1068"}})

	w, f = storedSchemeBufferWorld(t, false)
	oid := strconv.Itoa(int(f.ObjectID()))
	w.talkTo(t, f)
	listed := `<font color="LEVEL">Fight [1 / 20] - cost: 2,500</font><br1>` +
		`<a action="bypass npc_` + oid + `_givebuffs Fight 2500">Use on Me</a>&nbsp;|&nbsp;` +
		`<a action="bypass npc_` + oid + `_givebuffs Fight 2500 pet">Use on Pet</a>&nbsp;|&nbsp;` +
		`<a action="bypass npc_` + oid + `_editschemes Buffs Fight 1">Edit</a>&nbsp;|&nbsp;` +
		`<a action="bypass npc_` + oid + `_deletescheme Fight">Delete</a><br>`
	assertSchemePage(t, "support after the restart", w.bypass(t, npcCommand(f, "support")),
		datapackPage(t, f, "50002-1.htm", "%schemes%", listed, "%max_schemes%", "4"))

	w.bypass(t, npcCommand(f, "deletescheme Fight"))
	w.srv.Halt(t)
	assertStoredSchemes(t, w, nil)
}

// storedSchemeBufferWorld is schemeBufferWorld with the schemes stored in
// and restored from the fixture database, holding no adena. A first boot
// seeds the talker; a restart plays the one it stored.
func storedSchemeBufferWorld(t *testing.T, first bool) (*folkWorld, *npc.Folk) {
	t.Helper()
	stored := storedSchemeBuffer(t)
	if first {
		return schemeBufferWorld(t, nil, 0, stored)
	}
	pages := dialogPages()
	for _, name := range []string{"50002.htm", "50002-1.htm", "50002-2.htm", "50002-3.htm"} {
		data, err := os.ReadFile(datapack.Path(t, "data", "html", "mods", "buffer", name))
		if err != nil {
			t.Fatal(err)
		}
		pages["mods/buffer/"+name] = string(data)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(pages),
		stored,
		noBypassReuse,
	)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	startInWorld(t, w.srv, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	return w, w.spawnFolk(t, folkTemplate("SchemeBuffer", schemeBufferID), 60)
}

// storedSchemeBuffer wires the shipped buffer's schemes to the fixture
// database: loaded at boot, written as they change.
func storedSchemeBuffer(t *testing.T) gameservertest.Option {
	t.Helper()
	return gameservertest.WithStoredSchemeBuffer(func(store schemebuffer.Option) *schemebuffer.Manager {
		return schemebuffer.New(schemebuffer.DefaultConfig(), schemeBufferBuffs(t), schemeBufferSkills(t), store)
	})
}

// assertStoredSchemes checks every buffer_schemes row against want.
func assertStoredSchemes(t *testing.T, w *folkWorld, want []schemebuffer.Row) {
	t.Helper()
	got, err := gamesql.NewBufferSchemeStore(w.srv.DB).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("stored schemes = %+v, want %+v", got, want)
	}
}
