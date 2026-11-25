package npcs

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Owner-approved divergences from the reference, each stricter than it:
//   - #5147: a wyvern manager forwards no chamberlain command while its
//     castle's siege runs; the chamberlain's busy page answers instead. Its
//     own Chat stays open.
//   - #5148: a gate upgrade's price class is the one the NPC's gate page
//     links the gate with; a buy packet to a chamberlain or a mercenary
//     manager meets the gates of the dialog that opens the list; the seed
//     and crop setting windows show the NPC's own castle whatever castle
//     the request names.
//   - #4975 (the reference's own behavior): a siege messenger's dialog
//     commands are the generic ones.

// TestWyvernManagerChamberlainCommandsBusyUnderSiege pins #5147: during
// the siege a chamberlain command sent through the wyvern manager opens
// busy.htm alone and moves no adena into the treasury, while the wyvern's
// own Chat still opens its page; outside the siege the deposit goes
// through.
func TestWyvernManagerChamberlainCommandsBusyUnderSiege(t *testing.T) {
	t.Parallel()
	for _, siege := range []bool{true, false} {
		t.Run("siege "+strconv.FormatBool(siege), func(t *testing.T) {
			t.Parallel()
			s := commandSetup{residenceSetup: residenceSetup{siege: siege}}
			s.extra = append(s.extra, gameservertest.WithHTMLPages(chamberlainPages()))
			w := bootResidenceCommands(t, s)
			f := w.spawnKeeper(t, "WyvernManagerNpc", wyvernManagerID, 30)
			cw := &chamberlainWorld{folkWorld: w, f: f}
			w.giveAdena(t, chamberlainAdena)
			frames := cw.run(t, "manage_vault deposit 1000")
			c, _ := w.srv.Castles.Get(taxCastleID)
			if siege {
				assertAnswer(t, frames, pageThenFinal, f, cw.want("busy.htm"))
				if got := c.Treasury(); got != 0 {
					t.Fatalf("treasury after a busy deposit = %d", got)
				}
				if got := w.held(t, item.AdenaID); got != chamberlainAdena {
					t.Fatalf("adena after a busy deposit = %d", got)
				}
			} else if got := c.Treasury(); got != 1000 {
				t.Fatalf("treasury after the deposit = %d (% x)", got, opcodes(frames))
			}
			assertAnswer(t, w.command(t, f, "Chat 3"), chatThenFinal, f, filled("wyvernmanager/wyvernmanager-3.htm", f, wyvernPairs...))
		})
	}
}

// TestSiegeMessengerGenericCommands pins #4975: a siege messenger answers
// the generic dialog commands, here Chat <n> opening its page from
// data/html/default, npcdefault.htm when it has none.
func TestSiegeMessengerGenericCommands(t *testing.T) {
	t.Parallel()
	const messengerID = 35104
	pages := dialogPages()
	pages["default/35104-3.htm"] = "<html><body>messenger three %objectId%</body></html>"
	w := bootFolkWorld(t, pages, noBypassReuse)
	f := w.spawnFolk(t, folkTemplate("SiegeNpc", messengerID), 50)
	for _, tc := range []struct{ command, page string }{
		{"Chat 3", pages["default/35104-3.htm"]},
		{"Chat 4", pages["npcdefault.htm"]},
	} {
		w.openAnyNpcPage(t)
		assertAnswer(t, w.bypass(t, npcCommand(f, tc.command)), chatWindowAnswer, f, wantChatPage(tc.page, f))
	}
}

// TestChamberlainManorSettingWindowsKeepToOwnCastle pins #5148: the seed
// and crop setting windows show the chamberlain's own castle even when the
// request names another.
func TestChamberlainManorSettingWindowsKeepToOwnCastle(t *testing.T) {
	t.Parallel()
	w := bootManorSetup(t, residenceSetup{})
	spawnManorNPC(t, w, "CastleChamberlain", castleChamberlainID, 30)
	for _, ask := range []string{"7", "8"} {
		own := bytes.Join(w.bypass(t, "manor_menu_select?ask="+ask+"&state=-1&time=0"), nil)
		other := bytes.Join(w.bypass(t, "manor_menu_select?ask="+ask+"&state=2&time=0"), nil)
		if len(own) == 0 || !bytes.Equal(own, other) {
			t.Fatalf("ask %s for castle 2 = %x, want Gludio's %x", ask, other, own)
		}
	}
}

