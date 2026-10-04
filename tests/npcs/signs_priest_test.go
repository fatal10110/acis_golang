package npcs

import (
	"context"
	"database/sql"
	"errors"
	"maps"
	"slices"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Seven Signs dialog items (SevenSignsManager) and NPCs.
const (
	recordOfSevenSigns    int32 = 5707
	certificateOfApproval int32 = 6388
	blueStone             int32 = 6360
	greenStone            int32 = 6361
	redStone              int32 = 6362

	dawnPriestID     = 31078
	duskPriestID     = 31085
	blackMarketeerID = 31092
	merchantMammonID = 31113
)

// Seven Signs dialog system messages, by their reference ids.
const (
	msgContribIncreased = 1267
	msgJoinedDawn       = 1273
	msgJoinedDusk       = 1274
	msgFightForAvarice  = 1275
	msgFightForGnosis   = 1276
	msgFightForStrife   = 1277
	msgContribExceeded  = 1279
)

var (
	htmlFrame = []byte{serverpackets.OpcodeNpcHtmlMessage}
	released  = []byte{serverpackets.OpcodeActionFailed}
)

// priestPageNames are the seven_signs pages the dialog opens in these
// scenarios, each answering with its own name.
var priestPageNames = []string{
	"signs_2_dawn", "signs_2_dawn_no", "signs_2_dusk_no",
	"signs_3_dawn", "signs_8_dusk", "signs_12", "signs_19_Avarice_dawn", "desc_1",
	"signs_33_dawn", "signs_33_dusk", "signs_33_dawn_fee", "signs_33_dawn_firstclass", "signs_33_dusk_firstclass",
	"signs_33_dawn_member", "signs_33_dusk_member", "signs_33_dawn_no", "signs_33_dusk_no",
	"signs_4_dawn", "signs_4_dusk",
	"signs_5_dawn", "signs_5_dawn_no",
	"signs_6_dawn", "signs_6_dawn_no_stones", "signs_6_dawn_failure", "signs_6_dawn_low_stones",
	"signs_9_dawn_a", "signs_9_dawn_b",
	"signs_16_dawn", "signs_18_dawn", "signs_18_dawn_no_stones", "signs_18_dawn_low_stones", "signs_18_dawn_failed",
	"blkmrkt_3", "blkmrkt_4", "blkmrkt_5",
}

// priestPages is signsPages plus the dialog's pages and the page that
// admits every npc_ command; the stone forms show their placeholders.
func priestPages() map[string]string {
	pages := signsPages()
	maps.Copy(pages, dialogPages())
	for _, name := range priestPageNames {
		path := "seven_signs/" + name + ".htm"
		pages[path] = "<html><body>" + path + " %objectId%</body></html>"
	}
	pages["seven_signs/signs_6_dawn_contribute.htm"] = "<html><body>contribute %stoneColor% %stoneCount% %stoneItemId% %objectId%</body></html>"
	pages["seven_signs/signs_17_dawn.htm"] = "<html><body>exchange %stoneColor% %stoneValue% %stoneCount% %stoneItemId% %objectId%</body></html>"
	return pages
}

// priestTemplates is the behavior catalog plus the dialog's items.
func priestTemplates() *item.Table {
	currency := func(id int32, name string) *item.Template {
		return &item.Template{
			ID: id, Name: name, Kind: item.KindEtcItem, Duration: -1, Stackable: true,
			Dropable: true, Tradable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{},
		}
	}
	return item.NewTable(append(gameservertest.ItemTemplates().All(),
		currency(item.AncientAdenaID, "Ancient Adena"), currency(recordOfSevenSigns, "Record of Seven Signs"),
		currency(certificateOfApproval, "Certificate of Approval"), currency(blueStone, "Blue Seal Stone"),
		currency(greenStone, "Green Seal Stone"), currency(redStone, "Red Seal Stone")))
}

// classTemplates gives the fixture class body to the Warrior (1) and the
// Gladiator (2).
func classTemplates() gameservertest.Option {
	warrior, gladiator := gameservertest.ClassTemplate(), gameservertest.ClassTemplate()
	warrior.ID, warrior.Skills = warriorClass, nil
	gladiator.ID, gladiator.Skills = gladiatorClass, nil
	return gameservertest.WithClassTemplates(warrior, gladiator)
}

// bootPriestWorld enters the world holding held (template id to count),
// seeded by the statements of seed.
func bootPriestWorld(t *testing.T, held map[int32]int32, seed ...string) *folkWorld {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Talker", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(priestPages()),
		gameservertest.WithItemTemplates(priestTemplates()),
		classTemplates(),
		gameservertest.WithClanSeed(seedStatements(t, seed...)),
		noBypassReuse,
	)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	for _, id := range slices.Sorted(maps.Keys(held)) {
		srv.GiveItem(t, w.player, id, held[id])
	}
	startInWorld(t, w.srv, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	return w
}

// selectedPriest spawns a civilian of kind and npcID and selects it.
func (w *folkWorld) selectedPriest(t *testing.T, kind string, npcID, dx int) *npc.Folk {
	t.Helper()
	f := w.spawnFolk(t, folkTemplate(kind, npcID), dx)
	w.selectFolk(t, f)
	return f
}

// ask sends command to f from a page admitting it and keeps the frames
// a dialog answer is judged by.
func (w *folkWorld) ask(t *testing.T, f *npc.Folk, command string) [][]byte {
	t.Helper()
	w.openAnyNpcPage(t)
	return w.tripFrames(t, npcCommand(f, command))
}

// assertDialog checks frames are want and that the page they open, if
// any, is page.
func assertDialog(t *testing.T, what string, frames [][]byte, f *npc.Folk, page string, want ...[]byte) {
	t.Helper()
	assertFrames(t, what, frames, want...)
	frame, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	if !ok {
		return
	}
	if objectID, got, _ := htmlMessage(t, frame); objectID != f.ObjectID() || got != page {
		t.Fatalf("%s: page = %d %q, want %d %q", what, objectID, got, f.ObjectID(), page)
	}
}

// chatWindow is a dialog answer opening a chat window after messages: the
// page, its own release, then the dispatcher's.
func chatWindow(messages ...[]byte) [][]byte {
	return append(slices.Clone(messages), htmlFrame, released, released)
}

// signsPage is the fixture page name as f shows it.
func signsPage(f *npc.Folk, name string) string {
	return wantSignsPage("seven_signs/"+name+".htm", f)
}

// signsRow is Talker's persisted sign-up.
type signsRow struct {
	cabal, seal                          string
	red, green, blue, adena, contributed int
}

// savedSignsRow waits for queued saves and reads Talker's seven_signs row;
// ok is false without one.
func (w *folkWorld) savedSignsRow(t *testing.T) (signsRow, bool) {
	t.Helper()
	w.srv.FlushPersistence(t)
	var r signsRow
	err := w.srv.DB.QueryRowContext(context.Background(), `SELECT cabal, seal, red_stones, green_stones, blue_stones,
		ancient_adena_amount, contribution_score FROM seven_signs WHERE char_obj_id = ?`, w.player).
		Scan(&r.cabal, &r.seal, &r.red, &r.green, &r.blue, &r.adena, &r.contributed)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return r, true
}

// setSignsRow rewrites Talker's sign-up columns and reloads the state.
func (w *folkWorld) setSignsRow(t *testing.T, set string) {
	t.Helper()
	// A save queued by the last turn-in lands first, so the reload reads it.
	w.srv.FlushPersistence(t)
	ctx := context.Background()
	if _, err := w.srv.DB.ExecContext(ctx, `UPDATE seven_signs SET `+set+` WHERE char_obj_id = ?`, w.player); err != nil {
		t.Fatal(err)
	}
	if err := w.srv.SevenSigns.Restore(ctx); err != nil {
		t.Fatal(err)
	}
}

// assertHeld checks the live and the persisted count of each template.
func (w *folkWorld) assertHeld(t *testing.T, want map[int32]int) {
	t.Helper()
	for _, id := range slices.Sorted(maps.Keys(want)) {
		if got := w.held(t, id); got != want[id] {
			t.Fatalf("item %d held = %d, want %d", id, got, want[id])
		}
		if got := w.savedCount(t, id); got != want[id] {
			t.Fatalf("item %d saved = %d, want %d", id, got, want[id])
		}
	}
}

func destroyed(id int32, count int32) []byte {
	if count > 1 {
		return sysMsg(serverpackets.SystemMessageS2S1Disappeared, itemNameParam(id), itemNumberParam(count))
	}
	return sysMsg(serverpackets.SystemMessageS1Disappeared, itemNameParam(id))
}

func ancientAdenaEarned(n int32) []byte {
	return sysMsg(serverpackets.SystemMessageEarnedS2S1S, itemNameParam(item.AncientAdenaID), numberParam(n))
}

func contribIncreased(n int32) []byte { return sysMsg(msgContribIncreased, itemNumberParam(n)) }

// TestSignsPriestSellsRecord pins "SevenSigns 2": 500 adena taken, then the
// Record of Seven Signs handed over as picked up, then the priest's own
// page; a talker short of adena hears so and gets the _no page, the Dusk
// Priestess's for her. The record and the adena are persisted.
func TestSignsPriestSellsRecord(t *testing.T) {
	t.Parallel()
	w := bootPriestWorld(t, map[int32]int32{item.AdenaID: 1000})
	dawn := w.selectedPriest(t, "DawnPriest", dawnPriestID, 30)

	sold := chatWindow(sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(500)),
		sysMsg(serverpackets.SystemMessageYouPickedUpS1, itemNameParam(recordOfSevenSigns)))
	for range 2 {
		assertDialog(t, "buy record", w.ask(t, dawn, "SevenSigns 2"), dawn, signsPage(dawn, "signs_2_dawn"), sold...)
	}
	short := chatWindow(sysMsg(serverpackets.SystemMessageYouNotEnoughAdena))
	assertDialog(t, "buy record broke", w.ask(t, dawn, "SevenSigns 2"), dawn, signsPage(dawn, "signs_2_dawn_no"), short...)
	w.assertHeld(t, map[int32]int{item.AdenaID: 0, recordOfSevenSigns: 2})

	dusk := w.selectedPriest(t, "DuskPriest", duskPriestID, 40)
	assertDialog(t, "dusk record broke", w.ask(t, dusk, "SevenSigns 2"), dusk, signsPage(dusk, "signs_2_dusk_no"), short...)
}

