package npcs

import (
	"context"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/blackjudge"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/noblesseteleporter"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The Black Judge, the gatekeeper the Noblesse teleporter talks through and
// that gatekeeper's destinations: a standard one, then one for the
// Noblesse Gate Pass and one for adena, which the window lists by their
// index among all three.
const (
	blackJudgeID     = 30981
	nobleGatekeeper  = 30080
	noblePassID      = 6651
	nobleAdenaPrice  = 1000
	judgedLevel      = 20
	judgedPenalty    = 2
	judgeFeeAtLevel  = 8640
	judgeFeeIndex    = "4"
	judgeOpenedPage  = "black_judge001.htm"
	judgeFeePage     = "black_judge003.htm"
	judgeShortPage   = "black_judge008.htm"
	judgeNothingPage = "black_judge009.htm"
)

var (
	nobleStandardSpot = location.Location{X: 15000, Y: 25000, Z: -3000}
	noblePassSpot     = location.Location{X: -87328, Y: 142266, Z: -3640}
	nobleAdenaSpot    = location.Location{X: 73579, Y: 142709, Z: -3768}
)

// proofWorld is a booted server running the Black Judge and the Noblesse
// teleporter from their scripts.xml paths, with its character in the world.
type proofWorld struct {
	*folkWorld
}

// bootProofWorld boots the two scripts with the datapack's pages, sets the
// character's death penalty level and noblesse status before it enters the
// world, and gives it adena.
func bootProofWorld(t *testing.T, penalty int, noble bool, adena int32) *proofWorld {
	t.Helper()
	pages := map[string]string{"test/any.htm": anyNpcPage}
	addPage := func(rel string) {
		data, err := os.ReadFile(datapack.Path(t, append([]string{"data", "html"}, strings.Split(rel, "/")...)...))
		if err != nil {
			t.Fatal(err)
		}
		pages[rel] = string(data)
	}
	addPage("gatekeeper/" + strconv.Itoa(nobleGatekeeper) + ".htm")
	for _, dir := range []string{"feature/BlackJudge", "teleport/NoblesseTeleporter"} {
		entries, err := os.ReadDir(datapack.Path(t, append([]string{"data", "html", "script"}, strings.Split(dir, "/")...)...))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			addPage("script/" + dir + "/" + e.Name())
		}
	}
	list := []script.Listing{{Path: "script.feature.BlackJudge"}, {Path: "script.teleport.NoblesseTeleporter"}}
	catalog := script.Catalog{
		"script.feature.BlackJudge":          blackjudge.New,
		"script.teleport.NoblesseTeleporter": noblesseteleporter.New,
	}
	kinds := map[int32]script.NPCKind{blackJudgeID: script.KindFolk, nobleGatekeeper: script.KindFolk}
	teleports := travel.TeleportTable{nobleGatekeeper: {
		{Location: nobleStandardSpot, Description: "Town of Dion", Kind: travel.KindStandard, PriceID: int(item.AdenaID), PriceCount: 5000},
		{Location: noblePassSpot, Description: "Gludin Arena", Kind: travel.KindNobleHuntingZonePass, PriceID: noblePassID, PriceCount: 1},
		{Location: nobleAdenaSpot, Description: "Giran Arena", Kind: travel.KindNobleHuntingZoneAdena, PriceID: int(item.AdenaID), PriceCount: nobleAdenaPrice},
	}}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Judged", judgedLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(pages),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithTeleports(teleports, nil, false, newTeleportClock().now),
		noBypassReuse,
	)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	if _, err := srv.DB.ExecContext(context.Background(),
		`UPDATE characters SET death_penalty_level = ?, nobless = ? WHERE obj_Id = ?`, penalty, noble, w.player); err != nil {
		t.Fatal(err)
	}
	if adena > 0 {
		srv.GiveItem(t, w.player, item.AdenaID, adena)
	}
	startInWorld(t, w.srv, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	return &proofWorld{w}
}

