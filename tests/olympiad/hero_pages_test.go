package olympiad

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// heroPageOptions boots the shipped hero diary and fight history pages.
func heroPageOptions(t *testing.T) gameservertest.Option {
	t.Helper()
	pages := map[string]string{}
	for _, name := range []string{"olympiad/herodiary.htm", "olympiad/herohistory.htm"} {
		content, err := os.ReadFile(datapack.Path(t, "data", "html", name))
		if err != nil {
			t.Fatal(err)
		}
		pages[name] = string(content)
	}
	return gameservertest.WithHTMLPages(pages)
}

// seedStatements runs stmts as the Olympiad seed.
func seedStatements(t *testing.T, stmts []string) func(*sql.DB) {
	return func(db *sql.DB) {
		for _, stmt := range stmts {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("seed %q: %v", stmt, err)
			}
		}
	}
}

func encodeHeroBypass(command string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBypassToServer)
	w.WriteString(command)
	return w.Bytes()
}

func encodeWriteHeroWords(message string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestWriteHeroWords)
	w.WriteString(message)
	return w.Bytes()
}

// heroPage sends command and returns the html of the one NpcHtmlMessage it
// answers with, from no object; "" when nothing answers.
func heroPage(t *testing.T, c *testsupport.ScriptedClient, command string) string {
	t.Helper()
	c.Send(encodeHeroBypass(command))
	frames := drainFrames(t, c)
	if len(frames) == 0 {
		return ""
	}
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeNpcHtmlMessage {
		t.Fatalf("%s answered %d frames, first %#x; want one NpcHtmlMessage", command, len(frames), frames[0][0])
	}
	r := wire.NewReader(frames[0][1:])
	if id := r.ReadInt32(); id != 0 {
		t.Fatalf("%s page object id = %d, want 0", command, id)
	}
	return r.ReadString()
}

// localHour and localMinute write an instant the way the pages date it,
// in the server's zone.
func localHour(at time.Time) string {
	at = at.Local()
	return fmt.Sprintf("%04d-%02d-%02d %02d", at.Year(), at.Month(), at.Day(), at.Hour())
}

func localMinute(at time.Time) string {
	return localHour(at) + fmt.Sprintf(":%02d", at.Local().Minute())
}