// TestSignsPriestParticipationGates pins "SevenSigns 33 <cabal>": a member
// of a cabal is told so; a starting class may not join; a first
// occupation joins either cabal freely; past it the castle owners' clans
// may not join the Dusk, and everyone else is asked the Dawn's fee. The
// page suffix follows the NPC, not the cabal asked for.
func TestSignsPriestParticipationGates(t *testing.T) {
	t.Parallel()
	const castleClanID = 268435456
	castleClan := []string{
		`INSERT INTO clan_data (clan_id, clan_name, clan_level, hasCastle, leader_id)
			SELECT ` + strconv.Itoa(castleClanID) + `, 'Lords', 5, 1, obj_Id FROM characters WHERE char_name = 'Talker'`,
		`UPDATE characters SET clanid = ` + strconv.Itoa(castleClanID) + ` WHERE char_name = 'Talker'`,
	}
	class := func(id int) string {
		return `UPDATE characters SET classid = ` + strconv.Itoa(id) + `, base_class = ` + strconv.Itoa(id) + ` WHERE char_name = 'Talker'`
	}
	for _, tc := range []struct {
		name               string
		seed               []string
		member             bool
		dawnPage, duskPage string // answers to "33 2" at the Dawn priest and "33 1" at the Dusk one
	}{
		{"starting class", nil, false, "signs_33_dawn_firstclass", "signs_33_dusk_firstclass"},
		{"first occupation", []string{class(warriorClass)}, false, "signs_33_dawn", "signs_33_dusk"},
		{"second occupation", []string{class(gladiatorClass)}, false, "signs_33_dawn_fee", "signs_33_dusk"},
		{"castle clan", append([]string{class(gladiatorClass)}, castleClan...), false, "signs_33_dawn", "signs_33_dusk_no"},
		{"member", nil, true, "signs_33_dawn_member", "signs_33_dusk_member"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootPriestWorld(t, nil, tc.seed...)
			if tc.member {
				setSevenSigns(t, w, sevensigns.Competition, sevensigns.NoCabal, sevensigns.NoCabal, sevensigns.NoCabal, sevensigns.Dawn)
			}
			dawn := w.selectedPriest(t, "DawnPriest", dawnPriestID, 30)
			assertDialog(t, "33 2", w.ask(t, dawn, "SevenSigns 33 2"), dawn, signsPage(dawn, tc.dawnPage), chatWindow()...)
			dusk := w.selectedPriest(t, "DuskPriest", duskPriestID, 40)
			assertDialog(t, "33 1", w.ask(t, dusk, "SevenSigns 33 1"), dusk, signsPage(dusk, tc.duskPage), chatWindow()...)
		})
	}
}

