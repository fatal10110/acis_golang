package npcs

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: CastleChamberlain.onBypassFeedback (CastleChamberlain.java:
// 55-660) and its helpers (662-856), WyvernManagerNpc.onBypassFeedback
// (WyvernManagerNpc.java:31-98, 118-125):
//   - every command first meets the talk condition: no clan, another clan
//     or no castle opens noprivs.htm, a clan member while the siege runs
//     busy.htm, each as read; a wyvern manager answers its castle owner's
//     members and never as under siege;
//   - validatePrivileges opens noprivs.htm as read and stops;
//     sendFileMessage fills %objectId%, %npcId% and %time%;
//   - manage_vault moves adena between the talker and the treasury below
//     the int32 maximum, vault-no.htm for a withdrawal the treasury does not
//     cover; tax_set opens tax-leader.htm (limit 25/5/15 by the Seal of
//     Strife) or, after noprivs.htm, tax-member.htm; tax_adjust sets the
//     next rate within the limit, tax-toohigh.htm beyond it;
//   - give_crown (leader only) adds item 6841 once; receive_report (leader
//     only) names the owner, its leader, the castle, the period and seals;
//   - manor_certificate / validate_certificate sell 10 certificates (6388)
//     for 10 x 1000 adena during seal validation to a non-Dusk talker while
//     the castle has some left;
//   - doors/traps upgrades price 300000.. x3 under Dusk, x0.8 under Dawn,
//     refuse an upgrade already reached, and store the new ratio or level;
//   - operate_door works the castle's gates; siege_change and
//     siege_time_set read and move the siege date; any other command falls
//     to the merchant's.

const (
	chamberlainAdena = 5_000_000
	lordCrownID      = 6841
	certificateID    = 6388
)

// chamberlainPage names file and every placeholder the chamberlain dialog
// fills.
func chamberlainPage(file string) string {
	return "<html><body>" + file + " %objectId% %npcId% %time% %tax_income% %tax_income_reserved% %seed_income% " +
		"%withdraw_amount% %currentTaxRate% %nextTaxRate% %taxLimit% %clanname% %clanleadername% %castlename% " +
		"%ss_event% %ss_avarice% %ss_gnosis% %ss_strife% %CharName% %FeudName% %left% %bundle% %price% %id% " +
		"%type% %level% %trapIndex%</body></html>"
}

var chamberlainFiles = []string{
	"banishafter.htm", "banishfore.htm", "manage.htm", "products.htm", "report.htm", "vault.htm", "vault-no.htm",
	"35100-d.htm", "doors-open.htm", "doors-close.htm", "tax-member.htm", "tax-leader.htm", "tax-toohigh.htm",
	"tax-after.htm", "manor/manor.htm", "manor/manor_help002.htm", "no.htm", "siegetime1.htm", "siegetime2.htm",
	"siegetime3.htm", "siegetime6.htm", "siegetime8.htm", "gavecrown.htm", "hascrown.htm",
	"not-dawn-or-event.htm", "not-enough-ticket.htm", "sell-dawn-ticket.htm", "not-enough-adena.htm",
	"devices.htm", "35100-gu.htm", "doors-update.htm", "doors-confirm.htm", "doors-already-updated.htm",
	"doors-success.htm", "35100-tu.htm", "traps-update.htm", "traps-confirm.htm", "traps-already-updated.htm",
	"traps-success.htm", "noprivs.htm", "busy.htm",
}

func chamberlainPages() map[string]string {
	pages := commandPages()
	for _, file := range chamberlainFiles {
		pages["chamberlain/"+file] = chamberlainPage(file)
	}
	pages["npcdefault.htm"] = chamberlainPage("npcdefault.htm")
	pages["merchant/35100-1.htm"] = chamberlainPage("merchant-1.htm")
	return pages
}

// chamberlainWorld is a castle world with the chamberlain spawned and
// talked to.
type chamberlainWorld struct {
	*folkWorld
	f *npc.Folk
}

// chamberlainCatalog is the shared catalog plus the Lord's Crown and the
// Certificate of Dawn.
func chamberlainCatalog() gameservertest.Option {
	templates := append([]*item.Template(nil), gameservertest.ItemTemplates().All()...)
	templates = append(templates,
		&item.Template{
			ID: lordCrownID, Name: "Lord's Crown", Kind: item.KindEtcItem, Duration: -1,
			Dropable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		},
		&item.Template{
			ID: certificateID, Name: "Certificate of Dawn", Kind: item.KindEtcItem, Duration: -1, Stackable: true,
			Dropable: true, Tradable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{},
		})
	return gameservertest.WithItemTemplates(item.NewTable(templates))
}

func bootChamberlain(t *testing.T, s commandSetup) *chamberlainWorld {
	t.Helper()
	s.extra = append(s.extra, gameservertest.WithHTMLPages(chamberlainPages()), chamberlainCatalog())
	w := bootResidenceCommands(t, s)
	return &chamberlainWorld{folkWorld: w, f: w.spawnKeeper(t, "CastleChamberlain", castleChamberlainID, 30)}
}

// run sends command to the chamberlain and keeps the frames a dialog
// answers with: inventory and status refreshes are left out.
func (w *chamberlainWorld) run(t *testing.T, command string) [][]byte {
	t.Helper()
	var out [][]byte
	for _, f := range w.command(t, w.f, command) {
		if f[0] != serverpackets.OpcodeInventoryUpdate && f[0] != serverpackets.OpcodeStatusUpdate {
			out = append(out, f)
		}
	}
	return out
}

// want is file's chamberlain page with pairs filled, as sent.
func (w *chamberlainWorld) want(file string, pairs ...string) string {
	return fill(chamberlainPage(file), pairs...) + "\n"
}

// filledFile is file as sendFileMessage fills it, plus pairs.
func (w *chamberlainWorld) filledFile(t *testing.T, file string, pairs ...string) string {
	t.Helper()
	c, ok := w.srv.Castles.Get(taxCastleID)
	if !ok {
		t.Fatal("no Gludio")
	}
	date := time.UnixMilli(c.SiegeDate()).Format("Mon Jan 02 15:04:05 MST 2006")
	return w.want(file, append([]string{"%objectId%", w.oid(), "%npcId%", "35100", "%time%", date}, pairs...)...)
}

func (w *chamberlainWorld) oid() string { return strconv.Itoa(int(w.f.ObjectID())) }

// giveAdena hands the talker n adena.
func (w *folkWorld) giveAdena(t *testing.T, n int) {
	t.Helper()
	id := w.srv.NewObjectID()
	w.onRiderQueue(t, func(c *player.Character) {
		if c.Inventory().AddNew(item.AdenaID, n, id) == nil {
			t.Error("adena not added")
		}
	})
}

// castleRow reads Gludio's stored treasury, next tax rate, certificates,
// siege date and date lock.
func (w *folkWorld) castleRow(t *testing.T) (treasury int64, nextTax, certificates int, siegeDate int64, regOver string) {
	t.Helper()
	w.srv.FlushPersistence(t)
	err := w.srv.DB.QueryRowContext(context.Background(),
		`SELECT treasury, nextTaxPercent, certificates, siegeDate, regTimeOver FROM castle WHERE id = ?`, taxCastleID).
		Scan(&treasury, &nextTax, &certificates, &siegeDate, &regOver)
	if err != nil {
		t.Fatalf("read castle row: %v", err)
	}
	return
}

var pageOnly = []byte{serverpackets.OpcodeNpcHtmlMessage}

// TestChamberlainTalkConditionAndPrivileges pins the gate every command
// meets: a stranger gets noprivs.htm and a talker during the siege busy.htm,
// each as read; a member lacking a command's privilege gets noprivs.htm
// as read; one holding it gets the command's page filled by
// sendFileMessage. Each is followed by the dispatcher's ActionFailed.
func TestChamberlainTalkConditionAndPrivileges(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		setup residenceSetup
		file  string
	}{
		{"stranger", residenceSetup{rival: true}, "noprivs.htm"},
		{"clanless", residenceSetup{clanless: true}, "noprivs.htm"},
		{"under siege", residenceSetup{siege: true}, "busy.htm"},
		{"member without the privilege", residenceSetup{rank: 5, privs: privCastleSiege}, "noprivs.htm"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := bootChamberlain(t, commandSetup{residenceSetup: c.setup})
			assertAnswer(t, w.run(t, "products"), pageThenFinal, w.f, w.want(c.file))
		})
	}
	t.Run("member with the privilege", func(t *testing.T) {
		t.Parallel()
		w := bootChamberlain(t, commandSetup{residenceSetup: residenceSetup{rank: 5, privs: privCastleFunctions}})
		assertAnswer(t, w.run(t, "PRODUCTS"), pageThenFinal, w.f, w.filledFile(t, "products.htm"))
		// A member is no lord: the report and the crown are refused.
		assertAnswer(t, w.run(t, "receive_report"), pageThenFinal, w.f, w.filledFile(t, "noprivs.htm"))
		assertAnswer(t, w.run(t, "give_crown"), pageThenFinal, w.f, w.want("noprivs.htm"))
		// give_crown matches exactly.
		assertAnswer(t, w.run(t, "GIVE_CROWN"), []byte{serverpackets.OpcodeActionFailed}, w.f, "")
	})
}

