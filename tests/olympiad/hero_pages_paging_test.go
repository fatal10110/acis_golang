package olympiad

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// heroPageButton is the paging button the reference writes into a hero page.
func heroPageButton(label, command string, page int) string {
	return fmt.Sprintf(`<button value="%s" action="bypass %s?class=88&page=%d" width=60 height=25 back="L2UI_ct1.button_df" fore="L2UI_ct1.button_df">`, label, command, page)
}

// heroPageFooter is the paging table closing both shipped hero pages.
func heroPageFooter(prev, next string) string {
	return `<table width="270" cellspacing="2">` + "\n" +
		"<tr>\n<td width=135>" + prev + "</td>\n<td width=135 align=right>" + next + "</td>\n</tr>\n</table>\n</body></html>\n"
}

// TestHeroPagesPaging pins the Prev and Next buttons of the shipped
// herodiary.htm and herohistory.htm, byte for byte, through the bypass
// path: eleven diary entries and twenty-one fights fill a first page with
// Prev to page 2, a second page holds the rest with Next to page 1, and a
// page past the end shows no entry yet keeps both buttons, as the reference
// does.
func TestHeroPagesPaging(t *testing.T) {
	t.Parallel()
	const diaryEntries, fightCount = 11, 21
	diaryStart := time.Date(2025, time.March, 1, 0, 30, 0, 0, time.Local)
	fightStart := time.Date(2025, time.February, 1, 0, 10, 0, 0, time.Local)
	stmts := []string{
		`INSERT INTO heroes (char_id, class_id, count, played, active, message) SELECT obj_Id, 88, 1, 1, 1, 'Hail' FROM characters WHERE char_name = 'Login'`,
		fmt.Sprintf(`INSERT INTO characters (account_name, obj_Id, char_name, accesslevel) VALUES ('others', %d, 'Alpha', 0)`, alpha),
	}
	diaryAt := make([]time.Time, diaryEntries)
	for i := range diaryAt {
		diaryAt[i] = diaryStart.Add(time.Duration(i) * time.Hour)
		stmts = append(stmts, fmt.Sprintf(`INSERT INTO heroes_diary (char_id, time, action, param) SELECT obj_Id, %d, 2, 0 FROM characters WHERE char_name = 'Login'`, diaryAt[i].UnixMilli()))
	}
	fightAt := make([]time.Time, fightCount)
	for i := range fightAt {
		fightAt[i] = fightStart.Add(time.Duration(i) * time.Hour)
		stmts = append(stmts, fmt.Sprintf(`INSERT INTO olympiad_fights (charOneId, charTwoId, charOneClass, charTwoClass, winner, start, time, classed)
			SELECT obj_Id, %d, 88, 90, 1, %d, 125000, 1 FROM characters WHERE char_name = 'Login'`, alpha, fightAt[i].UnixMilli()))
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

	// diary is the diary page showing the entries at, newest first.
	diary := func(at []time.Time, prev, next string) string {
		var rows strings.Builder
		for i, j := 0, len(at)-1; j >= 0; i, j = i+1, j-1 {
			table := "<table width=270>"
			if i%2 == 0 {
				table = `<table width=270 bgcolor="131210">`
			}
			rows.WriteString("<tr><td>" + table + `<tr><td width=270><font color="LEVEL">` + localHour(at[j]) +
				`:xx</font></td></tr><tr><td width=270>Gained Hero status</td></tr><tr><td>&nbsp;</td></tr></table></td></tr>`)
		}
		return "<html><title>Monument of Heroes:</title><body>\n" +
			`<center><img src="l2ui_ch3.herotower_deco" width=256 height=32></center><br>` + "\n" +
			`<font color="LEVEL">History for :</font> Login<br>` + "\n" +
			"Hero Login's Message:<br>\n" +
			"Hail<br>\n" +
			`<center><img src="L2UI.SquareWhite" width=270 height=1></center><br>` + "\n" +
			`<table width="270" cellspacing="2">` + "\n" +
			rows.String() + "\n" +
			"</table>\n" +
			heroPageFooter(prev, next)
	}
	for _, tc := range []struct {
		command, want string
	}{
		{"_diary?class=88&page=1", diary(diaryAt[1:], heroPageButton("Prev", "_diary", 2), "")},
		{"_diary?class=88&page=2", diary(diaryAt[:1], "", heroPageButton("Next", "_diary", 1))},
		{"_diary?class=88&page=3", diary(nil, heroPageButton("Prev", "_diary", 4), heroPageButton("Next", "_diary", 2))},
	} {
		if got := heroPage(t, c, tc.command); got != tc.want {
			t.Fatalf("%s =\n%q\nwant\n%q", tc.command, got, tc.want)
		}
	}

	// history is the fight history page showing the fights at, oldest first.
	history := func(at []time.Time, prev, next string) string {
		var rows strings.Builder
		for i, start := range at {
			table := `<table width=270><tr><td width=220><font color="LEVEL">`
			if i%2 == 0 {
				table = `<table width=270 bgcolor="131210">`
			}
			rows.WriteString("<tr><td>" + table + localMinute(start) +
				`</font>&nbsp;&nbsp;<font color="00ff00">victory</font></td><td width=50 align=right><font color="FFFF99">cls</font></td></tr><tr><td width=220>vs Alpha (Phoenix Knight)</td><td width=50 align=right>(02:05)</td></tr><tr><td colspan=2>&nbsp;</td></tr></table></td></tr>`)
		}
		return "<html><title>Monument of Heroes:</title><body>\n" +
			`<center><img src="l2ui_ch3.herotower_deco" width=256 height=32></center><br>` + "\n" +
			`<font color="LEVEL">History for :</font> Login<br1>` + "\n" +
			fmt.Sprintf(`<font color="LEVEL">Total score: </font> %d Wins 0 Ties 0 Losses <br>`, fightCount) + "\n" +
			`<center><img src="L2UI.SquareWhite" width=270 height=1></center><br>` + "\n" +
			`<table width="270" cellspacing="2">` + "\n" +
			rows.String() + "\n" +
			"</table>\n" +
			heroPageFooter(prev, next)
	}
	for _, tc := range []struct {
		command, want string
	}{
		{"_match?class=88&page=1", history(fightAt[:20], heroPageButton("Prev", "_match", 2), "")},
		{"_match?class=88&page=2", history(fightAt[20:], "", heroPageButton("Next", "_match", 1))},
	} {
		if got := heroPage(t, c, tc.command); got != tc.want {
			t.Fatalf("%s =\n%q\nwant\n%q", tc.command, got, tc.want)
		}
	}
}