// TestSignsPriestSignUp pins "SevenSigns 34" and "SevenSigns 4 <cabal>
// <seal>" past the second occupation outside a castle clan: the fee page
// tells whether the talker can pay; joining the Dawn gives up a Certificate
// of Approval, else 50 000 adena, silently, and a talker with neither is
// refused; the Dusk is free. A sign-up says whom the talker joined and for
// which seal, opens the cabal's page and is stored at once.
func TestSignsPriestSignUp(t *testing.T) {
	t.Parallel()
	w := bootPriestWorld(t, map[int32]int32{item.AdenaID: 60000, certificateOfApproval: 1},
		`UPDATE characters SET classid = 2, base_class = 2 WHERE char_name = 'Talker'`)
	dawn := w.selectedPriest(t, "DawnPriest", dawnPriestID, 30)
	unsign := func() {
		setSevenSigns(t, w, sevensigns.Competition, sevensigns.NoCabal, sevensigns.NoCabal, sevensigns.NoCabal, sevensigns.NoCabal)
	}
	joined := func(cabal, seal int32) [][]byte {
		return chatWindow(sysMsg(cabal), sysMsg(seal))
	}
	assertSigned := func(cabal, seal string) {
		t.Helper()
		row, ok := w.savedSignsRow(t)
		if !ok || row.cabal != cabal || row.seal != seal {
			t.Fatalf("seven_signs row = %+v (found %v), want %s %s", row, ok, cabal, seal)
		}
	}

	unsign()
	assertDialog(t, "fee", w.ask(t, dawn, "SevenSigns 34 2"), dawn, signsPage(dawn, "signs_33_dawn"), chatWindow()...)
	assertDialog(t, "join with certificate", w.ask(t, dawn, "SevenSigns 4 2 1"), dawn, signsPage(dawn, "signs_4_dawn"), joined(msgJoinedDawn, msgFightForAvarice)...)
	assertSigned("DAWN", "AVARICE")
	w.assertHeld(t, map[int32]int{certificateOfApproval: 0, item.AdenaID: 60000})

	unsign()
	assertDialog(t, "join with adena", w.ask(t, dawn, "SevenSigns 4 2 2"), dawn, signsPage(dawn, "signs_4_dawn"), joined(msgJoinedDawn, msgFightForGnosis)...)
	assertSigned("DAWN", "GNOSIS")
	w.assertHeld(t, map[int32]int{item.AdenaID: 10000})

	unsign()
	assertDialog(t, "fee broke", w.ask(t, dawn, "SevenSigns 34 2"), dawn, signsPage(dawn, "signs_33_dawn_no"), chatWindow()...)
	assertDialog(t, "join broke", w.ask(t, dawn, "SevenSigns 4 2 3"), dawn, signsPage(dawn, "signs_33_dawn_no"), chatWindow()...)
	if row, ok := w.savedSignsRow(t); ok {
		t.Fatalf("refused sign-up stored %+v", row)
	}
	w.assertHeld(t, map[int32]int{item.AdenaID: 10000})
	assertDialog(t, "join dusk", w.ask(t, dawn, "SevenSigns 4 1 3"), dawn, signsPage(dawn, "signs_4_dusk"), joined(msgJoinedDusk, msgFightForStrife)...)
	assertSigned("DUSK", "STRIFE")
}

