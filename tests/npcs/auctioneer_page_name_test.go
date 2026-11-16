package npcs

import (
	"bytes"
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestAuctionSaleInfoNamesItsPageFile: with //show html on, a game master
// whose clan sells its hall, opening the sale info page, is told that
// page's file, AgitSaleInfo.htm, and not the hall info page's.
func TestAuctionSaleInfoNamesItsPageFile(t *testing.T) {
	t.Parallel()
	admin, err := gamexml.LoadAdminData(datapack.Path(t, "data", "xml"))
	if err != nil {
		t.Fatalf("load admin data: %v", err)
	}
	setup := auctionSetup{
		clanLevel: 2, adena: 1_000_000,
		halls: []hallRow{{
			id: moonstone, owner: auctionClanID, paid: true, paidUntil: time.Now().Add(72 * time.Hour).UnixMilli(),
			sellerBid: 30_000_000, sellerName: "Talker", sellerCln: "Bidders", endDate: time.Now().Add(48 * time.Hour).UnixMilli(),
		}},
	}
	w := bootAuction(t, setup, gameservertest.WithAdmin(admin), gameservertest.WithClanSeed(func(db *sql.DB) {
		seedAuction(t, db, setup)
		if _, err := db.ExecContext(context.Background(), "UPDATE characters SET accesslevel = 7 WHERE char_name = 'Talker'"); err != nil {
			t.Fatalf("set access level: %v", err)
		}
	}))
	cmd := wire.NewPacketWriter(clientpackets.OpcodeSendBypassBuildCmd)
	cmd.WriteString("show html")
	w.c.Send(cmd.Bytes())
	drainFrames(t, w.c)
	w.open(t)

	notice := wire.NewPacketWriter(serverpackets.OpcodeCreatureSay)
	notice.WriteInt32(0)
	notice.WriteInt32(0)
	notice.WriteString("HTML")
	notice.WriteString("auction/AgitSaleInfo.htm")
	frames := w.bypass(t, "selectedItems")
	page := -1
	for i, f := range frames {
		if f[0] == serverpackets.OpcodeNpcHtmlMessage {
			page = i
		}
	}
	if page < 0 || page+1 >= len(frames) || !bytes.Equal(frames[page+1], notice.Bytes()) {
		t.Fatalf("sale info = %x, want the page, then the notice naming auction/AgitSaleInfo.htm", opcodes(frames))
	}
}
