package npcs

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Merchant.showBuyWindow and RequestBuyItem.runImpl
// (Merchant.java:152-160, RequestBuyItem.java:79-80, 118-119, 172-175) tax
// each unit price at the merchant's castle rate, whether or not a clan owns
// it, and raise the castle's tax revenue by (int) (subTotal * rate);
// PreparedListContainer and PreparedEntry (PreparedListContainer.java:
// 34-45, PreparedEntry.java:20-45) add Math.round(count * rate) per tax
// ingredient to a taxing list's adena only under an owned castle, and
// MultiSellChoose.runImpl (MultiSellChoose.java:337-339) pays the castle
// the tax times the amount; Npc.onBypassFeedback "TerritoryStatus"
// (Npc.java:1135-1158) shows territorystatus.htm or territorynoclan.htm.
// Npc.getCastle is the castle whose castles.xml npcs list holds the NPC's
// id (NpcTemplate.java:148-156). aCis revision in the outer repo.

const (
	taxCastleID  = 1 // Gludio: its town name is NpcString 1001001
	taxClanID    = 0x10000077
	taxPercent   = 10
	taxClanName  = "Lords"
	taxTownName  = "Gludio"
	taxKingdom   = "The Kingdom of Aden"
	statusPage   = `<html><body>%townName% of %territory%: %clanLeaderName% (%clanName%) %taxPercent%% <a action="bypass -h npc_%objectId%_Chat 0">Back</a></body></html>`
	noClanPage   = `<html><body>%townName% of %territory%: no clan <a action="bypass -h npc_%objectId%_Chat 0">Back</a></body></html>`
	taxCastleTax = 15 // the default rate; the tests put taxPercent in force
)

// taxCastles is Gludio with merchantID among its NPCs, no system cut and
// no parent, so the revenue a sale raises is the tax itself.
func taxCastles(t *testing.T) *castledata.Table {
	t.Helper()
	gludio, err := castledata.NewCastle(castledata.CastleAttrs{
		ID: taxCastleID, Alias: "gludio_castle", Name: "Gludio Castle", CircletID: 6838,
		Tax: residence.Tax{Rate: taxCastleTax}, NPCs: []int{merchantID},
	}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	table, err := castledata.NewTable([]*castledata.Castle{gludio})
	if err != nil {
		t.Fatal(err)
	}
	return table
}

// withTaxCastle loads taxCastles and, when owned, a clan led by the
// character leader holding Gludio.
func withTaxCastle(t *testing.T, leader string, owned bool) []gameservertest.Option {
	t.Helper()
	opts := []gameservertest.Option{gameservertest.WithCastles(taxCastles(t))}
	if owned {
		opts = append(opts, gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, s := range []struct {
				q    string
				args []any
			}{
				{"UPDATE characters SET clanid = ? WHERE char_name = ?", []any{taxClanID, leader}},
				{`INSERT INTO clan_data (clan_id, clan_name, clan_level, hasCastle, leader_id)
					SELECT ?, ?, 5, ?, obj_Id FROM characters WHERE char_name = ?`, []any{taxClanID, taxClanName, taxCastleID, leader}},
			} {
				if _, err := db.ExecContext(context.Background(), s.q, s.args...); err != nil {
					t.Fatalf("seed castle clan: %v", err)
				}
			}
		}))
	}
	return opts
}

// taxCastle puts taxPercent in force at Gludio and returns it.
func taxCastle(t *testing.T, srv *gameservertest.Server, owned bool) *castle.Castle {
	t.Helper()
	c, ok := srv.Castles.Get(taxCastleID)
	if !ok {
		t.Fatal("Gludio not loaded")
	}
	if c.IsFree() == owned {
		t.Fatalf("Gludio free = %v, want %v", c.IsFree(), !owned)
	}
	c.SetCurrentTaxPercent(taxPercent, false)
	return c
}