// TestSignsPriestStoneContribution pins the seal stone turn-in: "5" opens
// the member's or the non-member's page; "6 <color>" the form to turn one
// color in; "21 <stone> <amount>" turns in as many as held and the cap
// allows, a negative amount taking nothing; "6 4" turns in every stone the
// cap leaves room for, red first; with none left it opens the no-stones
// page, and a capped contribution is refused. Each turn-in names the
// stones taken and the points earned, and is stored at once.
func TestSignsPriestStoneContribution(t *testing.T) {
	t.Parallel()
	w := bootPriestWorld(t, map[int32]int32{blueStone: 10, greenStone: 6, redStone: 3})
	dawn := w.selectedPriest(t, "DawnPriest", dawnPriestID, 30)
	assertDialog(t, "5 unsigned", w.ask(t, dawn, "SevenSigns 5 2"), dawn, signsPage(dawn, "signs_5_dawn_no"), chatWindow()...)
	setSevenSigns(t, w, sevensigns.Competition, sevensigns.NoCabal, sevensigns.NoCabal, sevensigns.NoCabal, sevensigns.Dawn)
	assertDialog(t, "5 member", w.ask(t, dawn, "SevenSigns 5 2"), dawn, signsPage(dawn, "signs_5_dawn"), chatWindow()...)

	form := wantChatPage("<html><body>contribute Blue 10 6360 %objectId%</body></html>", dawn)
	assertDialog(t, "6 1", w.ask(t, dawn, "SevenSigns 6 1"), dawn, form, htmlFrame, released)

	assertDialog(t, "21 four blue", w.ask(t, dawn, "SevenSigns 21 6360 4"), dawn, signsPage(dawn, "signs_6_dawn"),
		chatWindow(destroyed(blueStone, 4), contribIncreased(12))...)
	assertDialog(t, "21 malformed", w.ask(t, dawn, "SevenSigns 21 6360 many"), dawn, signsPage(dawn, "signs_6_dawn_failure"), chatWindow()...)
	assertDialog(t, "21 negative", w.ask(t, dawn, "SevenSigns 21 6360 -5"), dawn, signsPage(dawn, "signs_6_dawn_low_stones"),
		chatWindow(sysMsg(serverpackets.SystemMessageNotEnoughItems))...)
	if row, _ := w.savedSignsRow(t); row.blue != 4 || row.contributed != 12 || row.adena != 12 {
		t.Fatalf("after 21: row = %+v, want 4 blue worth 12", row)
	}
	w.assertHeld(t, map[int32]int{blueStone: 6})

	// 999 995 points leave room for one green stone, taken alone.
	w.setSignsRow(t, "contribution_score = 999995")
	assertDialog(t, "6 4 capped", w.ask(t, dawn, "SevenSigns 6 4"), dawn, signsPage(dawn, "signs_6_dawn"),
		chatWindow(destroyed(greenStone, 1), contribIncreased(5))...)
	assertDialog(t, "6 1 at the cap", w.ask(t, dawn, "SevenSigns 6 1"), dawn, "", sysMsg(msgContribExceeded), released)

	w.setSignsRow(t, "contribution_score = 0")
	assertDialog(t, "6 4", w.ask(t, dawn, "SevenSigns 6 4"), dawn, signsPage(dawn, "signs_6_dawn"),
		chatWindow(destroyed(redStone, 3), destroyed(greenStone, 5), destroyed(blueStone, 6), contribIncreased(30+25+18))...)
	assertDialog(t, "6 4 empty", w.ask(t, dawn, "SevenSigns 6 4"), dawn, signsPage(dawn, "signs_6_dawn_no_stones"), chatWindow()...)
	row, _ := w.savedSignsRow(t)
	if want := (signsRow{cabal: "DAWN", seal: "AVARICE", red: 3, green: 6, blue: 10, adena: 12 + 5 + 73, contributed: 73}); row != want {
		t.Fatalf("after 6 4: row = %+v, want %+v", row, want)
	}
	w.assertHeld(t, map[int32]int{blueStone: 0, greenStone: 0, redStone: 0})
}