// scriptPage returns the datapack page rel as n shows it: line ends read
// as the page cache keeps them, and n's object id filled in.
func scriptPage(t *testing.T, rel string, n *npc.Folk) string {
	t.Helper()
	data, err := os.ReadFile(datapack.Path(t, append([]string{"data", "html"}, strings.Split(rel, "/")...)...))
	if err != nil {
		t.Fatal(err)
	}
	page := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
	if !strings.HasSuffix(page, "\n") {
		page += "\n"
	}
	return strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(n.ObjectID())))
}

// dialogFrames keeps the frames a dialog answer is judged by: pages,
// releases, system messages and the status bar update.
func dialogFrames(frames [][]byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed,
			serverpackets.OpcodeSystemMessage, serverpackets.OpcodeEtcStatusUpdate:
			out = append(out, f)
		}
	}
	return out
}

// wantPage requires frames to be page shown through n, then failures
// ActionFailed packets.
func wantPage(t *testing.T, frames [][]byte, n *npc.Folk, page string, failures int) {
	t.Helper()
	frames = dialogFrames(frames)
	want := append([]byte{serverpackets.OpcodeNpcHtmlMessage}, slices.Repeat([]byte{serverpackets.OpcodeActionFailed}, failures)...)
	if got := opcodes(frames); !slices.Equal(got, want) {
		t.Fatalf("answer = %x, want %x", got, want)
	}
	if obj, html, _ := htmlMessage(t, frames[0]); obj != n.ObjectID() || html != page {
		t.Fatalf("page through %d = %q, want through %d %q", obj, html, n.ObjectID(), page)
	}
}

// wantTalkPage is wantPage for the answer to a talk, whose own release
// may come before the page.
func wantTalkPage(t *testing.T, frames [][]byte, n *npc.Folk, page string) {
	t.Helper()
	frames = dialogFrames(frames)
	if i := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeNpcHtmlMessage }); i > 0 {
		frames = frames[i:]
	}
	wantPage(t, frames, n, page, 1)
}

// penaltyAndAdena returns the character's death penalty level and adena.
func (w *proofWorld) penaltyAndAdena(t *testing.T) (int, int) {
	t.Helper()
	var level int
	w.srv.RunQuest(t, w.player, "BlackJudge", func(_ *script.Quests, c *player.Character, _ *script.Script) {
		level = c.DeathPenaltyLevel()
	})
	return level, w.srv.PlayerItemCount(t, w.player, item.AdenaID)
}

// openJudge talks to the judge, whose first talk answers with its opening
// page, and follows its death penalty link to the fee page of the
// character's level.
func (w *proofWorld) openJudge(t *testing.T, judge *npc.Folk) {
	t.Helper()
	w.selectFolk(t, judge)
	wantTalkPage(t, w.talk(t, judge, false), judge, scriptPage(t, "script/feature/BlackJudge/"+judgeOpenedPage, judge))
	wantPage(t, w.bypass(t, "Quest BlackJudge test_dp"), judge, scriptPage(t, "script/feature/BlackJudge/"+judgeFeePage, judge), 1)
}

