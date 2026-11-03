package admin

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/castlemanor"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// manorAdminSeeds is Gludio's manor: Red Coda (seed 5012, reference price
// 10; crop 5068) and Dark Coda (seed 5016, reference price 5; crop 5073).
func manorAdminSeeds() *manor.Table {
	table := manor.NewTable([]manor.Manor{{ID: 1, Name: "gludio", Seeds: []manor.Seed{
		{CropID: 5068, SeedID: 5012, MatureID: 5098, Level: 12, Reward1: 1865, Reward2: 1879, CastleID: 1, SeedsLimit: 5000, CropsLimit: 5000},
		{CropID: 5073, SeedID: 5016, MatureID: 5103, Level: 10, Reward1: 1864, Reward2: 1878, CastleID: 1, SeedsLimit: 5000, CropsLimit: 5000},
	}}})
	prices := map[int32]int32{5012: 10, 5068: 30, 5016: 5, 5073: 51}
	table.ApplyReferencePrices(func(id int32) (int32, bool) {
		p, ok := prices[id]
		return p, ok
	})
	return table
}

// manorAdminRows are Gludio's stored lists. The running period: 1000 Dark
// Coda seeds (1000 x reference price 5) and 100 Dark Coda crops at 50,
// 10,000 in all. The next one: 2000 Red Coda seeds (2000 x 10) and 50 Red
// Coda crops at 30, 21,500 in all. The amounts left and the seed sale price
// do not count.
var manorAdminRows = []string{
	"INSERT INTO castle_manor_production VALUES (1, 5016, 7, 1000, 99, 0), (1, 5012, 3, 2000, 99, 1)",
	"INSERT INTO castle_manor_procure VALUES (1, 5073, 40, 100, 50, 1, 0), (1, 5068, 1, 50, 30, 2, 1)",
}

// TestAdminManor pins //manor (AdminManor.java): the manor page shows the
// mode's name, the next mode change as dd/MM HH:mm:ss and, for each castle
// in castles.xml order, the running and the next period's cost with
// thousands grouped. The clicked link opens the same page.
func TestAdminManor(t *testing.T) {
	t.Parallel()
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	cfg := castlemanor.DefaultConfig()
	cfg.SavePeriod = 0
	// A modifiable morning: the lists are approved at 06:00.
	start := time.Date(2026, 10, 5, 5, 58, 0, 0, time.UTC)
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithHTMLPages(shippedAdminPages(t, "manor.htm")),
		gameservertest.WithCastles(castles),
		gameservertest.WithManor(network.ManorConfig{Allowed: true, CropRate: 1, Seeds: manorAdminSeeds()}),
		gameservertest.WithCastleManor(cfg, start),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, q := range manorAdminRows {
				if _, err := db.ExecContext(context.Background(), q); err != nil {
					t.Fatalf("seed manor: %v", err)
				}
			}
		}),
	)
	gm := srv.Client
	enterWorld(t, gm)

	// The page as the HTML cache serves it, line breaks stripped.
	raw, ok := gameservertest.HTMLCache(t, shippedAdminPages(t, "manor.htm")).Get("data/html/admin/manor.htm")
	if !ok {
		t.Fatal("no admin/manor.htm in the HTML cache")
	}
	var info strings.Builder
	costs := map[int][2]string{1: {"10,000", "21,500"}}
	for _, c := range castles.All() {
		cost, ok := costs[c.ID]
		if !ok {
			cost = [2]string{"0", "0"}
		}
		info.WriteString("<tr><td width=110>Name:</td><td width=160><font color=008000>" + c.Name + "</font></td></tr>")
		info.WriteString("<tr><td>Current period cost:</td><td><font color=FF9900>" + cost[0] + " Adena</font></td></tr>")
		info.WriteString("<tr><td>Next period cost:</td><td><font color=FF9900>" + cost[1] + " Adena</font></td></tr>")
		info.WriteString("<tr><td>&nbsp;</td></tr>")
	}
	want := strings.NewReplacer(
		"%status%", "MODIFIABLE",
		"%change%", "05/10 06:00:00",
		"%castleInfo%", info.String(),
	).Replace(raw)
	if !strings.Contains(want, "Schuttgart Castle") {
		t.Fatalf("expected page misses the last castle:\n%s", want)
	}

	for _, payload := range [][]byte{encodeBuildCmd("manor"), encodeBypass("admin_manor")} {
		frames := exchange(t, gm, payload)
		if len(frames) != 1 {
			t.Fatalf("frames = %x, want one NpcHtmlMessage", testsupport.FrameOpcodes(frames))
		}
		if got := htmlBody(t, frames[0]); got != want {
			t.Fatalf("manor page =\n%s\nwant\n%s", got, want)
		}
	}
}

// TestAdminManorDisabled pins //manor with the manor off: the page has no
// mode change or lists to show, so the command is answered with
// ActionFailed alone.
func TestAdminManorDisabled(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithHTMLPages(shippedAdminPages(t, "manor.htm")))
	gm := srv.Client
	enterWorld(t, gm)
	frames := exchange(t, gm, encodeBuildCmd("manor"))
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("frames = %x, want ActionFailed", testsupport.FrameOpcodes(frames))
	}
}