// TestSignsPriestStoneExchange pins the exchange of seal stones for ancient
// adena: "16" opens its page, "17 <color>" the form, "18 <stone> <amount>"
// trades that many, refusing an amount it cannot read, one of zero or past
// the stack, and a stone not held; "17 4" trades every stone held.
func TestSignsPriestStoneExchange(t *testing.T) {
	t.Parallel()
	w := bootPriestWorld(t, map[int32]int32{blueStone: 10, greenStone: 4, redStone: 3})
	dawn := w.selectedPriest(t, "DawnPriest", dawnPriestID, 30)

	assertDialog(t, "16", w.ask(t, dawn, "SevenSigns 16"), dawn, signsPage(dawn, "signs_16_dawn"), chatWindow()...)
	form := wantChatPage("<html><body>exchange blue 3 10 6360 %objectId%</body></html>", dawn)
	assertDialog(t, "17 1", w.ask(t, dawn, "SevenSigns 17 1"), dawn, form, htmlFrame, released)
	assertDialog(t, "18 four", w.ask(t, dawn, "SevenSigns 18 6360 4"), dawn, signsPage(dawn, "signs_18_dawn"),
		chatWindow(destroyed(blueStone, 4), ancientAdenaEarned(12))...)
	for _, command := range []string{"SevenSigns 18 6360 0", "SevenSigns 18 6360 99"} {
		assertDialog(t, command, w.ask(t, dawn, command), dawn, signsPage(dawn, "signs_18_dawn_low_stones"), chatWindow()...)
	}
	assertDialog(t, "18 malformed", w.ask(t, dawn, "SevenSigns 18 6360 x"), dawn, signsPage(dawn, "signs_18_dawn_failed"), chatWindow()...)
	assertDialog(t, "17 4", w.ask(t, dawn, "SevenSigns 17 4"), dawn, signsPage(dawn, "signs_18_dawn"),
		chatWindow(destroyed(blueStone, 6), destroyed(greenStone, 4), destroyed(redStone, 3), ancientAdenaEarned(18+20+30))...)
	assertDialog(t, "17 4 empty", w.ask(t, dawn, "SevenSigns 17 4"), dawn, signsPage(dawn, "signs_18_dawn_no_stones"), chatWindow()...)
	assertDialog(t, "18 none held", w.ask(t, dawn, "SevenSigns 18 6360 1"), dawn, signsPage(dawn, "signs_18_dawn_no_stones"), chatWindow()...)
	w.assertHeld(t, map[int32]int{item.AncientAdenaID: 80, blueStone: 0, greenStone: 0, redStone: 0})
}