// Clan privileges, as clan_privs stores them.
const (
	privCastleEntryExit = 32768
	privCastleManor     = 65536
	privCastleSiege     = 131072
	privCastleFunctions = 262144
	privCastleTaxes     = 1048576
	privCastleSetFuncs  = 4194304
)

// TestChamberlainVaultAndTaxes walks the lord through the vault and the tax
// pages: a deposit moves adena into the treasury, a withdrawal back, one
// the treasury does not cover opens vault-no.htm; tax_set shows the limit
// the Seal of Strife allows, tax_adjust stores a rate within it and refuses
// one beyond it. A member without the tax privilege sees the rates after
// the refusal page.
func TestChamberlainVaultAndTaxes(t *testing.T) {
	t.Parallel()
	t.Run("lord", func(t *testing.T) {
		t.Parallel()
		w := bootChamberlain(t, commandSetup{residenceSetup: residenceSetup{strife: sevensigns.Dawn}})
		w.giveAdena(t, chamberlainAdena)
		vault := func(treasury, amount string) []string {
			return []string{"%objectId%", w.oid(), "%tax_income%", treasury, "%tax_income_reserved%", "0", "%seed_income%", "0", "%withdraw_amount%", amount}
		}
		frames := w.run(t, "manage_vault deposit 1500000")
		assertAnswer(t, frames, []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}, w.f, w.want("vault.htm", vault("1,500,000", "1,500,000")...))
		if !bytes.Equal(frames[0], sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(1_500_000))) {
			t.Fatalf("deposit message = %x", frames[0])
		}
		frames = w.run(t, "manage_vault withdraw 500000")
		assertAnswer(t, frames, []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}, w.f, w.want("vault.htm", vault("1,000,000", "500,000")...))
		if !bytes.Equal(frames[0], sysMsg(serverpackets.SystemMessageEarnedS1Adena, numberParam(500_000))) {
			t.Fatalf("withdraw message = %x", frames[0])
		}
		assertAnswer(t, w.run(t, "manage_vault withdraw 2000000"), pageThenFinal, w.f, w.want("vault-no.htm", vault("1,000,000", "2,000,000")...))
		assertAnswer(t, w.run(t, "manage_vault"), pageThenFinal, w.f, w.want("vault.htm", vault("1,000,000", "0")...))
		if got := w.held(t, item.AdenaID); got != chamberlainAdena-1_000_000 {
			t.Fatalf("adena = %d, want %d", got, chamberlainAdena-1_000_000)
		}
		// A malformed amount stops the handling: not even ActionFailed.
		if got := w.run(t, "manage_vault deposit x"); len(got) != 0 {
			t.Fatalf("malformed amount = % x, want nothing", opcodes(got))
		}

		taxes := []string{"%currentTaxRate%", "15", "%nextTaxRate%", "15", "%objectId%", w.oid()}
		assertAnswer(t, w.run(t, "tax_set"), pageThenFinal, w.f, w.want("tax-leader.htm", append(taxes, "%taxLimit%", "25")...))
		assertAnswer(t, w.run(t, "tax_adjust 26"), pageThenFinal, w.f, w.want("tax-toohigh.htm", "%objectId%", w.oid(), "%taxLimit%", "25"))
		assertAnswer(t, w.run(t, "tax_adjust 25"), pageThenFinal, w.f, w.want("tax-after.htm", "%nextTaxRate%", "25", "%objectId%", w.oid()))
		if treasury, next, _, _, _ := w.castleRow(t); treasury != 1_000_000 || next != 25 {
			t.Fatalf("stored treasury %d next tax %d, want 1000000 and 25", treasury, next)
		}
	})
	t.Run("member without the tax privilege", func(t *testing.T) {
		t.Parallel()
		w := bootChamberlain(t, commandSetup{residenceSetup: residenceSetup{rank: 5, privs: privCastleFunctions}})
		frames := w.run(t, "tax_set")
		if got, want := opcodes(frames), []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}; !bytes.Equal(got, want) {
			t.Fatalf("tax_set = % x, want % x", got, want)
		}
		if _, got, _ := htmlMessage(t, frames[1]); got != w.want("tax-member.htm", "%currentTaxRate%", "15", "%nextTaxRate%", "15", "%objectId%", w.oid()) {
			t.Fatalf("tax-member page = %q", got)
		}
		assertAnswer(t, w.run(t, "manage_vault deposit 10"), pageThenFinal, w.f, w.want("noprivs.htm"))
	})
}

