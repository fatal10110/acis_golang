package npcs

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// festivalCommands are the guide commands these tests send; every guide
// and witch page links each of them.
var festivalCommands = []string{
	"Festival 0", "Festival 3", "Festival 4", "Festival 5", "Festival 6", "Festival 7", "Festival", "FestivalDesc 1", "Chat 9",
}

// festivalGuidePages are a guide page linking every festival command, as
// each oracle's guides and the witches show it, and the festival pages the
// commands open. Pages 0a and 0b are missing, as in the shipped datapack.
func festivalGuidePages() map[string]string {
	var links strings.Builder
	for _, c := range festivalCommands {
		links.WriteString(`<a action="bypass -h npc_%objectId%_` + c + `">` + c + `</a>`)
	}
	guide := "<html><body>guide %festivalMins%" + links.String() + "</body></html>"
	return map[string]string{
		"seven_signs/festival/dawn_guide.htm":     guide,
		"seven_signs/festival/dusk_guide.htm":     guide,
		"seven_signs/festival/festival_witch.htm": guide,
		"seven_signs/festival/festival_5.htm":     "<html><body>stats %objectId%<table>%statsTable%</table></body></html>",
		"seven_signs/festival/festival_6.htm":     "<html><body>bonus<table>%bonusTable%</table></body></html>",
		"seven_signs/festival/festival_7.htm":     "<html><body>return from %festivalType%</body></html>",
		"seven_signs/festival/desc_1.htm":         "<html><body>about %festivalType%: %blueStoneNeeded%/%greenStoneNeeded%/%redStoneNeeded% in %cycleMins%</body></html>",
	}
}

// withFestivalRows seeds the festival scores, completing the current
// cycle (1) with a blank score for each festival and cabal not given, and
// the accumulated bonuses.
func withFestivalRows(t *testing.T, bonuses [festival.Count]int, scores ...festival.Score) gameservertest.Option {
	return gameservertest.WithFestivalSeed(func(store *gamesql.FestivalStore) {
		ctx := context.Background()
		rows := append([]festival.Score(nil), scores...)
		for id := range festival.Count {
			for _, cabal := range []sevensigns.Cabal{sevensigns.Dusk, sevensigns.Dawn} {
				given := false
				for _, s := range scores {
					given = given || (s.Cycle == 1 && s.FestivalID == id && s.Cabal == cabal)
				}
				if !given {
					rows = append(rows, festival.Score{FestivalID: id, Cabal: cabal, Cycle: 1})
				}
			}
		}
		if err := store.SaveScores(ctx, rows); err != nil {
			t.Fatalf("seed festival scores: %v", err)
		}
		if err := store.SaveStatus(ctx, festival.Status{Bonuses: bonuses}); err != nil {
			t.Fatalf("seed festival status: %v", err)
		}
	})
}

// festivalAnswer reopens f's guide page, sends command and returns the
// answer.
func (w *folkWorld) festivalAnswer(t *testing.T, f *npc.Folk, command string) [][]byte {
	t.Helper()
	w.talkPage(t, f, false)
	return w.bypass(t, npcCommand(f, command))
}

// festivalDate is a score's date as the high score page writes it:
// year/month/day in the server's time zone, January being month 0.
func festivalDate(millis int64) string {
	d := time.UnixMilli(millis).In(time.Local)
	return strconv.Itoa(d.Year()) + "/" + strconv.Itoa(int(d.Month())-1) + "/" + strconv.Itoa(d.Day())
}