// TestBlackMarketeerExchangesAncientAdena pins "SevenSigns 7 <amount>" at
// the Black Marketeer of Mammon: ancient adena taken, then as much adena
// earned, then blkmrkt_5; an amount below 1 or unreadable opens blkmrkt_3,
// one past the ancient adena held blkmrkt_4. Its Chat opens its page, and
// the Merchant of Mammon's Chat refuses while no cabal won.
func TestBlackMarketeerExchangesAncientAdena(t *testing.T) {
	t.Parallel()
	w := bootPriestWorld(t, map[int32]int32{item.AncientAdenaID: 100})
	market := w.selectedPriest(t, "SignsPriest", blackMarketeerID, 30)

	assertDialog(t, "7 30", w.ask(t, market, "SevenSigns 7 30"), market, signsPage(market, "blkmrkt_5"),
		chatWindow(destroyed(item.AncientAdenaID, 30), sysMsg(serverpackets.SystemMessageEarnedS1Adena, numberParam(30)))...)
	for command, page := range map[string]string{"SevenSigns 7 0": "blkmrkt_3", "SevenSigns 7 lots": "blkmrkt_3", "SevenSigns 7 500": "blkmrkt_4"} {
		assertDialog(t, command, w.ask(t, market, command), market, signsPage(market, page), chatWindow()...)
	}
	w.assertHeld(t, map[int32]int{item.AncientAdenaID: 70, item.AdenaID: 30})
	assertDialog(t, "Chat 0", w.ask(t, market, "Chat 0"), market, signsPage(market, "blkmrkt_1"), chatWindow()...)

	merchant := w.selectedPriest(t, "SignsPriest", merchantMammonID, 40)
	assertDialog(t, "merchant Chat 0", w.ask(t, merchant, "Chat 0"), merchant, "", sysMsg(serverpackets.SystemMessageQuestEventPeriod), released)
}

