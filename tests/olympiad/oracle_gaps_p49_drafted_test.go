package olympiad

import (
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// managerPages are the shipped pages of the Grand Olympiad Manager.
func managerPages(t *testing.T) map[string]string {
	t.Helper()
	dir := datapack.Path(t, "data", "html", "olympiad")
	matches, err := filepath.Glob(filepath.Join(dir, "*.htm"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("glob olympiad pages: %v (%d)", err, len(matches))
	}
	pages := map[string]string{}
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		pages["olympiad/"+filepath.Base(path)] = string(raw)
	}
	return pages
}

// duelistClass is the Duelist, a third class the manager registers.
const duelistClass = 88

// TestOlympiadManagerWhileDraftedIntoMatch has a noble, drafted into a
// match that is yet to open its stadium, ask the Grand Olympiad Manager to
// register it again and to take it off the waiting list
// (OlympiadManager.registerNoble and unRegisterNoble, OlympiadManager.java
// isInCompetition): either registration is refused with
// ALREADY_REGISTERED_ON_THE_EVENT_WAITING_LIST, the unregistration is
// answered with YOU_HAVE_NOT_BEEN_REGISTERED_IN_A_WAITING_LIST_OF_A_GAME
// (the draw already took the noble off the queue, and the registered check
// precedes the in-competition one), and the noble stays drafted.
func TestOlympiadManagerWhileDraftedIntoMatch(t *testing.T) {
	t.Parallel()
	duelist := gameservertest.ClassTemplate()
	duelist.ID = duelistClass
	duelist.Skills = nil
	m := bootMatchPrepared(t, nil, func(srv *gameservertest.Server) {
		if _, err := srv.DB.Exec(`UPDATE characters SET classid = ?, base_class = ? WHERE char_name = 'Alpha'`, duelistClass, duelistClass); err != nil {
			t.Fatalf("make Alpha a duelist: %v", err)
		}
	}, gameservertest.WithOlympiadSeed(makeNobles), gameservertest.WithClassTemplates(duelist),
		gameservertest.WithHTMLPages(managerPages(t)), gameservertest.WithReuseDelays(0, 0))
	a := m.ones[0]
	home := m.start[0]
	manager := m.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("OlympiadManagerNpc", 31688),
		location.Location{X: home.X + 30, Y: home.Y, Z: home.Z})
	m.read(t)
	m.drawn(t)

	send := func(payload []byte) [][]byte {
		t.Helper()
		from := len(a.frames)
		a.c.Send(payload)
		m.srv.Advance(t, 50*time.Millisecond)
		m.read(t)
		m.srv.Advance(t, 50*time.Millisecond)
		m.read(t)
		return a.frames[from:]
	}
	send(encodeSelect(manager.ObjectID(), home))
	send(encodeSelect(manager.ObjectID(), home))
	bypass := func(command string) [][]byte {
		t.Helper()
		return send(encodeHeroBypass("npc_" + strconv.Itoa(int(manager.ObjectID())) + "_" + command))
	}
	for _, tc := range []struct {
		route  []string
		choice string
	}{
		// The unregistration comes first: the manager's main page, open
		// after the two selections, is the one offering its link.
		{nil, "1"}, {[]string{"Chat 10", "Chat 11"}, "4"}, {[]string{"Chat 10", "Chat 12"}, "5"},
	} {
		choice := tc.choice
		for _, link := range tc.route {
			if len(bypass(link)) == 0 {
				t.Fatalf("%s opened no page on the way to OlympiadNoble %s", link, choice)
			}
		}
		frames := send(encodeHeroBypass("npc_" + strconv.Itoa(int(manager.ObjectID())) + "_OlympiadNoble " + choice))
		// The entry countdown of the match runs meanwhile.
		var ids []int
		for _, f := range frames {
			if f[0] == serverpackets.OpcodeSystemMessage {
				if id := int(wire.NewReader(f[1:]).ReadInt32()); id != serverpackets.SystemMessageEnterOlympiadStadiumInS1Seconds {
					ids = append(ids, id)
				}
			}
		}
		want := []int{serverpackets.SystemMessageAlreadyRegisteredInEventWaitingList}
		if choice == "1" {
			want = []int{serverpackets.SystemMessageNotRegisteredInGameWaitingList}
		}
		if !slices.Equal(ids, want) {
			t.Fatalf("OlympiadNoble %s answered %d frames (messages %v), want messages %v", choice, len(frames), ids, want)
		}
	}
	if !m.srv.Olympiad.IsRegisteredInComp(a.id) {
		t.Fatal("Alpha was taken off the match it is drafted into")
	}
}

// makeNobles makes both competitors nobles, as the manager's registration
// asks of the character itself.
func makeNobles(db *sql.DB) {
	if _, err := db.Exec(`UPDATE characters SET nobless = 1`); err != nil {
		panic(err)
	}
}