// TestChamberlainGateUpgradeClassFromItsPage pins #5148: a gate upgrade
// naming a class other than the one the gate page links the gate with is
// answered nothing, and takes neither adena nor a ratio; the gate's own
// class is priced and paid.
func TestChamberlainGateUpgradeClassFromItsPage(t *testing.T) {
	t.Parallel()
	w := bootChamberlain(t, commandSetup{residenceSetup: residenceSetup{strife: sevensigns.Dawn}, gates: true})
	w.giveAdena(t, chamberlainAdena)
	gate := strconv.Itoa(castleGateB)
	c, _ := w.srv.Castles.Get(taxCastleID)
	ratio := func() int {
		r, ok := c.GateUpgradeRatio(castleGateB)
		if !ok {
			t.Fatal("gate B is no castle gate")
		}
		return r
	}
	before := ratio()
	assertAnswer(t, w.run(t, "doors_choose_upgrade "+gate+" 1 2"), []byte{serverpackets.OpcodeActionFailed}, w.f, "")
	assertAnswer(t, w.run(t, "doors_confirm_upgrade "+gate+" 1 2"), []byte{serverpackets.OpcodeActionFailed}, w.f, "")
	if got := w.held(t, item.AdenaID); got != chamberlainAdena || ratio() != before {
		t.Fatalf("after a cheaper class: adena %d ratio %d, want %d and %d", got, ratio(), chamberlainAdena, before)
	}
	assertAnswer(t, w.run(t, "doors_choose_upgrade "+gate+" 2 2"), pageThenFinal, w.f, w.want("doors-confirm.htm",
		"%objectId%", w.oid(), "%id%", gate, "%level%", "2", "%type%", "2", "%price%", "600000"))
	if _, ok := firstOpcode(w.run(t, "doors_confirm_upgrade "+gate+" 2 2"), serverpackets.OpcodeNpcHtmlMessage); !ok {
		t.Fatal("the gate's own class was not upgraded")
	}
	if got := w.held(t, item.AdenaID); got != chamberlainAdena-600_000 || ratio() != 2 {
		t.Fatalf("after the upgrade: adena %d ratio %d", got, ratio())
	}
}

// TestCastleBuyPacketsMeetTheDialogGates pins #5148: a buy packet sent
// straight to a chamberlain buys only for a talker its products command
// admits, and one to a mercenary manager only for a talker its hire
// command admits; anyone else is answered nothing and keeps its adena.
func TestCastleBuyPacketsMeetTheDialogGates(t *testing.T) {
	t.Parallel()
	products := buylist.NewList(351001, castleChamberlainID, []buylist.Product{{ItemID: certificateID, Price: 10, MaxCount: -1}})
	for _, c := range []struct {
		name  string
		setup residenceSetup
		buys  bool
	}{
		{"lord", residenceSetup{}, true},
		{"member without the privilege", residenceSetup{rank: 5}, false},
		{"lord under siege", residenceSetup{siege: true}, false},
	} {
		t.Run("chamberlain "+c.name, func(t *testing.T) {
			t.Parallel()
			w := bootChamberlain(t, commandSetup{residenceSetup: c.setup, extra: []gameservertest.Option{gameservertest.WithBuyLists(products)}})
			w.giveAdena(t, chamberlainAdena)
			w.send(t, encodeRequestBuyItem(351001, buyRow{certificateID, 1}))
			if got, want := w.held(t, certificateID), map[bool]int{true: 1}[c.buys]; got != want {
				t.Fatalf("certificates bought = %d, want %d", got, want)
			}
		})
	}
	shop := []gameservertest.Option{gameservertest.WithItemTemplates(merchantItems()), gameservertest.WithBuyLists(mercenaryList())}
	for _, c := range []struct {
		name   string
		setup  residenceSetup
		period sevensigns.Period
		buys   bool
	}{
		{"lord", residenceSetup{}, sevensigns.SealValidation, true},
		{"lord outside seal validation", residenceSetup{}, sevensigns.Competition, false},
		{"member", residenceSetup{rank: 5}, sevensigns.SealValidation, false},
	} {
		t.Run("mercenary manager "+c.name, func(t *testing.T) {
			t.Parallel()
			w := bootResidenceCommands(t, commandSetup{residenceSetup: c.setup, period: c.period, extra: shop})
			w.spawnKeeper(t, "MercenaryManagerNpc", castleMercenaryID, 30)
			w.giveAdena(t, chamberlainAdena)
			w.send(t, encodeRequestBuyItem(castleMercenaryID*10+1, buyRow{shopPotionID, 1}))
			if got, want := w.held(t, shopPotionID), map[bool]int{true: 1}[c.buys]; got != want {
				t.Fatalf("potions bought = %d, want %d", got, want)
			}
		})
	}
}