// TestChamberlainCrownReportAndCertificates pins the lord's crown, handed
// once; the report; and the certificate sale, during seal validation only,
// refused to a Dusk talker, and a talker short of the bundle's adena.
func TestChamberlainCrownReportAndCertificates(t *testing.T) {
	t.Parallel()
	t.Run("seal validation", func(t *testing.T) {
		t.Parallel()
		w := bootChamberlain(t, commandSetup{
			residenceSetup: residenceSetup{strife: sevensigns.Dusk},
			period:         sevensigns.SealValidation,
		})
		frames := w.run(t, "give_crown")
		assertAnswer(t, frames, []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}, w.f, w.want("gavecrown.htm", "%CharName%", "Talker", "%FeudName%", "Gludio Castle"))
		if !bytes.Equal(frames[0], sysMsg(serverpackets.SystemMessageYouPickedUpS1, itemNameParam(lordCrownID))) {
			t.Fatalf("crown message = %x", frames[0])
		}
		assertAnswer(t, w.run(t, "give_crown"), pageThenFinal, w.f, w.want("hascrown.htm"))
		if got := w.held(t, lordCrownID); got != 1 {
			t.Fatalf("crowns = %d, want 1", got)
		}

		assertAnswer(t, w.run(t, "receive_report"), pageThenFinal, w.f, w.want("report.htm",
			"%objectId%", w.oid(), "%clanname%", taxClanName, "%clanleadername%", "Talker", "%castlename%", "Gludio Castle",
			"%ss_event%", "Seal Validation", "%ss_avarice%", "Not in Possession", "%ss_gnosis%", "Not in Possession",
			"%ss_strife%", "Revolutionaries of Dusk"))

		_, _, left, _, _ := w.castleRow(t)
		sale := func(left int) string {
			return w.want("sell-dawn-ticket.htm", "%left%", strconv.Itoa(left), "%bundle%", "10", "%price%", "1000", "%objectId%", w.oid())
		}
		assertAnswer(t, w.run(t, "manor_certificate"), pageThenFinal, w.f, sale(left))
		frames = w.run(t, "validate_certificate")
		assertAnswer(t, frames, []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}, w.f, w.want("not-enough-adena.htm", "%objectId%", w.oid()))
		if !bytes.Equal(frames[0], sysMsg(serverpackets.SystemMessageYouNotEnoughAdena)) {
			t.Fatalf("short message = %x", frames[0])
		}
		w.giveAdena(t, 10_000)
		frames = w.run(t, "validate_certificate")
		assertAnswer(t, frames, []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}, w.f, sale(left-10))
		if !bytes.Equal(frames[1], sysMsg(serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(certificateID), itemNumberParam(10))) {
			t.Fatalf("certificates message = %x", frames[1])
		}
		if got := w.held(t, certificateID); got != 10 {
			t.Fatalf("certificates held = %d, want 10", got)
		}
		if _, _, stored, _, _ := w.castleRow(t); stored != left-10 {
			t.Fatalf("stored certificates = %d, want %d", stored, left-10)
		}
		// manor_certificate matches exactly.
		assertAnswer(t, w.run(t, "Manor_Certificate"), []byte{serverpackets.OpcodeActionFailed}, w.f, "")
	})
	t.Run("outside seal validation", func(t *testing.T) {
		t.Parallel()
		w := bootChamberlain(t, commandSetup{period: sevensigns.Competition})
		assertAnswer(t, w.run(t, "validate_certificate"), pageThenFinal, w.f, w.want("not-dawn-or-event.htm", "%objectId%", w.oid()))
	})
}

