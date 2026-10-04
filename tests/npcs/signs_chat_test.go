package npcs

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// signsPages answers every Seven Signs, festival and Olympiad page these
// tests open with a page naming itself and its NPC.
func signsPages() map[string]string {
	pages := map[string]string{}
	add := func(path string) {
		pages[path] = "<html><body>" + path + " %objectId%</body></html>"
	}
	for _, cabal := range []string{"dawn", "dusk"} {
		for _, p := range []string{"1", "2a", "2b", "2c", "2d", "3", "4", "5", "6"} {
			add("seven_signs/" + cabal + "_priest_" + p + ".htm")
		}
	}
	for _, p := range []string{"blkmrkt_1.htm", "mammmerch_1.htm", "mammblack_1.htm"} {
		add("seven_signs/" + p)
	}
	pages["seven_signs/festival/dawn_guide.htm"] = `<html><body>dawn guide %objectId%<br>%festivalMins%<a action="bypass -h npc_%objectId%_Festival 1">Join</a></body></html>`
	pages["seven_signs/festival/dusk_guide.htm"] = "<html><body>dusk guide<br>%festivalMins%</body></html>"
	pages["seven_signs/festival/festival_witch.htm"] = "<html><body>witch<br>%festivalMins%</body></html>"
	add("olympiad/noble.htm")
	add("olympiad/noble_main.htm")
	add("olympiad/hero_main2.htm")
	return pages
}

// wantSignsPage is the page signsPages holds at path, as f shows it.
func wantSignsPage(path string, f *npc.Folk) string {
	return wantChatPage("<html><body>"+path+" %objectId%</body></html>", f)
}

// setSevenSigns rewrites the status row and the player's sign-up, then
// has the running Seven Signs state read them back.
func setSevenSigns(t *testing.T, w *folkWorld, period sevensigns.Period, winner, gnosis, avarice, player sevensigns.Cabal) {
	t.Helper()
	ctx := context.Background()
	var dawnStones, duskStones int
	switch winner {
	case sevensigns.Dawn:
		dawnStones = 10
	case sevensigns.Dusk:
		duskStones = 10
	}
	if _, err := w.srv.DB.ExecContext(ctx, `UPDATE seven_signs_status SET active_period = ?, dawn_stone_score = ?, dusk_stone_score = ?,
		avarice_owner = ?, gnosis_owner = ?, date = 0 WHERE id = 0`, period.String(), dawnStones, duskStones, avarice.String(), gnosis.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := w.srv.DB.ExecContext(ctx, `DELETE FROM seven_signs`); err != nil {
		t.Fatal(err)
	}
	if player != sevensigns.NoCabal {
		if _, err := w.srv.DB.ExecContext(ctx, `INSERT INTO seven_signs (char_obj_id, cabal, seal) VALUES (?, ?, 'AVARICE')`, w.player, player.String()); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.srv.SevenSigns.Restore(ctx); err != nil {
		t.Fatal(err)
	}
}

// chatAnswer is what a talk answers once the player stands at the NPC:
// the frames after MoveToPawn and the talk animation.
func chatAnswer(frames [][]byte) [][]byte {
	for i, f := range frames {
		if f[0] == serverpackets.OpcodeMoveToPawn {
			rest := frames[i+1:]
			if len(rest) > 0 && rest[0][0] == serverpackets.OpcodeSocialAction {
				rest = rest[1:]
			}
			return rest
		}
	}
	return nil
}

// talkPage talks to f and returns the page it opens, failing unless the
// answer is the page then ActionFailed, or ActionFailed then the page when
// released first.
func (w *folkWorld) talkPage(t *testing.T, f *npc.Folk, releasedFirst bool) string {
	t.Helper()
	w.selectFolk(t, f)
	answer := chatAnswer(w.talk(t, f, false))
	want := []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}
	html := 0
	if releasedFirst {
		want, html = []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeNpcHtmlMessage}, 1
	}
	if got := opcodes(answer); string(got) != string(want) {
		t.Fatalf("%d answer = %x, want %x", f.NpcID(), got, want)
	}
	_, page, _ := htmlMessage(t, answer[html])
	return page
}

