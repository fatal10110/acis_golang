package bbs

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// regionCastlePage names every placeholder of the castle page, %tax% twice
// as the shipped page shows both of its tax rates with it.
const regionCastlePage = "CASTLE %castleName% tax=%tax%/%tax% lord=%lord% clan=%clanName% ally=%allyName% siege=%siegeDate% halls=%hallsList%"

// moonstoneHallID is Moonstone Hall, a clan hall of the town of Gludio.
const moonstoneHallID = 22

// bootRegion boots the pair over the shipped castles and clan halls with
// the board on: Alice leads Wolves, of the alliance Pack, owning Gludio
// Castle (tax 12, siege at siegeDate) and Moonstone Hall; every other
// castle is free at the default tax of 15.
func bootRegion(t *testing.T, siegeDate time.Time) *pair {
	t.Helper()
	datapack.Require(t)
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	halls, err := gamexml.LoadClanHalls(datapack.Path(t, "data", "xml", "clanHalls.xml"))
	if err != nil {
		t.Fatalf("load clan halls: %v", err)
	}
	decos, err := gamexml.LoadClanHallDeco(datapack.Path(t, "data", "xml", "clanHallDeco.xml"))
	if err != nil {
		t.Fatalf("load clan hall decos: %v", err)
	}
	seed := gameservertest.WithClanSeed(func(db *sql.DB) {
		for _, q := range []struct {
			query string
			args  []any
		}{
			{`UPDATE characters SET clanid = ?, power_grade = 0 WHERE char_name = 'Alice'`, []any{seededClanID}},
			{`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id, ally_id, ally_name, hasCastle)
				SELECT ?, 'Wolves', 5, obj_Id, ?, 'Pack', 1 FROM characters WHERE char_name = 'Alice'`, []any{seededClanID, seededClanID}},
			{`UPDATE castle SET currentTaxPercent = 12, nextTaxPercent = 7, siegeDate = ? WHERE id = 1`, []any{siegeDate.UnixMilli()}},
			{`INSERT INTO clanhall (id, ownerId, paidUntil, paid) VALUES (?, ?, ?, 1)`, []any{moonstoneHallID, seededClanID, time.Now().Add(7 * 24 * time.Hour).UnixMilli()}},
		} {
			if _, err := db.ExecContext(context.Background(), q.query, q.args...); err != nil {
				t.Fatalf("%s: %v", q.query, err)
			}
		}
	})
	return bootPair(t,
		gameservertest.WithCommunityBoard(boardOn),
		gameservertest.WithCastles(castles),
		gameservertest.WithClanHalls(halls, decos),
		gameservertest.WithHTMLPages(map[string]string{"CommunityBoard/region/castle.htm": regionCastlePage}),
		seed,
	)
}

// regionRow is one castle of the castle list.
func regionRow(id int, name, owner, ally, tax string) string {
	return `<table><tr><td width=5></td><td width=160><a action="bypass _bbsloc;` + strconv.Itoa(id) + `">` + name +
		`</a></td><td width=160>` + owner + `</td><td width=160>` + ally + `</td><td width=120>` + tax +
		`</td><td width=5></td></tr></table><br1><img src="L2UI.Squaregray" width=605 height=1><br1>`
}

// Reference: RegionBBSManager.showRegionsList (RegionBBSManager.java:46-59)
// lists every castle by id: its page link, its owner's clan link or None,
// the owner's alliance or None, and the tax rate in force, or 0 for a free
// castle whatever its rate.
func TestRegionBoardCastleList(t *testing.T) {
	p := bootRegion(t, time.Date(2026, 10, 11, 20, 0, 0, 0, time.Local))
	p.enterAll(t)

	wolves := `<a action="bypass _bbsclan;home;` + clanID + `">Wolves</a>`
	var want strings.Builder
	want.WriteString("CASTLES ")
	want.WriteString(regionRow(1, "Gludio Castle", wolves, "Pack", "12"))
	for i, name := range []string{"Dion", "Giran", "Oren", "Aden", "Innadril", "Goddard", "Rune", "Schuttgart"} {
		want.WriteString(regionRow(i+2, name+" Castle", "None", "None", "0"))
	}
	want.WriteString("\n")
	assertPage(t, command(t, p.alice, "_bbsloc"), want.String())
}

// Reference: RegionBBSManager.showRegion (RegionBBSManager.java:61-90)
// fills a castle's page: its name, the tax rate in force (for both rates
// the page shows, whether or not a clan owns it), the owner's leader, clan
// link and alliance or None, and the siege date as yyyy-MM-dd HH:mm. Its
// hall list takes the halls whose town is named as the castle is: no
// shipped hall's town ("Gludio") is a castle's name ("Gludio Castle"), so
// the list stays empty even beside an owned Gludio hall.
func TestRegionBoardCastlePage(t *testing.T) {
	p := bootRegion(t, time.Date(2026, 10, 11, 20, 0, 0, 0, time.Local))
	p.enterAll(t)

	wolves := `<a action="bypass _bbsclan;home;` + clanID + `">Wolves</a>`
	assertPage(t, command(t, p.alice, "_bbsloc;1"),
		"CASTLE Gludio Castle tax=12/12 lord=Alice clan="+wolves+" ally=Pack siege=2026-10-11 20:00 halls=\n")
	assertPage(t, command(t, p.alice, "_bbsloc;2"),
		"CASTLE Dion Castle tax=15/15 lord=None clan=None ally=None siege="+time.UnixMilli(0).Format("2006-01-02 15:04")+" halls=\n")
}

// A region command naming no castle, a castle id that does not read or a
// castle that does not exist shows nothing, as the reference's parse and
// lookup failures send nothing; the region form still answers as an
// unknown one.
func TestRegionBoardBadCommandsSilent(t *testing.T) {
	p := bootRegion(t, time.Now())
	p.enterAll(t)

	for _, cmd := range []string{"_bbsloc;", "_bbslocation", "_bbsloc;x", "_bbsloc;99", "_bbsloc;0"} {
		if frames := command(t, p.alice, cmd); len(frames) != 0 {
			t.Fatalf("%s answer = %x, want silence", cmd, opcodes(frames))
		}
	}
	assertPage(t, write(t, p.alice, "_bbsloc", "first", "second"),
		"<html><body><br><br><center>The command: first isn't implemented.</center></body></html>")
}