// TestChamberlainGateAndTrapUpgrades walks the lord through a gate and a
// trap upgrade under a Dawn Seal of Strife: the confirmation pages show the
// discounted prices, the upgrade takes them and stores the ratio and the
// level, and asking again for as much is refused.
func TestChamberlainGateAndTrapUpgrades(t *testing.T) {
	t.Parallel()
	w := bootChamberlain(t, commandSetup{residenceSetup: residenceSetup{strife: sevensigns.Dawn}, gates: true, lifeTower: true})
	w.giveAdena(t, chamberlainAdena)
	gate := strconv.Itoa(castleGateA)
	assertAnswer(t, w.run(t, "doors_update"), pageThenFinal, w.f, w.want("35100-gu.htm", "%objectId%", w.oid()))
	assertAnswer(t, w.run(t, "doors_update "+gate+" 1"), pageThenFinal, w.f, w.want("doors-update.htm", "%id%", gate, "%type%", "1", "%objectId%", w.oid()))
	assertAnswer(t, w.run(t, "doors_choose_upgrade "+gate+" 1 2"), pageThenFinal, w.f, w.want("doors-confirm.htm",
		"%objectId%", w.oid(), "%id%", gate, "%level%", "2", "%type%", "1", "%price%", "240000"))
	frames := w.run(t, "doors_confirm_upgrade "+gate+" 1 2")
	if _, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); !ok {
		t.Fatalf("upgrade = % x, want a page", opcodes(frames))
	}
	if got := pageIn(t, frames); got != w.want("doors-success.htm", "%objectId%", w.oid()) {
		t.Fatalf("upgrade page = %q", got)
	}
	if got := w.held(t, item.AdenaID); got != chamberlainAdena-240_000 {
		t.Fatalf("adena after gate = %d", got)
	}
	assertAnswer(t, w.run(t, "doors_confirm_upgrade "+gate+" 1 2"), pageThenFinal, w.f, w.want("doors-already-updated.htm", "%level%", "200", "%objectId%", w.oid()))
	// An unpriced ratio and a door that is no gate answer nothing.
	assertAnswer(t, w.run(t, "doors_confirm_upgrade "+gate+" 1 4"), []byte{serverpackets.OpcodeActionFailed}, w.f, "")
	assertAnswer(t, w.run(t, "doors_confirm_upgrade "+strconv.Itoa(frontierGate)+" 1 3"), []byte{serverpackets.OpcodeActionFailed}, w.f, "")

	assertAnswer(t, w.run(t, "traps_update"), pageThenFinal, w.f, w.want("35100-tu.htm", "%objectId%", w.oid()))
	assertAnswer(t, w.run(t, "traps_update 0"), pageThenFinal, w.f, w.want("traps-update.htm", "%trapIndex%", "0", "%objectId%", w.oid()))
	assertAnswer(t, w.run(t, "traps_choose_upgrade 0 1"), pageThenFinal, w.f, w.want("traps-confirm.htm",
		"%objectId%", w.oid(), "%trapIndex%", "0", "%level%", "1", "%price%", "2400000"))
	frames = w.run(t, "traps_confirm_upgrade 0 1")
	if got := pageIn(t, frames); got != w.want("traps-success.htm", "%objectId%", w.oid()) {
		t.Fatalf("trap page = %q", got)
	}
	assertAnswer(t, w.run(t, "traps_confirm_upgrade 0 1"), pageThenFinal, w.f, w.want("traps-already-updated.htm", "%level%", "1", "%objectId%", w.oid()))
	// A trap of no control tower stops the handling.
	if got := w.run(t, "traps_confirm_upgrade 3 2"); len(got) != 0 {
		t.Fatalf("trap of no tower = % x, want nothing", opcodes(got))
	}
	if got := w.held(t, item.AdenaID); got != chamberlainAdena-240_000-2_400_000 {
		t.Fatalf("adena after trap = %d", got)
	}
	w.srv.FlushPersistence(t)
	var ratio, level int
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT hp FROM castle_doorupgrade WHERE doorId = ?`, castleGateA).Scan(&ratio); err != nil || ratio != 2 {
		t.Fatalf("stored gate ratio = %d (%v), want 2", ratio, err)
	}
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT level FROM castle_trapupgrade WHERE castleId = ? AND towerIndex = 0`, taxCastleID).Scan(&level); err != nil || level != 1 {
		t.Fatalf("stored trap level = %d (%v), want 1", level, err)
	}
}