// A Priest of Dawn and a Dusk Priestess release the client, then greet
// with a page chosen by the talker's cabal, the period and, in seal
// validation, the competition's outcome: the rival cabal is turned away
// (3), recruiting (6) and results (5) have their own pages, the
// competition the general one (1); in seal validation a member of the
// winning cabal gets 2a, or 2c when the cabal lacks the Seal of Gnosis, a
// talker of no cabal 4, and everyone else 2b, or 2d when nobody won.
func TestSevenSignsPriestPages(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, signsPages())
	dawn := w.spawnFolk(t, folkTemplate("DawnPriest", 31078), 30)
	dusk := w.spawnFolk(t, folkTemplate("DuskPriest", 31085), 40)
	const (
		none = sevensigns.NoCabal
		Dawn = sevensigns.Dawn
		Dusk = sevensigns.Dusk
	)
	for _, tc := range []struct {
		period                   sevensigns.Period
		winner, gnosis, player   sevensigns.Cabal
		wantDawnPage, wantDuskPg string
	}{
		{sevensigns.Competition, none, none, none, "1", "1"},
		{sevensigns.Competition, Dawn, none, Dawn, "1", "3"},
		{sevensigns.Competition, Dawn, none, Dusk, "3", "1"},
		{sevensigns.Recruiting, none, none, none, "6", "6"},
		{sevensigns.Recruiting, none, none, Dusk, "3", "6"},
		{sevensigns.Results, Dawn, Dawn, Dawn, "5", "3"},
		{sevensigns.SealValidation, Dawn, Dusk, none, "4", "2b"},
		{sevensigns.SealValidation, Dawn, Dusk, Dawn, "2c", "3"},
		{sevensigns.SealValidation, Dawn, Dawn, Dawn, "2a", "3"},
		{sevensigns.SealValidation, Dusk, Dusk, Dusk, "3", "2a"},
		{sevensigns.SealValidation, Dusk, none, Dusk, "3", "2c"},
		{sevensigns.SealValidation, Dusk, Dusk, none, "2b", "4"},
		{sevensigns.SealValidation, none, none, none, "2d", "2d"},
		{sevensigns.SealValidation, none, none, Dawn, "2d", "3"},
	} {
		name := fmt.Sprintf("%v winner %v gnosis %v player %v", tc.period, tc.winner, tc.gnosis, tc.player)
		setSevenSigns(t, w, tc.period, tc.winner, tc.gnosis, none, tc.player)
		for _, c := range []struct {
			f      *npc.Folk
			prefix string
			page   string
		}{{dawn, "dawn", tc.wantDawnPage}, {dusk, "dusk", tc.wantDuskPg}} {
			want := wantSignsPage("seven_signs/"+c.prefix+"_priest_"+c.page+".htm", c.f)
			if got := w.talkPage(t, c.f, true); got != want {
				t.Fatalf("%s: %s priest page = %q, want %q", name, c.prefix, got, want)
			}
		}
	}
}