// TestBlackJudgeLiftsDeathPenaltyForItsFee plays the Black Judge as a
// level 20 character with two levels of death penalty: the first talk
// opens the judge's page and its link the level's fee page. Short of the
// fee the judge says so and nothing changes; paying it, from the fee page
// opened again, takes the adena and
// lifts one level, with the level message and the status update and no
// page; the second payment lifts the penalty, and the judge then says
// nothing is left to heal.
func TestBlackJudgeLiftsDeathPenaltyForItsFee(t *testing.T) {
	t.Parallel()
	w := bootProofWorld(t, judgedPenalty, false, judgeFeeAtLevel-1)
	judge := w.spawnFolk(t, folkTemplate("Folk", blackJudgeID), 50)
	remove := "Quest BlackJudge remove_dp " + judgeFeeIndex

	w.openJudge(t, judge)
	wantPage(t, w.bypass(t, remove), judge, scriptPage(t, "script/feature/BlackJudge/"+judgeShortPage, judge), 1)
	if level, adena := w.penaltyAndAdena(t); level != judgedPenalty || adena != judgeFeeAtLevel-1 {
		t.Fatalf("short of the fee: penalty %d adena %d, want %d and %d", level, adena, judgedPenalty, judgeFeeAtLevel-1)
	}

	w.srv.RunScript(t, w.player, "BlackJudge", func(s *script.Script, p *script.Player) {
		s.GiveItems(p, item.AdenaID, judgeFeeAtLevel+1)
	})
	w.srv.ReadQueued(t, w.c)
	w.openJudge(t, judge)
	w.c.Send(encodeBypass(remove))
	paid := dialogFrames(w.srv.ReadQueued(t, w.c))
	wantPaid := func(t *testing.T, frames [][]byte, left int32) {
		t.Helper()
		want := []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeEtcStatusUpdate}
		if got := opcodes(frames); !slices.Equal(got, want) {
			t.Fatalf("paid answer = %x, want the adena message, the penalty message and the status update", got)
		}
		r := wire.NewReader(frames[0][1:])
		if id, n, typ, count := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != serverpackets.SystemMessageS1DisappearedAdena || n != 1 ||
			typ != serverpackets.SystemMessageParamNumber || count != judgeFeeAtLevel {
			t.Fatalf("adena message = %d (%d params, type %d, %d), want %d adena disappeared", id, n, typ, count, judgeFeeAtLevel)
		}
		r = wire.NewReader(frames[1][1:])
		if left > 0 {
			if id, n, typ, level := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != serverpackets.SystemMessageDeathPenaltyLevelS1Added || n != 1 ||
				typ != serverpackets.SystemMessageParamNumber || level != left {
				t.Fatalf("penalty message = %d (%d params, type %d, level %d), want level %d", id, n, typ, level, left)
			}
		} else if id, n := r.ReadInt32(), r.ReadInt32(); id != serverpackets.SystemMessageDeathPenaltyLifted || n != 0 {
			t.Fatalf("penalty message = %d (%d params), want the penalty lifted", id, n)
		}
	}
	wantPaid(t, paid, judgedPenalty-1)
	if level, adena := w.penaltyAndAdena(t); level != judgedPenalty-1 || adena != judgeFeeAtLevel {
		t.Fatalf("after one payment: penalty %d adena %d, want %d and %d", level, adena, judgedPenalty-1, judgeFeeAtLevel)
	}

	w.openJudge(t, judge)
	w.c.Send(encodeBypass(remove))
	wantPaid(t, dialogFrames(w.srv.ReadQueued(t, w.c)), 0)
	if level, adena := w.penaltyAndAdena(t); level != 0 || adena != 0 {
		t.Fatalf("after two payments: penalty %d adena %d, want none", level, adena)
	}

	w.openJudge(t, judge)
	wantPage(t, w.bypass(t, remove), judge, scriptPage(t, "script/feature/BlackJudge/"+judgeNothingPage, judge), 1)
}

// TestBlackJudgeRefusesFeesItDoesNotOffer sends the judge fee links its
// pages never offer, from a page admitting any: a fee for a higher level
// than the character's, an index past the fees, and malformed indexes each
// answer nothing and leave the penalty and the adena alone.
func TestBlackJudgeRefusesFeesItDoesNotOffer(t *testing.T) {
	t.Parallel()
	w := bootProofWorld(t, judgedPenalty, false, 1_000_000)
	judge := w.spawnFolk(t, folkTemplate("Folk", blackJudgeID), 50)
	w.openJudge(t, judge)
	w.openAnyNpcPage(t)
	for _, event := range []string{"remove_dp 0", "remove_dp 3", "remove_dp 6", "remove_dp 99999999999", "remove_dp", "remove_dp x", "remove_dp -1", "remove_dp  4"} {
		w.c.Send(encodeBypass("Quest BlackJudge " + event))
		if got := dialogFrames(w.srv.ReadQueued(t, w.c)); len(got) != 0 {
			t.Fatalf("%q answered %x, want nothing", event, opcodes(got))
		}
	}
	if level, adena := w.penaltyAndAdena(t); level != judgedPenalty || adena != 1_000_000 {
		t.Fatalf("penalty %d adena %d, want both untouched", level, adena)
	}
}

// nobleWindow is a gatekeeper's window listing rows, as the client
// receives it.
func nobleWindow(rows ...string) string {
	return "<html><body>&$556;<br><br>" + strings.Join(rows, "") + "</body></html>"
}