// A festival guide's and a witch's own commands read the festival's
// scores and bonuses of the cycle, here cycle 1 over a cycle-0 record:
//   - Festival 4 builds the festival's high score page, each cabal's best
//     this cycle and the best ever, always said to be the Children of
//     Dusk's, and only the dispatcher releases the client;
//   - Festival 5 lists each festival's standing (Dusk, Dawn, who is ahead)
//     and Festival 6 its accumulated bonus, Festival 7 opens its page and
//     FestivalDesc 1 its description, naming the festival, its signup fee
//     in blue, green and red stones and the 38 minutes to the next
//     festival cycle; each page then releases the client twice;
//   - Chat with any page re-opens the guide page;
//   - Festival 3 from a player in no party opens page 3b, missing here,
//     and a Festival command naming no choice answers nothing at all.
func TestFestivalGuideScorePages(t *testing.T) {
	t.Parallel()
	boot := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	thisCycle := time.Date(2026, time.August, 20, 18, 30, 0, 0, time.UTC).UnixMilli()
	record := time.Date(2025, time.December, 31, 23, 30, 0, 0, time.UTC).UnixMilli()
	w := bootFolkWorld(t, festivalGuidePages(), noBypassReuse,
		gameservertest.WithFestivalClock(func() time.Time { return boot }),
		withFestivalRows(t, [festival.Count]int{100, 0, 2500, 0, 7},
			festival.Score{FestivalID: 0, Cabal: sevensigns.Dawn, Cycle: 1, Date: thisCycle, Score: 7, Members: "Ann,Bob"},
			festival.Score{FestivalID: 0, Cabal: sevensigns.Dusk, Cycle: 1, Date: thisCycle, Score: 5, Members: "Cid"},
			festival.Score{FestivalID: 3, Cabal: sevensigns.Dusk, Cycle: 1, Date: thisCycle, Score: 9, Members: "Dan"},
			festival.Score{FestivalID: 3, Cabal: sevensigns.Dawn, Cycle: 1, Date: thisCycle, Score: 4, Members: "Eve"},
			festival.Score{FestivalID: 0, Cabal: sevensigns.Dawn, Cycle: 0, Date: record, Score: 99, Members: "Old,Timer"},
		),
	)
	guide := w.spawnFolk(t, folkTemplate("FestivalGuide", 31127), 30)
	witch := w.spawnFolk(t, folkTemplate("FestivalGuide", 31146), 35)
	pageAnswer := []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}
	builtAnswer := []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}

	back := func(f *npc.Folk) string {
		return `<a action="bypass -h npc_` + strconv.Itoa(int(f.ObjectID())) + `_Chat 0">Go back.</a></body></html>`
	}
	assertAnswer(t, w.festivalAnswer(t, guide, "Festival 4"), builtAnswer, guide,
		"<html><body>Festival Guide:<br>These are the top scores of the week, for the Level 31 or lower festival.<br>"+
			"Dawn: "+festivalDate(thisCycle)+". Score 7<br>Ann,Bob<br>"+
			"Dusk: "+festivalDate(thisCycle)+". Score 5<br>Cid<br>"+
			"Consecutive top scores: "+festivalDate(record)+". Score 99<br>Affilated side: Children of Dusk<br>Old,Timer<br>"+back(guide))
	assertAnswer(t, w.festivalAnswer(t, witch, "Festival 4"), builtAnswer, witch,
		"<html><body>Festival Guide:<br>These are the top scores of the week, for the No Level Limit festival.<br>"+
			"Dawn: No record exists. Score 0<br>Dusk: No record exists. Score 0<br>Consecutive top scores: No record exists. Score 0<br>"+back(witch))

	row := func(name string, dusk, dawn int, ahead string) string {
		return `<tr><td width="100" align="center">` + name + `</td><td align="center" width="35">` + strconv.Itoa(dusk) +
			`</td><td align="center" width="35">` + strconv.Itoa(dawn) + `</td><td align="center" width="130">` + ahead + `</td></tr>`
	}
	stats := row("Level 31 or lower", 5, 7, "Children of Dawn") + row("Level 42 or lower", 0, 0, "None") + row("Level 53 or lower", 0, 0, "None") +
		row("Level 64 or lower", 9, 4, "Children of Dusk") + row("No Level Limit", 0, 0, "None")
	assertAnswer(t, w.festivalAnswer(t, guide, "Festival 5"), pageAnswer, guide, wantChatPage("<html><body>stats %objectId%<table>"+stats+"</table></body></html>", guide))

	var bonuses strings.Builder
	for i, amount := range []int{100, 0, 2500, 0, 7} {
		bonuses.WriteString(`<tr><td align="center" width="150">` + festival.Names[i] + `</td><td align="center" width="150">` + strconv.Itoa(amount) + `</td></tr>`)
	}
	assertAnswer(t, w.festivalAnswer(t, guide, "Festival 6"), pageAnswer, guide, wantChatPage("<html><body>bonus<table>"+bonuses.String()+"</table></body></html>", guide))
	assertAnswer(t, w.festivalAnswer(t, guide, "Festival 7"), pageAnswer, guide, wantChatPage("<html><body>return from Level 31 or lower</body></html>", guide))
	assertAnswer(t, w.festivalAnswer(t, witch, "FestivalDesc 1"), pageAnswer, witch, wantChatPage("<html><body>about No Level Limit: 6000/3600/1800 in 38</body></html>", witch))

	countdown := `<font color="FF0000">The next festival will begin in 22 minute(s).</font>`
	guidePage := festivalGuidePages()["seven_signs/festival/dawn_guide.htm"]
	assertAnswer(t, w.festivalAnswer(t, guide, "Chat 9"), pageAnswer, guide, wantChatPage(strings.ReplaceAll(guidePage, "%festivalMins%", countdown), guide))

	assertAnswer(t, w.festivalAnswer(t, guide, "Festival 3"), pageAnswer, guide,
		"<html><body>My html is missing:<br>data/html/seven_signs/festival/festival_3b.htm</body></html>")
	if frames := w.festivalAnswer(t, guide, "Festival"); len(frames) != 0 {
		t.Fatalf("malformed Festival answer = %x, want nothing", opcodes(frames))
	}
}