// The Black Marketeer of Mammon always opens its page. The Merchant and
// the Blacksmith of Mammon serve only members of the winning cabal owning
// the Seal of Avarice, respectively of Gnosis: anyone else hears whom they
// serve, then the client is released. With no winner the merchant says it
// serves during the quest event period only and releases nothing, while
// the blacksmith serves everybody.
func TestMammonPagesAndRefusals(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, signsPages())
	market := w.spawnFolk(t, folkTemplate("SignsPriest", 31092), 30)
	merchant := w.spawnFolk(t, folkTemplate("SignsPriest", 31113), 35)
	smith := w.spawnFolk(t, folkTemplate("SignsPriest", 31126), 40)
	const (
		none = sevensigns.NoCabal
		Dawn = sevensigns.Dawn
		Dusk = sevensigns.Dusk
	)
	type answer struct {
		page    string // the page opened, or ""
		message int    // the refusal's system message, or 0
		release bool   // a refusal also sends ActionFailed
	}
	page := func(name string) answer { return answer{page: name} }
	competitionOnly := answer{message: serverpackets.SystemMessageQuestEventPeriod}
	dawnOnly := answer{message: serverpackets.SystemMessageCanBeUsedByDawn, release: true}
	duskOnly := answer{message: serverpackets.SystemMessageCanBeUsedByDusk, release: true}
	for _, tc := range []struct {
		winner, avarice, gnosis, player sevensigns.Cabal
		merchant, smith                 answer
	}{
		{Dawn, Dawn, Dawn, Dawn, page("mammmerch_1.htm"), page("mammblack_1.htm")},
		{Dawn, Dawn, Dusk, Dawn, page("mammmerch_1.htm"), dawnOnly},
		{Dawn, Dawn, Dawn, Dusk, dawnOnly, dawnOnly},
		{Dawn, Dawn, Dawn, none, dawnOnly, dawnOnly},
		{Dusk, Dusk, Dusk, Dusk, page("mammmerch_1.htm"), page("mammblack_1.htm")},
		{Dusk, Dawn, Dusk, Dusk, duskOnly, page("mammblack_1.htm")},
		{none, none, none, none, competitionOnly, page("mammblack_1.htm")},
		{none, Dawn, Dawn, Dawn, competitionOnly, page("mammblack_1.htm")},
	} {
		name := fmt.Sprintf("winner %v avarice %v gnosis %v player %v", tc.winner, tc.avarice, tc.gnosis, tc.player)
		setSevenSigns(t, w, sevensigns.SealValidation, tc.winner, tc.gnosis, tc.avarice, tc.player)
		if got, want := w.talkPage(t, market, false), wantSignsPage("seven_signs/blkmrkt_1.htm", market); got != want {
			t.Fatalf("%s: black marketeer page = %q, want %q", name, got, want)
		}
		for _, c := range []struct {
			f    *npc.Folk
			want answer
		}{{merchant, tc.merchant}, {smith, tc.smith}} {
			if c.want.page != "" {
				if got, want := w.talkPage(t, c.f, false), wantSignsPage("seven_signs/"+c.want.page, c.f); got != want {
					t.Fatalf("%s: %d page = %q, want %q", name, c.f.NpcID(), got, want)
				}
				continue
			}
			w.selectFolk(t, c.f)
			got := chatAnswer(w.talk(t, c.f, false))
			want := []byte{serverpackets.OpcodeSystemMessage}
			if c.want.release {
				want = append(want, serverpackets.OpcodeActionFailed)
			}
			if string(opcodes(got)) != string(want) {
				t.Fatalf("%s: %d refusal = %x, want %x", name, c.f.NpcID(), opcodes(got), want)
			}
			if id := int(binary.LittleEndian.Uint32(got[0][1:])); id != c.want.message {
				t.Fatalf("%s: %d refusal message = %d, want %d", name, c.f.NpcID(), id, c.want.message)
			}
		}
	}
}

