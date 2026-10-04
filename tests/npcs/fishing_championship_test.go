package npcs

import (
	"database/sql"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/fishchamp"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// fishermanID is Fishing Guild Member Klufe, whose shipped first page links
// the championship.
const fishermanID = 31562

// releaseAnswer is a page answered with the dispatcher's release only.
var releaseAnswer = []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}

// shippedPage reads a datapack page as the page cache holds it.
func shippedPage(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(datapack.Path(t, strings.Split(rel, "/")...))
	if err != nil {
		t.Fatal(err)
	}
	page := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasSuffix(page, "\n") {
		page += "\n"
	}
	return page
}

// fishermanPages are the dialog fixture pages plus the fisherman's first
// page and the championship pages, as shipped.
func fishermanPages(t *testing.T) map[string]string {
	t.Helper()
	pages := dialogPages()
	for _, rel := range []string{
		"fisherman/" + strconv.Itoa(fishermanID) + ".htm",
		"fisherman/championship/fish_event001.htm",
		"fisherman/championship/fish_event_reward001.htm",
		"fisherman/championship/no_fish_event001.htm",
		"fisherman/championship/no_fish_event_reward001.htm",
	} {
		pages[rel] = shippedPage(t, "data/html/"+rel)
	}
	return pages
}

// seedChampionship stores a week ending in a day and the given
// fishing_championship rows: name, length, rewarded.
func seedChampionship(t *testing.T, end time.Time, rows ...[]any) func(*sql.DB) {
	return func(db *sql.DB) {
		if _, err := db.Exec("INSERT INTO server_memo (var, value) VALUES ('fishChampionshipEnd', ?)", strconv.FormatInt(end.UnixMilli(), 10)); err != nil {
			t.Errorf("seed server_memo: %v", err)
		}
		for _, r := range rows {
			if _, err := db.Exec("INSERT INTO fishing_championship (player_name, fish_length, rewarded) VALUES (?, ?, ?)", r...); err != nil {
				t.Errorf("seed fishing_championship: %v", err)
			}
		}
	}
}

// bootFisherman boots the player Talker next to the fisherman, with the
// championship run under cfg on rows.
func bootFisherman(t *testing.T, cfg fishchamp.Config, rows ...[]any) (*folkWorld, *npc.Folk) {
	t.Helper()
	w := bootFolkWorld(t, fishermanPages(t), noBypassReuse,
		gameservertest.WithFishingChampionship(cfg, seedChampionship(t, time.Now().Add(24*time.Hour), rows...)))
	return w, w.spawnFolk(t, folkTemplate("Fisherman", fishermanID), 60)
}

// championshipRow is one place of a ranking table as the pages show it.
func championshipRow(place int, name, length string) string {
	return "<tr><td width=70 align=center>" + strconv.Itoa(place) + "</td><td width=110 align=center>" + name + "</td><td width=80 align=center>" + length + "</td></tr>"
}

// winnersPage is the shipped winners page for fisherman f with the given
// table and minutes left.
func winnersPage(t *testing.T, f *npc.Folk, table string, minutes int64) string {
	t.Helper()
	page := shippedPage(t, fishchamp.PageWinners)
	for _, kv := range [][2]string{
		{"%TABLE%", table},
		{"%prizeItem%", "Adena"},
		{"%prizeFirst%", "800000"},
		{"%prizeTwo%", "500000"},
		{"%prizeThree%", "300000"},
		{"%prizeFour%", "200000"},
		{"%prizeFive%", "100000"},
		{"%refresh%", strconv.FormatInt(minutes, 10)},
		{"%objectId%", strconv.Itoa(int(f.ObjectID()))},
	} {
		page = strings.ReplaceAll(page, kv[0], kv[1])
	}
	return page
}

// storedChampionship reads the stored fishers: name, length and reward.
func storedChampionship(t *testing.T, w *folkWorld) []string {
	t.Helper()
	w.srv.FlushPersistence(t)
	rows, err := w.srv.DB.Query("SELECT player_name, fish_length, rewarded FROM fishing_championship")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name, length string
		var rewarded int
		if err := rows.Scan(&name, &length, &rewarded); err != nil {
			t.Fatal(err)
		}
		out = append(out, name+" "+length+" "+strconv.Itoa(rewarded))
	}
	return out
}