// TestChamberlainDoorsManorAndSiegeTime pins operate_door, which opens and
// closes the castle's gates; the manor pages; the buy window of a product
// list; and the siege date pages, siege_time_set moving the siege to 20:00
// and closing its date.
func TestChamberlainDoorsManorAndSiegeTime(t *testing.T) {
	t.Parallel()
	products := buylist.NewList(351001, castleChamberlainID, []buylist.Product{{ItemID: 57, Price: 1, MaxCount: -1}})
	w := bootChamberlain(t, commandSetup{gates: true, extra: []gameservertest.Option{gameservertest.WithBuyLists(products)}})
	assertAnswer(t, w.run(t, "operate_door"), pageThenFinal, w.f, w.want("35100-d.htm", "%objectId%", w.oid()))
	gates := strconv.Itoa(castleGateA) + " " + strconv.Itoa(castleGateB) + " " + strconv.Itoa(frontierGate)
	frames := w.command(t, w.f, "operate_door 1 "+gates)
	if ids, open, _ := gateStates(frames); len(ids) != 2 || !open[0] || !open[1] {
		t.Fatalf("opened gates = %v %v, want both castle gates open", ids, open)
	}
	if got := pageIn(t, frames); got != w.want("doors-open.htm", "%objectId%", w.oid()) {
		t.Fatalf("open page = %q", got)
	}
	if ids, open, _ := gateStates(w.command(t, w.f, "operate_door 0 "+gates)); len(ids) != 2 || open[0] || open[1] {
		t.Fatalf("closed gates = %v %v, want both castle gates closed", ids, open)
	}

	assertAnswer(t, w.run(t, "manor 0"), pageThenFinal, w.f, w.want("manor/manor.htm", "%objectId%", w.oid()))
	assertAnswer(t, w.run(t, "manor 4 2"), pageThenFinal, w.f, w.want("manor/manor_help002.htm", "%objectId%", w.oid()))
	assertAnswer(t, w.run(t, "manor 9"), pageThenFinal, w.f, w.want("no.htm", "%objectId%", w.oid()))
	if _, ok := firstOpcode(w.run(t, "items 35100"), serverpackets.OpcodeBuyList); !ok {
		t.Fatal("items 35100 opened no buy window of list 351001")
	}
	// Commands the chamberlain does not know reach the merchant's.
	assertAnswer(t, w.run(t, "Chat 1"), chatThenFinal, w.f, w.want("merchant-1.htm", "%objectId%", w.oid()))

	c, _ := w.srv.Castles.Get(taxCastleID)
	day := time.Date(2030, 6, 9, 20, 0, 0, 0, time.Local)
	c.SetSiegeDate(day.UnixMilli())
	c.SetTimeRegistrationOver(false)
	assertAnswer(t, w.run(t, "siege_change"), pageThenFinal, w.f, w.filledFile(t, "siegetime3.htm"))
	assertAnswer(t, w.run(t, "siege_time_set 0"), pageThenFinal, w.f, w.filledFile(t, "siegetime6.htm"))
	assertAnswer(t, w.run(t, "siege_time_set 1 4"), pageThenFinal, w.f, w.filledFile(t, "siegetime8.htm"))
	want := time.Date(2030, 6, 9, 16, 0, 0, 0, time.Local).UnixMilli()
	if _, _, _, date, over := w.castleRow(t); date != want || over != "true" {
		t.Fatalf("stored siege date %d over %q, want %d true", date, over, want)
	}
	assertAnswer(t, w.run(t, "siege_change"), pageThenFinal, w.f, w.filledFile(t, "siegetime2.htm"))
	c.SetSiegeDate(time.Now().Add(time.Hour).UnixMilli())
	assertAnswer(t, w.run(t, "siege_change"), pageThenFinal, w.f, w.filledFile(t, "siegetime1.htm"))
}