// During seal validation, Festival 0 pays a player on the winning side the
// share of the first score of the cycle whose members' text holds its name
// anywhere (Dusk's festivals first): the festival's bonus divided by the
// members, which leaves the bonus, so a second ask takes a share of what
// is left. The player is handed nothing; the shipped pages answering the
// payout (0a) and the refusal (0b) do not exist, so the missing-page notice
// opens. A player of the losing cabal is refused, the bonus kept. During
// the competition the guide only says bonuses are not paid then. The
// bonuses are written with the next status save.
func TestFestivalGuidePaysBonusShares(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, festivalGuidePages(), noBypassReuse,
		withFestivalRows(t, [festival.Count]int{50, 0, 1000, 0, 0},
			festival.Score{FestivalID: 0, Cabal: sevensigns.Dusk, Cycle: 1, Date: 1, Score: 3, Members: "Ann"},
			festival.Score{FestivalID: 2, Cabal: sevensigns.Dawn, Cycle: 1, Date: 1, Score: 8, Members: "Ann,Talkerson,Bob"},
		),
	)
	guide := w.spawnFolk(t, folkTemplate("FestivalGuide", 31137), 30)
	pageAnswer := []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}
	missing := func(page string) string {
		return "<html><body>My html is missing:<br>data/html/seven_signs/festival/" + page + "</body></html>"
	}
	savedBonus := func(id int) int {
		t.Helper()
		ctx := context.Background()
		if err := w.srv.Festival.SaveStatus(ctx); err != nil {
			t.Fatal(err)
		}
		var amount int
		if err := w.srv.DB.QueryRowContext(ctx, `SELECT accumulated_bonus`+strconv.Itoa(id)+` FROM seven_signs_status WHERE id = 0`).Scan(&amount); err != nil {
			t.Fatal(err)
		}
		return amount
	}

	setSevenSigns(t, w, sevensigns.SealValidation, sevensigns.Dawn, sevensigns.NoCabal, sevensigns.NoCabal, sevensigns.Dawn)
	assertAnswer(t, w.festivalAnswer(t, guide, "Festival 0"), pageAnswer, guide, missing("festival_0a.htm"))
	if got := savedBonus(2); got != 667 {
		t.Fatalf("bonus after the first share = %d, want 1000 - 1000/3 = 667", got)
	}
	assertAnswer(t, w.festivalAnswer(t, guide, "Festival 0"), pageAnswer, guide, missing("festival_0a.htm"))
	if got := savedBonus(2); got != 445 {
		t.Fatalf("bonus after the second share = %d, want 667 - 667/3 = 445", got)
	}
	if got := savedBonus(0); got != 50 {
		t.Fatalf("festival 0 bonus = %d, want it untouched", got)
	}

	setSevenSigns(t, w, sevensigns.SealValidation, sevensigns.Dawn, sevensigns.NoCabal, sevensigns.NoCabal, sevensigns.Dusk)
	assertAnswer(t, w.festivalAnswer(t, guide, "Festival 0"), pageAnswer, guide, missing("festival_0b.htm"))
	if got := savedBonus(2); got != 445 {
		t.Fatalf("bonus after a losing player's ask = %d, want it kept at 445", got)
	}

	setSevenSigns(t, w, sevensigns.Competition, sevensigns.Dawn, sevensigns.NoCabal, sevensigns.NoCabal, sevensigns.Dawn)
	frames := w.festivalAnswer(t, guide, "Festival 0")
	assertAnswer(t, frames, []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed}, guide, "")
	if id := systemMessageID(frames[0]); id != serverpackets.SystemMessageS1 {
		t.Fatalf("competition answer message = %d, want S1", id)
	}
	if text := systemMessageText(t, frames[0]); text != "Bonuses cannot be paid during the competition period." {
		t.Fatalf("competition answer = %q", text)
	}
	if got := savedBonus(2); got != 445 {
		t.Fatalf("bonus after a competition ask = %d, want it kept at 445", got)
	}
}
