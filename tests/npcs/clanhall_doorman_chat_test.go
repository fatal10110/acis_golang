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
// Moonstone Hall (22) lists the doorman 35385.

const hallDoormanID = 35385

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
		name  string
		setup hallManagerSetup
		file  string
		pairs []string
	}{
		{"owner", hallManagerSetup{}, "doormen.htm", []string{"%clanname%", "Hallkeepers"}},
		{"stranger", hallManagerSetup{foreign: true, rivalLeader: true}, "doormen-no.htm", []string{"%leadername%", "Rook", "%clanname%", "Rivals"}},
		{"leaderless owner", hallManagerSetup{foreign: true}, "emptyowner.htm", []string{"%hallname%", "Moonstone Hall"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			c.setup.extra = []gameservertest.Option{gameservertest.WithHTMLPages(map[string]string{
				"clanHallDoormen/" + c.file: hallDoormanPage(t, c.file),
			})}
			w := bootHallManager(t, c.setup)
			doorman := w.spawnFolk(t, folkTemplate("ClanHallDoorman", hallDoormanID), 40)
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