// TestChamberlainBanishForeigners: banish_foreigner throws a player of
// another clan standing in the castle grounds out to a banish point of the
// castle, and keeps the lord in place.
func TestChamberlainBanishForeigners(t *testing.T) {
	t.Parallel()
	banish := location.Location{X: 9000, Y: 9000, Z: gameservertest.SpawnZ}
	gludio, err := castledata.NewCastle(castledata.CastleAttrs{
		ID: taxCastleID, Alias: "gludio_castle", Name: "Gludio Castle", CircletID: 6838,
		Tax: residence.Tax{Rate: taxCastleTax}, NPCs: []int{castleChamberlainID},
	}, nil, nil, nil, nil, map[residence.SpawnType][]location.Location{residence.SpawnBanish: {banish}})
	if err != nil {
		t.Fatal(err)
	}
	castles, err := castledata.NewTable([]*castledata.Castle{gludio})
	if err != nil {
		t.Fatal(err)
	}
	form, err := zone.NewCuboid(-2000, 2000, -2000, 2000, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	grounds := zone.NewCastle(1, form)
	grounds.ResidenceID = taxCastleID
	zones := zone.NewIndex()
	zones.Add(grounds)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Talker", playerLevel, 0), gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(chamberlainPages()),
		gameservertest.WithCastles(castles),
		gameservertest.WithZones(zones),
		gameservertest.WithClanSeed(func(db *sql.DB) { seedResidenceChat(t, db, residenceSetup{}) }),
		noBypassReuse,
	)
	w := &chamberlainWorld{folkWorld: &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}}
	srv.SeedCharacterFor(t, "player2", "Bystander", playerLevel, 0)
	bystander := srv.DialClient(t, "player2", 1)
	startInWorld(t, srv, w.c)
	startInWorld(t, srv, bystander)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	w.f = w.spawnKeeper(t, "CastleChamberlain", castleChamberlainID, 30)
	drainUntilQuiet(t, bystander)

	assertAnswer(t, w.run(t, "banish_foreigner_show"), pageThenFinal, w.f, w.filledFile(t, "banishfore.htm"))
	// The lord also sees the bystander leave.
	var answer [][]byte
	for _, f := range w.run(t, "banish_foreigner") {
		if f[0] != serverpackets.OpcodeTeleportToLocation && f[0] != serverpackets.OpcodeDeleteObject {
			answer = append(answer, f)
		}
	}
	assertAnswer(t, answer, pageThenFinal, w.f, w.filledFile(t, "banishafter.htm"))
	teleport, ok := firstOpcode(drainFrames(t, bystander), serverpackets.OpcodeTeleportToLocation)
	if !ok {
		t.Fatal("bystander was not thrown out")
	}
	tx := int(int32(binary.LittleEndian.Uint32(teleport[5:])))
	ty := int(int32(binary.LittleEndian.Uint32(teleport[9:])))
	if abs(tx-banish.X) > 20 || abs(ty-banish.Y) > 20 {
		t.Fatalf("bystander sent to (%d, %d), want within 20 of %+v", tx, ty, banish)
	}
	if px, py, _ := srv.PlayerPosition(t, w.player); px != w.at.X || py != w.at.Y {
		t.Fatalf("lord moved to (%d, %d)", px, py)
	}
}