// TestHeroPages pins _diary and _match against the shipped pages, byte for
// byte, for a hero of the running era seeded with two diary entries and
// three fights, one of them this month: the diary newest first under the
// hero's message, the fights before this month oldest first with the
// score. A hero's words replace its message on the page and are stored at
// shutdown. A class without a hero, or a command that does not parse,
// answers nothing.
func TestHeroPages(t *testing.T) {
	t.Parallel()
	gained := time.Date(2025, time.March, 1, 9, 15, 0, 0, time.Local)
	later := time.Date(2025, time.March, 2, 22, 40, 0, 0, time.Local)
	won := time.Date(2025, time.February, 20, 18, 4, 0, 0, time.Local)
	lost := time.Date(2025, time.February, 21, 7, 30, 0, 0, time.Local)
	stmts := []string{
		`INSERT INTO heroes (char_id, class_id, count, played, active, message) SELECT obj_Id, 88, 1, 1, 1, 'Hail' FROM characters WHERE char_name = 'Login'`,
		fmt.Sprintf(`INSERT INTO characters (account_name, obj_Id, char_name, accesslevel) VALUES ('others', %d, 'Alpha', 0)`, alpha),
		fmt.Sprintf(`INSERT INTO heroes_diary (char_id, time, action, param) SELECT obj_Id, %d, 2, 0 FROM characters WHERE char_name = 'Login'`, gained.UnixMilli()),
		fmt.Sprintf(`INSERT INTO heroes_diary (char_id, time, action, param) SELECT obj_Id, %d, 2, 0 FROM characters WHERE char_name = 'Login'`, later.UnixMilli()),
		fmt.Sprintf(`INSERT INTO olympiad_fights (charOneId, charTwoId, charOneClass, charTwoClass, winner, start, time, classed)
			SELECT obj_Id, %d, 88, 90, 1, %d, 125000, 1 FROM characters WHERE char_name = 'Login'`, alpha, won.UnixMilli()),
		fmt.Sprintf(`INSERT INTO olympiad_fights (charOneId, charTwoId, charOneClass, charTwoClass, winner, start, time, classed)
			SELECT obj_Id, %d, 88, 90, 2, %d, 61000, 0 FROM characters WHERE char_name = 'Login'`, alpha, lost.UnixMilli()),
		fmt.Sprintf(`INSERT INTO olympiad_fights (charOneId, charTwoId, charOneClass, charTwoClass, winner, start, time, classed)
			SELECT obj_Id, %d, 88, 90, 0, %d, 1000, 0 FROM characters WHERE char_name = 'Login'`, alpha, time.Now().UnixMilli()),
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Login", 40, 0),
		gameservertest.WithWantChars(1),
		heroPageOptions(t),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithOlympiadSeed(seedStatements(t, stmts)),
	)
	c := srv.Client
	startInWorld(t, c)

	diary := func(message string) string {
		return "<html><title>Monument of Heroes:</title><body>\n" +
			`<center><img src="l2ui_ch3.herotower_deco" width=256 height=32></center><br>` + "\n" +
			`<font color="LEVEL">History for :</font> Login<br>` + "\n" +
			"Hero Login's Message:<br>\n" +
			message + "<br>\n" +
			`<center><img src="L2UI.SquareWhite" width=270 height=1></center><br>` + "\n" +
			`<table width="270" cellspacing="2">` + "\n" +
			`<tr><td><table width=270 bgcolor="131210"><tr><td width=270><font color="LEVEL">` + localHour(later) + `:xx</font></td></tr><tr><td width=270>Gained Hero status</td></tr><tr><td>&nbsp;</td></tr></table></td></tr>` +
			`<tr><td><table width=270><tr><td width=270><font color="LEVEL">` + localHour(gained) + `:xx</font></td></tr><tr><td width=270>Gained Hero status</td></tr><tr><td>&nbsp;</td></tr></table></td></tr>` + "\n" +
			"</table>\n" +
			`<table width="270" cellspacing="2">` + "\n" +
			"<tr>\n<td width=135></td>\n<td width=135 align=right></td>\n</tr>\n</table>\n</body></html>\n"
	}
	if got, want := heroPage(t, c, "_diary?class=88&page=1"), diary("Hail"); got != want {
		t.Fatalf("diary page =\n%q\nwant\n%q", got, want)
	}

	history := "<html><title>Monument of Heroes:</title><body>\n" +
		`<center><img src="l2ui_ch3.herotower_deco" width=256 height=32></center><br>` + "\n" +
		`<font color="LEVEL">History for :</font> Login<br1>` + "\n" +
		`<font color="LEVEL">Total score: </font> 1 Wins 0 Ties 1 Losses <br>` + "\n" +
		`<center><img src="L2UI.SquareWhite" width=270 height=1></center><br>` + "\n" +
		`<table width="270" cellspacing="2">` + "\n" +
		`<tr><td><table width=270 bgcolor="131210">` + localMinute(won) + `</font>&nbsp;&nbsp;<font color="00ff00">victory</font></td><td width=50 align=right><font color="FFFF99">cls</font></td></tr><tr><td width=220>vs Alpha (Phoenix Knight)</td><td width=50 align=right>(02:05)</td></tr><tr><td colspan=2>&nbsp;</td></tr></table></td></tr>` +
		`<tr><td><table width=270><tr><td width=220><font color="LEVEL">` + localMinute(lost) + `</font>&nbsp;&nbsp;<font color="ff0000">loss</font></td><td width=50 align=right><font color="999999">non-cls<font></td></tr><tr><td width=220>vs Alpha (Phoenix Knight)</td><td width=50 align=right>(01:01)</td></tr><tr><td colspan=2>&nbsp;</td></tr></table></td></tr>` + "\n" +
		"</table>\n" +
		`<table width="270" cellspacing="2">` + "\n" +
		"<tr>\n<td width=135></td>\n<td width=135 align=right></td>\n</tr>\n</table>\n</body></html>\n"
	if got := heroPage(t, c, "_match?class=88&page=1"); got != history {
		t.Fatalf("fight history page =\n%q\nwant\n%q", got, history)
	}

	for _, command := range []string{"_diary?class=89&page=1", "_match?class=88", "_diary?class=x&page=1", "_diary?class=88&page=0"} {
		if got := heroPage(t, c, command); got != "" {
			t.Errorf("%s answered a page: %q", command, got)
		}
	}

	c.Send(encodeWriteHeroWords("For $glory"))
	if got, want := heroPage(t, c, "_diary?class=88&page=1"), diary("For $glory"); got != want {
		t.Fatalf("diary page after the hero's words =\n%q\nwant\n%q", got, want)
	}
	srv.Heroes.Shutdown()
	srv.FlushPersistence(t)
	var stored string
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT message FROM heroes WHERE char_id = ?", srv.SoleObjectID(t)).Scan(&stored); err != nil || stored != "For $glory" {
		t.Fatalf("stored message = %q, %v; want the hero's words", stored, err)
	}
}

// TestHeroWordsNeedAHero pins that only a hero writes hero words: an
// inactive hero's are dropped, and its page keeps its stored message.
func TestHeroWordsNeedAHero(t *testing.T) {
	t.Parallel()
	stmts := []string{
		`INSERT INTO heroes (char_id, class_id, count, played, active, message) SELECT obj_Id, 88, 1, 1, 0, 'Kept' FROM characters WHERE char_name = 'Login'`,
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Login", 40, 0),
		gameservertest.WithWantChars(1),
		heroPageOptions(t),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithOlympiadSeed(seedStatements(t, stmts)),
	)
	c := srv.Client
	startInWorld(t, c)
	c.Send(encodeWriteHeroWords("Not a hero yet"))
	drainFrames(t, c)
	srv.Heroes.Shutdown()
	srv.FlushPersistence(t)
	var stored string
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT message FROM heroes WHERE char_id = ?", srv.SoleObjectID(t)).Scan(&stored); err != nil || stored != "Kept" {
		t.Fatalf("stored message = %q, %v; want Kept", stored, err)
	}
}