// TestSignsPriestCollectsReward pins "SevenSigns 9": during seal validation
// a member of the winning cabal collects what the stones are worth, earned
// as ancient adena with the _a page, the reward cleared and stored at once;
// with nothing left the _b page opens. A member of the losing cabal gets no
// answer of the dialog's own.
func TestSignsPriestCollectsReward(t *testing.T) {
	t.Parallel()
	w := bootPriestWorld(t, nil)
	dawn := w.selectedPriest(t, "DawnPriest", dawnPriestID, 30)
	setSevenSigns(t, w, sevensigns.SealValidation, sevensigns.Dawn, sevensigns.Dawn, sevensigns.Dawn, sevensigns.Dawn)
	w.setSignsRow(t, "blue_stones = 5, ancient_adena_amount = 15, contribution_score = 15")

	assertDialog(t, "9", w.ask(t, dawn, "SevenSigns 9"), dawn, signsPage(dawn, "signs_9_dawn_a"), chatWindow(ancientAdenaEarned(15))...)
	if row, _ := w.savedSignsRow(t); row.adena != 0 || row.blue != 0 || row.contributed != 15 {
		t.Fatalf("after 9: row = %+v, want the reward and stones cleared", row)
	}
	w.assertHeld(t, map[int32]int{item.AncientAdenaID: 15})
	assertDialog(t, "9 again", w.ask(t, dawn, "SevenSigns 9"), dawn, signsPage(dawn, "signs_9_dawn_b"), chatWindow()...)

	setSevenSigns(t, w, sevensigns.SealValidation, sevensigns.Dawn, sevensigns.Dawn, sevensigns.Dawn, sevensigns.Dusk)
	assertDialog(t, "9 loser", w.ask(t, dawn, "SevenSigns 9"), dawn, "", released)
}

// TestSignsPriestPagesAndGate pins the dialog's page commands: 3 and 8
// open the named cabal's page, 19 a seal's, 20 the seal status built from
// the owners, SevenSignsDesc a description, any other number its own page;
// Chat re-opens the priest's cabal page, released first. A command that
// does not parse stops the handling: nothing is sent. Once the talker
// selected another NPC, the priest answers nothing but its Chat.
func TestSignsPriestPagesAndGate(t *testing.T) {
	t.Parallel()
	w := bootPriestWorld(t, nil)
	setSevenSigns(t, w, sevensigns.Competition, sevensigns.NoCabal, sevensigns.Dusk, sevensigns.Dawn, sevensigns.NoCabal)
	dawn := w.selectedPriest(t, "DawnPriest", dawnPriestID, 30)

	for command, page := range map[string]string{
		"SevenSigns 3 2": "signs_3_dawn", "SevenSigns 8 1": "signs_8_dusk", "SevenSigns 19 2 1": "signs_19_Avarice_dawn",
		"SevenSigns 12": "signs_12", "SevenSignsDesc 1": "desc_1",
	} {
		assertDialog(t, command, w.ask(t, dawn, command), dawn, signsPage(dawn, page), chatWindow()...)
	}
	id := strconv.Itoa(int(dawn.ObjectID()))
	status := `<html><body>Priest of Dawn:<br><font color="LEVEL">[ Seal Status ]</font><br>` +
		`[Seal of Avarice: Lords of Dawn]<br>[Seal of Gnosis: Revolutionaries of Dusk]<br>[Seal of Strife: Nothingness]<br>` +
		`<a action="bypass -h npc_` + id + `_Chat 0">Go back.</a></body></html>`
	assertDialog(t, "20", w.ask(t, dawn, "SevenSigns 20 2"), dawn, status, htmlFrame, released)
	assertDialog(t, "Chat 0", w.ask(t, dawn, "Chat 0"), dawn, signsPage(dawn, "dawn_priest_1"), released, htmlFrame, released)
	for _, command := range []string{"SevenSigns 33 7", "SevenSigns", "SevenSigns 4 2 0", "SevenSignsDesc x", "SevenSigns 17 x"} {
		assertDialog(t, command, w.ask(t, dawn, command), dawn, "")
	}

	w.selectedPriest(t, "DuskPriest", duskPriestID, 40)
	assertDialog(t, "unselected", w.ask(t, dawn, "SevenSigns 16"), dawn, "", released)
	assertDialog(t, "unselected Chat", w.ask(t, dawn, "Chat 0"), dawn, signsPage(dawn, "dawn_priest_1"), released, htmlFrame, released)
}