// nobleRow is the window row of the destination index of the gatekeeper
// n, priced.
func nobleRow(n *npc.Folk, index int, desc, price string) string {
	return `<a action="bypass -h npc_` + strconv.Itoa(int(n.ObjectID())) + `_teleport ` + strconv.Itoa(index) +
		`" msg="811;` + desc + `">` + desc + " - " + price + "</a><br1>"
}

// TestNoblesseTeleporterRefusesNonNoble follows a gatekeeper's Noblesse
// link as a character who is no noblesse: the refusal page.
func TestNoblesseTeleporterRefusesNonNoble(t *testing.T) {
	t.Parallel()
	w := bootProofWorld(t, 0, false, 0)
	gk := w.spawnFolk(t, folkTemplate("Gatekeeper", nobleGatekeeper), 50)
	w.selectFolk(t, gk)
	wantTalkPage(t, w.talk(t, gk, false), gk, scriptPage(t, "gatekeeper/"+strconv.Itoa(nobleGatekeeper)+".htm", gk))
	wantPage(t, w.bypass(t, "npc_"+strconv.Itoa(int(gk.ObjectID()))+"_Quest NoblesseTeleporter"), gk,
		scriptPage(t, "script/teleport/NoblesseTeleporter/nobleteleporter-no.htm", gk), 2)
}

// TestNoblesseTeleporterOpensHuntingGroundWindows plays the Noblesse link
// of a gatekeeper as a noblesse: the Noblesse page, whose two links open
// the gatekeeper's window of Gate Pass destinations and of adena ones,
// each with no release and its index among all the gatekeeper's
// destinations. A destination of the adena window takes the character
// there for its price. An event naming a type in another case opens that
// type's window; one naming no type answers nothing.
func TestNoblesseTeleporterOpensHuntingGroundWindows(t *testing.T) {
	t.Parallel()
	w := bootProofWorld(t, 0, true, 5000)
	gk := w.spawnFolk(t, folkTemplate("Gatekeeper", nobleGatekeeper), 50)
	open := func(t *testing.T) {
		t.Helper()
		w.selectFolk(t, gk)
		w.talk(t, gk, false)
		wantPage(t, w.bypass(t, "npc_"+strconv.Itoa(int(gk.ObjectID()))+"_Quest NoblesseTeleporter"), gk,
			scriptPage(t, "script/teleport/NoblesseTeleporter/noble.htm", gk), 2)
	}
	passWindow := nobleWindow(nobleRow(gk, 1, "Gludin Arena", "1 &#6651;"))
	adenaWindow := nobleWindow(nobleRow(gk, 2, "Giran Arena", "1000 &#57;"))

	open(t)
	wantPage(t, w.bypass(t, "Quest NoblesseTeleporter NOBLE_HUNTING_ZONE_PASS"), gk, passWindow, 0)

	open(t)
	w.openAnyNpcPage(t)
	wantPage(t, w.bypass(t, "Quest NoblesseTeleporter noble_hunting_zone_pass"), gk, passWindow, 0)
	w.c.Send(encodeBypass("Quest NoblesseTeleporter UNKNOWN_TYPE"))
	if got := dialogFrames(w.srv.ReadQueued(t, w.c)); len(got) != 0 {
		t.Fatalf("an event naming no type answered %x, want nothing", opcodes(got))
	}

	open(t)
	wantPage(t, w.bypass(t, "Quest NoblesseTeleporter NOBLE_HUNTING_ZONE_ADENA"), gk, adenaWindow, 0)
	frames := w.tripFrames(t, "npc_"+strconv.Itoa(int(gk.ObjectID()))+"_teleport 2")
	if _, ok := firstOpcode(frames, serverpackets.OpcodeTeleportToLocation); !ok {
		t.Fatalf("trip answer = %x, want a teleport", opcodes(frames))
	}
	if got := w.srv.PlayerItemCount(t, w.player, item.AdenaID); got != 5000-nobleAdenaPrice {
		t.Fatalf("adena after the trip = %d, want %d", got, 5000-nobleAdenaPrice)
	}
}
