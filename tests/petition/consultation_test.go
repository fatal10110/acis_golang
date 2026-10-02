package petition

import (
	"context"
	"reflect"
	"strings"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/petition"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestPetitionConsultation drives one petition through its life: a game
// master joins it (Petition.join), both sides chat on the petition
// channels (ChatPetition, Petition.sendMessage), the petitioner cannot end
// it, the chat is replayed in the EnterWorld burst after the login
// board pages and before the reuse timers (EnterWorld.java:254), the game
// master closes it (RequestPetitionCancel.java:37-38) and the petitioner
// rates it (PetitionVote.java), which the petition window then shows.
func TestPetitionConsultation(t *testing.T) {
	t.Parallel()
	// The server news is on so the relog burst carries a login board page
	// the petition chat has to follow.
	r := boot(t, gameservertest.WithServerNews(true), gameservertest.WithHTMLPages(map[string]string{"servnews.htm": serverNews}))
	r.enterAll(t)
	id := r.submit(t)

	// Pending, the petition has no chat yet.
	assertMessages(t, exchange(t, r.player, encodeSay2(sayPetitionPlayer, "anyone?")), msg(serverpackets.SystemMessageYouAreNotInPetitionChat))

	frames := exchange(t, r.gm, encodeBypass("admin_petition join "+itoa(id)))
	if len(frames) != 2 {
		t.Fatalf("join frames = %x, want the consultation notice then the list", testsupport.FrameOpcodes(frames))
	}
	assertMessages(t, frames[:1], msg(serverpackets.SystemMessagePetitionWithS1UnderWay, "Player"))
	// The petition the game master answers is listed without a link, and
	// the unfollow button shows.
	assertPage(t, frames[1], "<td width=260>#"+itoa(id)+" by Player<br1>", `action="bypass -h admin_petition unfollow"`, "State:</font> ACCEPTED")
	assertMessages(t, collect(t, r.player), msg(serverpackets.SystemMessagePetitionAppAccepted))

	playerLine := say{r.playerID, sayPetitionPlayer, "Player", "my sword is gone"}
	assertSays(t, exchange(t, r.player, encodeSay2(sayPetitionPlayer, playerLine.text)), playerLine)
	assertSays(t, collect(t, r.gm), playerLine)
	// A game master's petition line moves to the game master's channel.
	gmLine := say{r.gmID, sayPetitionGM, "Admin", "looking into it"}
	assertSays(t, exchange(t, r.gm, encodeSay2(sayPetitionPlayer, gmLine.text)), gmLine)
	assertSays(t, collect(t, r.player), gmLine)

	assertMessages(t, exchange(t, r.player, encodePetitionCancel()), msg(serverpackets.SystemMessagePetitionUnderProcess))

	r.restart(t, r.player, r.playerID)
	burst := enterWorld(t, r.player)
	// EnterWorld.java:225-256: the login board page (here the server news)
	// comes first, then the petition chat, then the reuse timers.
	shortcuts, news, cooltimes := index(burst, serverpackets.OpcodeShortCutInit), index(burst, serverpackets.OpcodeNpcHtmlMessage), index(burst, serverpackets.OpcodeSkillCoolTime)
	if shortcuts < 0 || news != shortcuts+1 || cooltimes < news {
		t.Fatalf("burst = %x, want ShortCutInit, the server news, then SkillCoolTime", testsupport.FrameOpcodes(burst))
	}
	if page := htmlBody(t, burst[news]); page != serverNews+"\n" {
		t.Fatalf("server news = %q, want %q", page, serverNews)
	}
	assertSays(t, burst[news+1:cooltimes], playerLine, gmLine)
	drain(t, r.gm)

	assertMessages(t, exchange(t, r.gm, encodePetitionCancel()), msg(serverpackets.SystemMessagePetitionEndedWithS1, "Player"))
	frames = collect(t, r.player)
	testsupport.AssertOpcodeSequence(t, frames, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePetitionVote)
	assertMessages(t, frames[:1], msg(serverpackets.SystemMessageThisEndThePetitionPleaseProvideFeedback))
	if frames[1][0] != 0xf6 || len(frames[1]) != 1 {
		t.Fatalf("PetitionVote = %x, want f6 alone", frames[1])
	}

	if frames := exchange(t, r.player, encodeVote(0, "  great help \t")); len(frames) != 0 {
		t.Fatalf("vote frames = %x, want none", testsupport.FrameOpcodes(frames))
	}
	// Only the first vote counts.
	exchange(t, r.player, encodeVote(4, "changed my mind"))

	frames = exchange(t, r.gm, encodeBypass("admin_petition view "+itoa(id)))
	if len(frames) != 1 {
		t.Fatalf("view frames = %x, want the petition page", testsupport.FrameOpcodes(frames))
	}
	assertPage(t, frames[0], "<title>Petition #"+itoa(id)+"</title>", "Petitioner: Player (online)", "Type: BUG_REPORT",
		"State: CLOSED", "Responders: Admin </td>", "help<br>", "Rate: Very Good", "Feedback: great help</td>")
	if page := htmlBody(t, frames[0]); strings.Contains(page, "admin_petition join") {
		t.Fatalf("closed petition page offers join: %q", page)
	}

	// Stored at shutdown, then read back as the next boot reads it.
	r.srv.SavePetitions(t)
	records, err := gamesql.NewPetitionStore(r.srv.DB).Load(context.Background())
	if err != nil {
		t.Fatalf("load petitions: %v", err)
	}
	if want := r.srv.Petitions.Records(); !reflect.DeepEqual(records, want) {
		t.Fatalf("stored petitions = %+v, want %+v", records, want)
	}
	if len(records) != 1 {
		t.Fatalf("stored petitions = %+v, want one", records)
	}
	got := records[0]
	if got.State != petition.Closed || got.Rate != petition.VeryGood || got.Feedback != "great help" || got.Unread ||
		!reflect.DeepEqual(got.Responders, []int32{r.gmID}) || len(got.Messages) != 2 {
		t.Fatalf("stored petition = %+v", got)
	}
	var stored []string
	rows, err := r.srv.DB.QueryContext(context.Background(), "SELECT id, type, player_name, content FROM petition_message WHERE petition_oid = ? ORDER BY id", id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			n                  int
			typ, name, content string
		)
		if err := rows.Scan(&n, &typ, &name, &content); err != nil {
			t.Fatal(err)
		}
		stored = append(stored, itoa(int32(n))+" "+typ+" "+name+" "+content)
	}
	if want := []string{"0 PETITION_PLAYER Player my sword is gone", "1 PETITION_GM Admin looking into it"}; !reflect.DeepEqual(stored, want) {
		t.Fatalf("petition_message rows = %q, want %q", stored, want)
	}
}

// TestPetitionAcceptedWithoutChatStoresPending pins PetitionManager.store:
// a petition accepted with nothing said in it yet is stored pending, with
// no responder.
func TestPetitionAcceptedWithoutChatStoresPending(t *testing.T) {
	t.Parallel()
	r := boot(t)
	r.enterAll(t)
	id := r.submit(t)
	exchange(t, r.gm, encodeBypass("admin_petition join "+itoa(id)))
	r.srv.SavePetitions(t)
	var state, responders string
	if err := r.srv.DB.QueryRowContext(context.Background(), "SELECT state, responders FROM petition WHERE oid = ?", id).Scan(&state, &responders); err != nil {
		t.Fatal(err)
	}
	if state != "PENDING" || responders != "" {
		t.Fatalf("stored state, responders = %q, %q; want PENDING with none", state, responders)
	}
}

// index returns the position of the first frame carrying opcode, -1 when
// none does.
// serverNews is the server news page shown at login.
const serverNews = "<html><body>SERVER NEWS</body></html>"

func index(frames [][]byte, opcode byte) int {
	for i, frame := range frames {
		if frame[0] == opcode {
			return i
		}
	}
	return -1
}
