package party

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// shippedFestivalPages are the shipped festival guide pages.
func shippedFestivalPages(t *testing.T) map[string]string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(datapack.Path(t, "data", "html", "seven_signs", "festival"), "*.htm"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("glob festival pages: %v (%d)", err, len(matches))
	}
	pages := map[string]string{}
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		pages["seven_signs/festival/"+filepath.Base(path)] = string(raw)
	}
	return pages
}

// pageHTML is the html of the one NpcHtmlMessage among frames.
func pageHTML(t *testing.T, frames [][]byte) string {
	t.Helper()
	frame, ok := firstFrame(frames, serverpackets.OpcodeNpcHtmlMessage)
	if !ok {
		t.Fatalf("answer %x opens no page", opcodes(frames))
	}
	r := wire.NewReader(frame[1:])
	r.ReadInt32()
	return r.ReadString()
}

// Walked through the shipped pages, signing up reaches the signup pages:
// a player in no party sees festival_2b.htm naming the two-member minimum;
// a party leader sees festival_1.htm with the 900, 540 and 270 stones of
// each color, whose blue-stone link pays the fee and opens
// festival_2e.htm.
func TestFestivalShippedSignupPages(t *testing.T) {
	t.Parallel()
	datapack.Require(t)
	pages := shippedFestivalPages(t)
	f := bootFestivalWith(t, festivalSetup{seats: []seat{{"Leader", 20}, {"Member", 20}}, blueStones: 1000, minPlayers: 2, pages: pages})
	shipped := func(name string, fill ...string) string {
		t.Helper()
		page, ok := f.srv.HTML.Get("data/html/seven_signs/festival/" + name)
		if !ok {
			t.Fatalf("no shipped page %s", name)
		}
		page = strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(f.guide.ObjectID())))
		for i := 0; i+1 < len(fill); i += 2 {
			page = strings.ReplaceAll(page, fill[i], fill[i+1])
		}
		return page
	}

	if got, want := pageHTML(t, f.command(t, 0, "1")), shipped("festival_2b.htm", "%minFestivalPartyMembers%", "2"); got != want {
		t.Fatalf("partyless signup page = %q, want %q", got, want)
	}
	f.quiet(t)
	f.invite(t, 0, 1, 0)
	if got, want := pageHTML(t, f.command(t, 0, "1")), shipped("festival_1.htm", "%blueStoneNeeded%", "900", "%greenStoneNeeded%", "540", "%redStoneNeeded%", "270"); got != want {
		t.Fatalf("fee page = %q, want %q", got, want)
	}
	leader := f.players[0]
	leader.c.Send(encodeBypass(f.guide, "Festival 2 6360"))
	answer := drainFrames(t, leader.c)
	requireMessage(t, "leader", answer, serverpackets.SystemMessageS2S1Disappeared, int32(blueStoneID), int32(900))
	if got, want := pageHTML(t, answer), shipped("festival_2e.htm"); got != want {
		t.Fatalf("payment page = %q, want %q", got, want)
	}
	if !f.srv.Festival.IsParticipant(leader.id) {
		t.Fatal("the shipped fee link signed nobody up")
	}
}
