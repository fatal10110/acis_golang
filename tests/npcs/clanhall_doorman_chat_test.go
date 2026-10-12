package npcs

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: ClanHallDoorman.showChatWindow (ClanHallDoorman.java:109-146)
// sends ActionFailed, then, for a member of the clan owning the hall,
// doormen.htm (doormen_wyvern.htm at Aden's halls 36-41) with %clanname%;
// for anyone else doormen-no.htm naming the owner's leader and clan, or
// emptyowner.htm with %hallname% when no clan with a leader owns the hall.
// A doorman of no hall only sends ActionFailed (ClanHallDoorman.java:111-114).
// Moonstone Hall (22) lists the doorman 35385, Golden Chamber (36), an Aden
// hall, the manager 35439 and the doorman 35440.
//
// CastleDoorman.isOwnerClan (CastleDoorman.java:41-53) opens
// doormen/<npcId>.htm at a siegable hall to a member of the owning clan
// holding CHP_ENTRY_EXIT_RIGHTS, and <npcId>-no.htm to anyone else, then
// ActionFailed. Devastated Castle (34), a siegable hall, lists the doorman
// 35417 and the manager 35421.

const (
	hallDoormanID      = 35385
	goldenChamber      = 36
	goldenManagerID    = 35439
	goldenDoormanID    = 35440
	devastatedCastle   = 34
	devastatedManager  = 35421
	devastatedDoorman  = 35417
	unlistedResidentID = 39999
)

// hallDoormanPage is the shipped clan hall doorman page as the page cache
// holds it.
func hallDoormanPage(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(datapack.Path(t, "data", "html", "clanHallDoormen", name))
	if err != nil {
		t.Fatal(err)
	}
	page := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
	if !strings.HasSuffix(page, "\n") {
		page += "\n"
	}
	return page
}

// TestClanHallDoormanChatWindow pins the doorman's page for a member of
// the owning clan, for a stranger to a hall whose owner has a leader, and
// for a stranger to a hall whose owner has none, each after ActionFailed.
func TestClanHallDoormanChatWindow(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name    string
		setup   hallManagerSetup
		doorman int
		file    string
		pairs   []string
	}{
		{"owner", hallManagerSetup{}, hallDoormanID, "doormen.htm", []string{"%clanname%", "Hallkeepers"}},
		{
			"owner at an Aden hall",
			hallManagerSetup{hall: goldenChamber, manager: goldenManagerID},
			goldenDoormanID,
			"doormen_wyvern.htm",
			[]string{"%clanname%", "Hallkeepers"},
		},
		{"stranger", hallManagerSetup{foreign: true, rivalLeader: true}, hallDoormanID, "doormen-no.htm", []string{"%leadername%", "Rook", "%clanname%", "Rivals"}},
		{"leaderless owner", hallManagerSetup{foreign: true}, hallDoormanID, "emptyowner.htm", []string{"%hallname%", "Moonstone Hall"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			c.setup.extra = []gameservertest.Option{gameservertest.WithHTMLPages(map[string]string{
				"clanHallDoormen/" + c.file: hallDoormanPage(t, c.file),
			})}
			w := bootHallManager(t, c.setup)
			doorman := w.spawnFolk(t, folkTemplate("ClanHallDoorman", c.doorman), 40)
			w.selectFolk(t, doorman)
			frames := w.talk(t, doorman, false)
			want := []byte{
				serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeSocialAction,
				serverpackets.OpcodeActionFailed, serverpackets.OpcodeNpcHtmlMessage,
			}
			if got := interactOrder(frames); !bytes.Equal(got, want) {
				t.Fatalf("talk = % x, want % x", got, want)
			}
			wantPage := fill(hallDoormanPage(t, c.file), append(c.pairs, "%objectId%", strconv.Itoa(int(doorman.ObjectID())))...)
			if got := pageIn(t, frames); got != wantPage {
				t.Fatalf("page =\n%s\nwant\n%s", got, wantPage)
			}
		})
	}
}

// TestClanHallDoormanOfNoHall pins that a clan hall doorman listed by no
// hall only releases the client: no page follows the interact.
func TestClanHallDoormanOfNoHall(t *testing.T) {
	t.Parallel()
	w := bootHallManager(t, hallManagerSetup{})
	doorman := w.spawnFolk(t, folkTemplate("ClanHallDoorman", unlistedResidentID), 40)
	w.selectFolk(t, doorman)
	want := []byte{
		serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeSocialAction,
		serverpackets.OpcodeActionFailed,
	}
	if got := interactOrder(w.talk(t, doorman, false)); !bytes.Equal(got, want) {
		t.Fatalf("talk = % x, want % x", got, want)
	}
}

// TestSiegableHallDoormanChatWindow pins the castle doorman page at a
// siegable hall: its welcome for the leader of the owning clan and for a
// member holding the hall's entry rights, its refusal for a member holding
// only the castle's entry rights and for a stranger, each then ActionFailed.
func TestSiegableHallDoormanChatWindow(t *testing.T) {
	t.Parallel()
	const castleEntryExit = 32768
	welcome, refusal := "doormen/"+strconv.Itoa(devastatedDoorman)+".htm", "doormen/"+strconv.Itoa(devastatedDoorman)+"-no.htm"
	for _, c := range []struct {
		name  string
		setup hallManagerSetup
		file  string
	}{
		{"leader", hallManagerSetup{}, welcome},
		{"hall entry rights", hallManagerSetup{rank: 5, privs: privHallEntryExit}, welcome},
		{"castle entry rights", hallManagerSetup{rank: 5, privs: castleEntryExit}, refusal},
		{"stranger", hallManagerSetup{foreign: true}, refusal},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			c.setup.hall, c.setup.manager = devastatedCastle, devastatedManager
			c.setup.extra = []gameservertest.Option{gameservertest.WithHTMLPages(map[string]string{
				welcome: residencePage(welcome), refusal: residencePage(refusal),
			})}
			w := bootHallManager(t, c.setup)
			doorman := w.spawnFolk(t, folkTemplate("CastleDoorman", devastatedDoorman), 40)
			w.selectFolk(t, doorman)
			frames := w.talk(t, doorman, false)
			want := []byte{
				serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeSocialAction,
				serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed,
			}
			if got := interactOrder(frames); !bytes.Equal(got, want) {
				t.Fatalf("talk = % x, want % x", got, want)
			}
			if got, want := pageIn(t, frames), wantChatPage(residencePage(c.file), doorman); got != want {
				t.Fatalf("page = %q, want %q", got, want)
			}
		})
	}
}