// TestMerchantCastleTax pins the buy window and the purchase at a castle's
// merchant: each unit price is taxed at the castle rate, owned or not
// (50 * 1.1 = 55; the siege guard ticket 100 * 1 * 1.1 = 110), and an owned
// castle's revenue grows by (int) (165 * 0.1) = 16 for three potions; a
// free castle collects nothing.
func TestMerchantCastleTax(t *testing.T) {
	t.Parallel()
	for _, owned := range []bool{true, false} {
		t.Run(map[bool]string{true: "owned", false: "free"}[owned], func(t *testing.T) {
			t.Parallel()
			w := bootShop(t, 10000, withTaxCastle(t, "Buyer", owned)...)
			c := taxCastle(t, w.srv, owned)

			_, _, _, rows := decodeBuyList(t, w.openBuy(t, "1")[0])
			if rows[0].itemID != shopPotionID || rows[0].price != 55 {
				t.Fatalf("potion row = %+v, want price 55", rows[0])
			}
			if rows[6].itemID != ticketID || rows[6].price != 110 {
				t.Fatalf("ticket row = %+v, want price 110", rows[6])
			}

			w.send(t, encodeRequestBuyItem(shopListID, buyRow{shopPotionID, 3}))
			if adena, potions := w.countOf(t, item.AdenaID), w.countOf(t, shopPotionID); adena != 10000-165 || potions != 3 {
				t.Fatalf("after 3 potions: adena %d potions %d, want %d/3", adena, potions, 10000-165)
			}
			want := int64(0)
			if owned {
				want = 16
			}
			if got := c.TaxRevenue(); got != want {
				t.Fatalf("tax revenue = %d, want %d", got, want)
			}
		})
	}
}

// TestMultisellCastleTax pins a taxing list at a castle's NPC: under an
// owned castle, list 9001's first entry asks 100 adena plus
// Math.round(1000 * 0.1) = 100 of tax, which the exchange pays to the
// castle; under a free castle the tax ingredient is dropped and nothing is
// paid.
func TestMultisellCastleTax(t *testing.T) {
	t.Parallel()
	for _, owned := range []bool{true, false} {
		t.Run(map[bool]string{true: "owned", false: "free"}[owned], func(t *testing.T) {
			t.Parallel()
			w := bootMultisell(t, msTalker(), [][2]int32{{item.AdenaID, 10000}, {msOreID, 10}}, nil, withTaxCastle(t, "Trader", owned)...)
			c := taxCastle(t, w.srv, owned)

			pages := w.mustOpen(t, "9001")
			wantAdena, wantTax := int32(100), int64(0)
			if owned {
				wantAdena, wantTax = 200, 100
			}
			ins := pages[0].entries[0].ins
			if len(ins) != 2 || ins[0].id != msOreID || ins[0].count != 2 || ins[1].id != item.AdenaID || ins[1].count != wantAdena {
				t.Fatalf("entry 1 ingredients = %+v, want 2 ore then %d adena", ins, wantAdena)
			}

			frames := w.choose(t, "9001", 1, 1)
			if _, ok := firstOpcode(frames, serverpackets.OpcodeSystemMessage); !ok {
				t.Fatalf("exchange = %x, want its messages", opcodes(frames))
			}
			if got := w.held(t, item.AdenaID); got != 10000-int(wantAdena) {
				t.Fatalf("adena = %d, want %d", got, 10000-int(wantAdena))
			}
			if got := c.TaxRevenue(); got != wantTax {
				t.Fatalf("tax revenue = %d, want %d", got, wantTax)
			}
		})
	}
}

// TestTerritoryStatus pins TerritoryStatus at a castle's NPC: the owned
// castle's page names the town, the kingdom, the lord, its clan and the
// tax in force; a free castle's page only the town and kingdom.
func TestTerritoryStatus(t *testing.T) {
	t.Parallel()
	for _, owned := range []bool{true, false} {
		t.Run(map[bool]string{true: "owned", false: "free"}[owned], func(t *testing.T) {
			t.Parallel()
			pages := dialogPages()
			pages["territorystatus.htm"] = statusPage
			pages["territorynoclan.htm"] = noClanPage
			w := bootFolkWorld(t, pages, append(withTaxCastle(t, "Talker", owned), noBypassReuse)...)
			taxCastle(t, w.srv, owned)
			f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)
			w.talkTo(t, f)
			w.openAnyNpcPage(t)

			frames := w.bypass(t, npcCommand(f, "TerritoryStatus"))
			html, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
			if !ok {
				t.Fatalf("TerritoryStatus = %x, want NpcHtmlMessage", opcodes(frames))
			}
			page := noClanPage
			if owned {
				page = strings.NewReplacer("%clanLeaderName%", "Talker", "%clanName%", taxClanName, "%taxPercent%", "10").Replace(statusPage)
			}
			want := strings.NewReplacer("%townName%", taxTownName, "%territory%", taxKingdom).Replace(page)
			if objectID, got, _ := htmlMessage(t, html); objectID != f.ObjectID() || got != wantChatPage(want, f) {
				t.Fatalf("TerritoryStatus page = %d %q, want %q", objectID, got, wantChatPage(want, f))
			}
		})
	}
}
