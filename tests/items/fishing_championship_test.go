package items

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/fishchamp"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

func encodeRequestExFishRanking() []byte {
	return []byte{clientpackets.OpcodeExtended, byte(clientpackets.OpcodeRequestExFishRanking), byte(clientpackets.OpcodeRequestExFishRanking >> 8)}
}

// championshipPage reads a shipped championship page as the page cache
// holds it.
func championshipPage(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(datapack.Path(t, strings.Split(path, "/")...))
	if err != nil {
		t.Fatal(err)
	}
	page := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasSuffix(page, "\n") {
		page += "\n"
	}
	return page
}

// rankingAnswer sends the fishing window's ranking button and returns the
// page it opens, object id 0, or "" when nothing answers.
func rankingAnswer(t *testing.T, r *fishingRig) string {
	t.Helper()
	r.c.Send(encodeRequestExFishRanking())
	frames := collectUntilQuiet(t, r.c)
	if len(frames) == 0 {
		return ""
	}
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeNpcHtmlMessage {
		t.Fatalf("ranking answer = %d frames, first %#x; want one NpcHtmlMessage", len(frames), frames[0][0])
	}
	rd := wire.NewReader(frames[0][1:])
	if objectID := rd.ReadInt32(); objectID != 0 {
		t.Fatalf("ranking page object id = %d, want 0", objectID)
	}
	return rd.ReadString()
}

// TestFishingChampionshipMeasuresCatchAndRanks pins the championship's
// part in fishing: with it enabled ExFishingStart shows the ranking button;
// a caught fish is measured after it is added, CAUGHT_FISH_S1_LENGTH naming
// its length (60 + 10 + 345/1000), then REGISTERED_IN_FISH_SIZE_RANKING as
// it joins the empty ranking. RequestExFishRanking first answers that the
// ranking is being taken (fish_event003), then, within the minute, shows it
// (fish_event002): the fisher first, the prizes in adena.
func TestFishingChampionshipMeasuresCatchAndRanks(t *testing.T) {
	t.Parallel()
	rolls := []int{10, 345}
	roll := func(n int) int {
		if len(rolls) == 0 {
			t.Errorf("championship roll(%d) with none scripted", n)
			return 0
		}
		v := rolls[0]
		rolls = rolls[1:]
		return v
	}
	r := bootFishing(t,
		gameservertest.WithFishingChampionship(fishchamp.DefaultConfig(), nil, fishchamp.WithRoll(roll)),
		gameservertest.WithHTMLPages(map[string]string{
			"fisherman/championship/fish_event002.htm": championshipPage(t, fishchamp.PageRunning),
			"fisherman/championship/fish_event003.htm": championshipPage(t, fishchamp.PageRefreshing),
		}))
	id := r.objID

	requireEvents(t, "cast", r.cast(t, fishingSkill), append(castStart(fishingSkill),
		"msg 1461",
		fmt.Sprintf("start %d type 1 at 260,20,10 night 0 ranking 1", id),
		"sound 1 SF_P_01")...)
	r.dice.check.Store(50)
	r.awaitEvent(t, "combat ")
	fishingEvents(t, r.c)
	for range 3 {
		r.cast(t, pumpingSkill)
	}
	requireEvents(t, "catch", r.cast(t, pumpingSkill), append(castStart(pumpingSkill),
		"msg 1465 26",
		"gauge hp 0 mode 0 good 1 anim 1 penalty 0 deceptive 0",
		"msg 1469",
		fmt.Sprintf("msg 30 %d", fishCaughtID),
		"msg 1847 70.345",
		"msg 1848",
		fmt.Sprintf("end %d win 1", id),
		"msg 1460")...)

	if got, want := rankingAnswer(t, r), championshipPage(t, fishchamp.PageRefreshing); got != want {
		t.Fatalf("first ranking = %q, want fish_event003", got)
	}
	want := championshipPage(t, fishchamp.PageRunning)
	table := "<tr><td width=70 align=center>1</td><td width=110 align=center>Fisher</td><td width=80 align=center>70.345</td></tr>"
	for place := 2; place <= 5; place++ {
		table += fmt.Sprintf("<tr><td width=70 align=center>%d</td><td width=110 align=center>None</td><td width=80 align=center>0</td></tr>", place)
	}
	for _, kv := range [][2]string{{"%TABLE%", table}, {"%prizeItem%", "Adena"}, {"%prizeFirst%", "800000"}, {"%prizeTwo%", "500000"}, {"%prizeThree%", "300000"}, {"%prizeFour%", "200000"}, {"%prizeFive%", "100000"}} {
		want = strings.ReplaceAll(want, kv[0], kv[1])
	}
	if got := rankingAnswer(t, r); got != want {
		t.Fatalf("second ranking = %q, want %q", got, want)
	}
}

// TestFishingChampionshipDisabledIsSilent pins the championship switched
// off, as the fishing suites boot: ExFishingStart hides the ranking button,
// a catch is not measured (TestFishingCastBiteFightAndCatch), and
// RequestExFishRanking, which no button can send, answers nothing.
func TestFishingChampionshipDisabledIsSilent(t *testing.T) {
	t.Parallel()
	r := bootFishing(t)
	if got := rankingAnswer(t, r); got != "" {
		t.Fatalf("ranking while disabled = %q, want no answer", got)
	}
}