// A festival guide opens its oracle's guide page, and a festival witch the
// witches' page, each counting down to the next festival: the schedule
// started at boot awaits it 2 + 19 minutes later, read on a clock that
// stands still so the count is exactly 21 minutes, plus one. During seal
// validation the page says festivals resume next week. A guide's dialog
// commands are not in place yet: they log and release the client.
func TestFestivalGuidePages(t *testing.T) {
	t.Parallel()
	boot := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	w := bootFolkWorld(t, signsPages(), gameservertest.WithFestivalClock(func() time.Time { return boot }))
	dawnGuide := w.spawnFolk(t, folkTemplate("FestivalGuide", 31127), 30)
	duskGuide := w.spawnFolk(t, folkTemplate("FestivalGuide", 31141), 35)
	witch := w.spawnFolk(t, folkTemplate("FestivalGuide", 31146), 40)
	countdown := `<font color="FF0000">The next festival will begin in 22 minute(s).</font>`
	for _, c := range []struct {
		f    *npc.Folk
		page string
	}{
		{dawnGuide, "<html><body>dawn guide %objectId%<br>" + countdown + `<a action="bypass -h npc_%objectId%_Festival 1">Join</a></body></html>`},
		{duskGuide, "<html><body>dusk guide<br>" + countdown + "</body></html>"},
		{witch, "<html><body>witch<br>" + countdown + "</body></html>"},
	} {
		if got, want := w.talkPage(t, c.f, false), wantChatPage(c.page, c.f); got != want {
			t.Fatalf("%d page = %q, want %q", c.f.NpcID(), got, want)
		}
		if c.f != dawnGuide {
			continue
		}
		w.c.Send(encodeBypass(fmt.Sprintf("npc_%d_Festival 1", dawnGuide.ObjectID())))
		if got := opcodes(drainFrames(t, w.c)); string(got) != string([]byte{serverpackets.OpcodeActionFailed}) {
			t.Fatalf("Festival 1 answer = %x, want ActionFailed only", got)
		}
	}

	setSevenSigns(t, w, sevensigns.SealValidation, sevensigns.NoCabal, sevensigns.NoCabal, sevensigns.NoCabal, sevensigns.NoCabal)
	want := wantChatPage(`<html><body>witch<br><font color="FF0000">This is the Seal Validation period. Festivals will resume next week.</font></body></html>`, witch)
	if got := w.talkPage(t, witch, false); got != want {
		t.Fatalf("witch page in seal validation = %q, want %q", got, want)
	}
}

// The Grand Olympiad Manager greets a noble with noble_main.htm and anyone
// else with noble.htm. A Monument of Heroes greets anyone who is no hero
// with hero_main2.htm, noble or not (see TestMonumentOfHeroesPages).
func TestOlympiadManagerPages(t *testing.T) {
	t.Parallel()
	for _, noble := range []bool{false, true} {
		t.Run(fmt.Sprintf("noble=%v", noble), func(t *testing.T) {
			t.Parallel()
			var extra []gameservertest.Option
			if noble {
				extra = append(extra, gameservertest.WithOlympiadSeed(func(db *sql.DB) {
					if _, err := db.Exec(`UPDATE characters SET nobless = 1 WHERE char_name = 'Talker'`); err != nil {
						t.Fatal(err)
					}
				}))
			}
			w := bootFolkWorld(t, signsPages(), extra...)
			manager := w.spawnFolk(t, folkTemplate("OlympiadManagerNpc", 31688), 30)
			page := "olympiad/noble.htm"
			if noble {
				page = "olympiad/noble_main.htm"
			}
			if got, want := w.talkPage(t, manager, false), wantSignsPage(page, manager); got != want {
				t.Fatalf("manager page = %q, want %q", got, want)
			}

			monument := w.spawnFolk(t, folkTemplate("OlympiadManagerNpc", 31690), 40)
			if got, want := w.talkPage(t, monument, false), wantSignsPage("olympiad/hero_main2.htm", monument); got != want {
				t.Fatalf("monument page = %q, want %q", got, want)
			}
		})
	}
}

// The priests' own commands are not in place yet: a Chat or SevenSigns
// command logs and releases the client.
func TestPriestCommandsUnported(t *testing.T) {
	t.Parallel()
	pages := signsPages()
	pages["seven_signs/dawn_priest_1.htm"] = `<html><body>` +
		`<a action="bypass -h npc_%objectId%_Chat 0">Back</a>` +
		`<a action="bypass -h npc_%objectId%_SevenSigns 3 2">Join</a>` +
		`<a action="bypass -h npc_%objectId%_SevenSignsDesc 1">About</a></body></html>`
	w := bootFolkWorld(t, pages, noBypassReuse)
	priest := w.spawnFolk(t, folkTemplate("DawnPriest", 31078), 30)
	w.talkPage(t, priest, true)
	for _, command := range []string{"Chat 0", "SevenSigns 3 2", "SevenSignsDesc 1"} {
		w.c.Send(encodeBypass(fmt.Sprintf("npc_%d_%s", priest.ObjectID(), command)))
		if got := opcodes(drainFrames(t, w.c)); string(got) != string([]byte{serverpackets.OpcodeActionFailed}) {
			t.Fatalf("%s answer = %x, want ActionFailed only", command, got)
		}
	}
}
