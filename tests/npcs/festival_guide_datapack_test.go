package npcs

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// shippedFestivalPages are the shipped festival guide pages.
func shippedFestivalPages(t *testing.T) map[string]string {
	t.Helper()
	dir := datapack.Path(t, "data", "html", "seven_signs", "festival")
	matches, err := filepath.Glob(filepath.Join(dir, "*.htm"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("glob festival pages: %v (%d)", err, len(matches))
	}
	pages := map[string]string{}
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		pages["seven_signs/festival/"+filepath.Base(path)] = string(raw)
	}
	return pages
}

// Walked through the shipped pages, a Dawn guide's links reach the
// commands in place: its description pages, the high scores, the standings
// and the bonuses, whose Chat links go back to the guide page. The shipped standings and bonus pages are filled
// with the cycle's scores and bonuses, here all blank and zero.
func TestFestivalGuideShippedPages(t *testing.T) {
	t.Parallel()
	datapack.Require(t)
	boot := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	w := bootFolkWorld(t, shippedFestivalPages(t), noBypassReuse,
		gameservertest.WithFestivalClock(func() time.Time { return boot }),
		withFestivalRows(t, [festival.Count]int{}),
	)
	guide := w.spawnFolk(t, folkTemplate("FestivalGuide", 31129), 30)
	shipped := func(name string) string {
		t.Helper()
		page, ok := w.srv.HTML.Get("data/html/seven_signs/festival/" + name)
		if !ok {
			t.Fatalf("no shipped page %s", name)
		}
		return strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(guide.ObjectID())))
	}
	pageAnswer := []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}
	countdown := `<font color="FF0000">The next festival will begin in 22 minute(s).</font>`
	guidePage := strings.ReplaceAll(shipped("dawn_guide.htm"), "%festivalMins%", countdown)
	if got := w.talkPage(t, guide, false); got != guidePage {
		t.Fatalf("guide page = %q, want %q", got, guidePage)
	}

	for _, n := range []string{"1", "2", "3", "4", "5"} {
		w.talkPage(t, guide, false)
		assertAnswer(t, w.bypass(t, npcCommand(guide, "FestivalDesc "+n)), pageAnswer, guide, shipped("desc_"+n+".htm"))
	}
	// desc_5's Chat link leads back to the guide page.
	assertAnswer(t, w.bypass(t, npcCommand(guide, "Chat 0")), pageAnswer, guide, guidePage)

	var stats, bonuses strings.Builder
	for _, name := range festival.Names {
		stats.WriteString(`<tr><td width="100" align="center">` + name + `</td><td align="center" width="35">0</td><td align="center" width="35">0</td><td align="center" width="130">None</td></tr>`)
		bonuses.WriteString(`<tr><td align="center" width="150">` + name + `</td><td align="center" width="150">0</td></tr>`)
	}
	w.talkPage(t, guide, false)
	assertAnswer(t, w.bypass(t, npcCommand(guide, "Festival 5")), pageAnswer, guide, strings.ReplaceAll(shipped("festival_5.htm"), "%statsTable%", stats.String()))
	w.talkPage(t, guide, false)
	assertAnswer(t, w.bypass(t, npcCommand(guide, "Festival 6")), pageAnswer, guide, strings.ReplaceAll(shipped("festival_6.htm"), "%bonusTable%", bonuses.String()))
	w.talkPage(t, guide, false)
	frames := w.bypass(t, npcCommand(guide, "Festival 4"))
	if got := opcodes(frames); string(got) != string([]byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}) {
		t.Fatalf("high scores answer = %x", got)
	}
	if _, page, _ := htmlMessage(t, frames[0]); !strings.Contains(page, "for the Level 53 or lower festival.<br>Dawn: No record exists.") {
		t.Fatalf("high scores page = %q", page)
	}
	assertAnswer(t, w.bypass(t, npcCommand(guide, "Chat 0")), pageAnswer, guide, guidePage)
}