// TestWyvernManagerReachesChamberlainDialog pins that a wyvern manager's
// commands past its own reach the chamberlain dialog for a member of the
// owning clan, privileges and all, and its manor windows too.
func TestWyvernManagerReachesChamberlainDialog(t *testing.T) {
	t.Parallel()
	s := commandSetup{residenceSetup: residenceSetup{rank: 5, privs: privCastleFunctions}}
	s.extra = append(s.extra, gameservertest.WithHTMLPages(chamberlainPages()))
	w := bootResidenceCommands(t, s)
	f := w.spawnKeeper(t, "WyvernManagerNpc", wyvernManagerID, 30)
	cw := &chamberlainWorld{folkWorld: w, f: f}
	date := func() string {
		c, _ := w.srv.Castles.Get(taxCastleID)
		return time.UnixMilli(c.SiegeDate()).Format("Mon Jan 02 15:04:05 MST 2006")
	}
	assertAnswer(t, cw.run(t, "products"), pageThenFinal, f, cw.want("products.htm", "%objectId%", cw.oid(), "%npcId%", strconv.Itoa(wyvernManagerID), "%time%", date()))
	assertAnswer(t, cw.run(t, "manage_vault"), pageThenFinal, f, cw.want("noprivs.htm"))
	// The manor windows need the manor privilege too.
	assertAnswer(t, w.bypass(t, "manor_menu_select?ask=5&state=-1&time=0"), pageOnly, f, cw.want("noprivs.htm"))
}

// TestChamberlainPrivilegesTable pins the privilege each command group
// needs, by a member holding only that one.
func TestChamberlainPrivilegesTable(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		priv    int
		allowed string
		refused string
	}{
		{privCastleEntryExit, "operate_door", "castle_devices"},
		{privCastleSetFuncs, "castle_devices", "siege_change"},
		{privCastleSiege, "siege_change", "manor 0"},
		{privCastleManor, "manor 0", "operate_door"},
		{privCastleTaxes, "manage_vault", "products"},
	} {
		t.Run(c.allowed, func(t *testing.T) {
			t.Parallel()
			w := bootChamberlain(t, commandSetup{residenceSetup: residenceSetup{rank: 5, privs: c.priv}})
			if got := pageIn(t, w.run(t, c.allowed)); got == w.want("noprivs.htm") {
				t.Fatalf("%s refused with privilege %d", c.allowed, c.priv)
			}
			assertAnswer(t, w.run(t, c.refused), pageThenFinal, w.f, w.want("noprivs.htm"))
		})
	}
}