// TestFishermanChampionshipWinnersAndPrize walks Fisherman.onBypassFeedback's
// championship commands with last week's winners stored: FishingChampionship
// opens fish_event001 with the five places longest first, the prizes in
// adena, the minutes left in the running week and the claim link;
// FishingReward pays the talker the second place's 500,000 adena, named as
// picked up, then thanks it on fish_event_reward001 under object id 0, and
// the claim is stored. A second claim pays nothing and opens no page.
func TestFishermanChampionshipWinnersAndPrize(t *testing.T) {
	t.Parallel()
	w, f := bootFisherman(t, fishchamp.DefaultConfig(),
		[]any{"Talker", 81.5, 1}, []any{"Other", 88.25, 1}, []any{"Runner", 70, 0})
	w.talkTo(t, f)

	frames := w.dialogFrames(t, npcCommand(f, "FishingChampionship"))
	table := championshipRow(1, "Other", "88.25") + championshipRow(2, "Talker", "81.5") +
		championshipRow(3, "None", "0") + championshipRow(4, "None", "0") + championshipRow(5, "None", "0")
	assertAnswer(t, frames, releaseAnswer, f, winnersPage(t, f, table, w.srv.FishingChampionship.MinutesLeft()))

	before := w.srv.PlayerInventory(t, w.player).Adena()
	frames = w.dialogFrames(t, npcCommand(f, "FishingReward"))
	assertFrames(t, "claim", frames[:1], sysMsg(serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(item.AdenaID), itemNumberParam(500000)))
	if got := opcodes(frames[1:]); string(got) != string(releaseAnswer) {
		t.Fatalf("claim answer = %x, want %x", got, releaseAnswer)
	}
	if objectID, html, _ := htmlMessage(t, frames[1]); objectID != 0 || html != shippedPage(t, fishchamp.PageRewarded) {
		t.Fatalf("claim page = object %d %q, want object 0 fish_event_reward001", objectID, html)
	}
	if got := w.srv.PlayerInventory(t, w.player).Adena(); got != before+500000 {
		t.Fatalf("adena after the claim = %d, want %d", got, before+500000)
	}
	if got, want := storedChampionship(t, w), []string{"Other 88.25 1", "Talker 81.5 2", "Runner 70 0"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("stored championship = %q, want %q", got, want)
	}

	w.talkTo(t, f)
	w.dialogFrames(t, npcCommand(f, "FishingChampionship"))
	if got := opcodes(w.dialogFrames(t, npcCommand(f, "FishingReward"))); string(got) != string(releaseOnly) {
		t.Fatalf("second claim = %x, want the dispatcher's release only", got)
	}
	if got := w.srv.PlayerInventory(t, w.player).Adena(); got != before+500000 {
		t.Fatalf("adena after the second claim = %d, want %d", got, before+500000)
	}
}

// TestFishermanChampionshipRefusals pins the refusals: a claim from a
// player not among last week's winners, its name matched exactly, opens
// no_fish_event_reward001; with the championship disabled
// FishingChampionship opens no_fish_event001, which links nothing further.
func TestFishermanChampionshipRefusals(t *testing.T) {
	t.Parallel()
	t.Run("not a winner", func(t *testing.T) {
		t.Parallel()
		w, f := bootFisherman(t, fishchamp.DefaultConfig(), []any{"talker", 81.5, 1}, []any{"Talker", 75, 0})
		w.talkTo(t, f)
		w.dialogFrames(t, npcCommand(f, "FishingChampionship"))
		assertAnswer(t, w.dialogFrames(t, npcCommand(f, "FishingReward")), releaseAnswer, f, shippedPage(t, fishchamp.PageNotWinner))
	})
	t.Run("disabled", func(t *testing.T) {
		t.Parallel()
		w, f := bootFisherman(t, fishchamp.Config{}, []any{"Talker", 81.5, 1})
		w.talkTo(t, f)
		assertAnswer(t, w.dialogFrames(t, npcCommand(f, "FishingChampionship")), releaseAnswer, f, shippedPage(t, fishchamp.PageDisabled))
	})
}
