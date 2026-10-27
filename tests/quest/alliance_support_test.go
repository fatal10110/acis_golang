package quest

import (
	"context"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/ketraorcsupport"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/varkasilenossupport"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
	"github.com/rs/zerolog"
)

// What the Ketra and Varka support scenarios (tests/quest/testdata/
// scenarios/{ketra,varka}_support_*, alliance_support_*) cannot show: the
// soul guide's cast of a bought blessing (a scenario civilian has no MP),
// the gatekeeper's teleport windows (a scenario world has no teleport
// lists), the warehouse keeper with a warehouse that holds something (a
// scenario seeds none), the grocer's answer to a player with karma, and the
// demotion of every ally of the killer's party (a scenario seeds no party
// with a standing).

// The support features and the village folk the tests talk to.
const (
	ketraSupport = "script.feature.KetraOrcSupport"
	varkaSupport = "script.feature.VarkaSilenosSupport"
	asefaID      = 31372
	atanID       = 31373
	jaffID       = 31374
	hornID       = 7186
	footmanID    = 21324
)

// supportPages are the two features' datapack pages.
func supportPages(t *testing.T) map[string]string {
	t.Helper()
	pages := map[string]string{}
	for _, name := range []string{"KetraOrcSupport", "VarkaSilenosSupport"} {
		dir := datapack.Path(t, "data", "html", "script", "feature", name)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			data, err := os.ReadFile(dir + "/" + e.Name())
			if err != nil {
				t.Fatal(err)
			}
			pages["script/feature/"+name+"/"+e.Name()] = string(data)
		}
	}
	return pages
}

// supportItems is the shared catalog with the badges and the horn.
func supportItems() gameservertest.Option {
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{7211, 7212, 7213, 7214, 7215, 7221, 7222, 7223, 7224, 7225, hornID} {
		templates = append(templates, &item.Template{
			ID: id, Name: "Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	return gameservertest.WithItemTemplates(item.NewTable(templates))
}

// supportScripts registers both features, the folk ids civilians and the
// footman hostile.
func supportScripts() gameservertest.Option {
	kinds := map[int32]script.NPCKind{footmanID: script.KindHostile}
	for id := int32(31370); id <= 31383; id++ {
		kinds[id] = script.KindFolk
	}
	return gameservertest.WithNPCScripts(kinds,
		[]script.Listing{{Path: ketraSupport}, {Path: varkaSupport}},
		script.Catalog{ketraSupport: ketraorcsupport.New, varkaSupport: varkasilenossupport.New})
}

// bootSupport boots a level 40 character of standing with both features,
// runs before and enters the world.
func bootSupport(t *testing.T, standing int, before func(srv *gameservertest.Server, objID int32), opts ...gameservertest.Option) *dialogWorld {
	t.Helper()
	datapack.Require(t)
	return bootSeamQuest(t, 40, supportPages(t), func(srv *gameservertest.Server, objID int32) {
		seedStanding(t, srv, objID, standing)
		if before != nil {
			before(srv, objID)
		}
	}, append([]gameservertest.Option{supportScripts(), supportItems()}, opts...)...)
}

// firstTalk talks to the civilian obj and renders the talk's packets.
func (w *dialogWorld) firstTalk(t *testing.T, obj int32) []string {
	t.Helper()
	return w.renderTail(t, w.interact(t, obj))
}

// shownPage reports whether lines show the page html of the NPC role, its
// object id filled in (rendered as {role}); line ends and the trailing
// newline are left out of the comparison.
func shownPage(lines []string, role, html string) bool {
	norm := func(s string) string { return strings.TrimSpace(strings.ReplaceAll(s, "\r", "")) }
	html = norm(strings.ReplaceAll(html, "%objectId%", "{"+role+"}"))
	return slices.ContainsFunc(lines, func(l string) bool {
		i := strings.Index(l, `html="`)
		if !strings.HasPrefix(l, "S NpcHtmlMessage ") || i < 0 {
			return false
		}
		text, err := strconv.Unquote(l[i+len("html="):])
		return err == nil && norm(text) == html
	})
}

// TestAllianceSupportBlessingCast (KetraOrcSupport.onAdvEvent): a Ketra
// ally of level 5 holding 6 horns buys Haste; Asefa answers the bought
// page, takes the 6 horns and, on a later AI tick, casts Haste (4357, level
// 1) on the ally.
func TestAllianceSupportBlessingCast(t *testing.T) {
	t.Parallel()
	w := bootSupport(t, 5, func(srv *gameservertest.Server, objID int32) {
		srv.GiveItem(t, objID, hornID, 6)
	}, gameservertest.WithAITask())
	skills, err := gamexml.LoadSkillDefinitions(datapack.Path(t, "data", "xml", "skills"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	tmpl := gameservertest.FolkTemplate("Folk", asefaID)
	tmpl.MPMax = 1493
	asefa := w.srv.SpawnCastingFolkNPCAt(t, tmpl, location.Location{X: w.at.X + 20, Y: w.at.Y, Z: w.at.Z}, skills)
	w.roles[asefa.ObjectID()], w.npcs["asefa"] = "asefa", asefa.ObjectID()
	w.roles[w.player] = "player"
	w.srv.ReadQueued(t, w.srv.Client)

	pages := supportPages(t)
	if got := w.firstTalk(t, asefa.ObjectID()); !shownPage(got, "asefa", pages["script/feature/KetraOrcSupport/31372-4.htm"]) {
		t.Fatalf("talk = %q, want 31372-4.htm", got)
	}
	got := w.bypass(t, "Quest KetraOrcSupport 7")
	if !slices.Contains(got, "S SystemMessage id=301 3:7186 6:6") || !slices.Contains(got, "S ActionFailed") {
		t.Fatalf("blessing answer = %q, want the 6 horns taken", got)
	}
	if n := w.srv.PlayerItemCount(t, w.player, hornID); n != 0 {
		t.Fatalf("horns after the blessing = %d, want 0", n)
	}
	want := "S MagicSkillUse caster=asefa target=player skill=4357-1"
	var cast []string
	for range 3 {
		if err := w.srv.AI.Tick(); err != nil {
			t.Fatal(err)
		}
		w.srv.Settle(t)
		for _, f := range w.srv.ReadQueued(t, w.srv.Client) {
			if f[0] == serverpackets.OpcodeMagicSkillUse {
				line, err := scriptcontract.Packet(f, w.roles)
				if err != nil {
					t.Fatal(err)
				}
				cast = append(cast, line)
			}
		}
	}
	if !slices.ContainsFunc(cast, func(l string) bool { return strings.HasPrefix(l, want) }) {
		t.Fatalf("after three AI ticks = %q, want %s", cast, want)
	}
}

// noQuestPage is the page a refused event answers.
const noQuestPage = "<html><body>You are either not on a quest that involves this NPC, or you don't meet this NPC's minimum quest requirements.</body></html>"

// TestAllianceSupportWarehouseWithdraw (onFirstTalk, onAdvEvent
// "Withdraw"): the warehouse keeper of a Ketra ally of level 4 whose
// warehouse holds an item answers its warehouse page; the withdrawal
// releases the client, opens the withdrawal list, then answers the no-quest
// page, as the event's answer stays the reference's default.
func TestAllianceSupportWarehouseWithdraw(t *testing.T) {
	t.Parallel()
	w := bootSupport(t, 4, func(srv *gameservertest.Server, objID int32) {
		obj := srv.GiveItem(t, objID, hornID, 1)
		if _, err := srv.DB.ExecContext(context.Background(), "UPDATE items SET loc = 'WAREHOUSE' WHERE object_id = ?", obj); err != nil {
			t.Fatal(err)
		}
	})
	jaff := w.spawnFolk(t, "jaff", jaffID, 20, 0, 0)
	if got := w.firstTalk(t, jaff.ObjectID()); !shownPage(got, "jaff", supportPages(t)["script/feature/KetraOrcSupport/31374-4.htm"]) {
		t.Fatalf("talk = %q, want 31374-4.htm", got)
	}
	w.srv.Client.Send(encodeBypass("Quest KetraOrcSupport Withdraw"))
	var ops []byte
	var noQuest bool
	for _, f := range w.srv.ReadQueued(t, w.srv.Client) {
		switch f[0] {
		case serverpackets.OpcodeActionFailed, serverpackets.OpcodeWarehouseWithdrawList:
			ops = append(ops, f[0])
		case serverpackets.OpcodeNpcHtmlMessage:
			ops = append(ops, f[0])
			noQuest = shownPage(w.renderTail(t, [][]byte{f}), "jaff", noQuestPage)
		}
	}
	want := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeWarehouseWithdrawList, serverpackets.OpcodeNpcHtmlMessage}
	if len(ops) < len(want) || !slices.Equal(ops[:len(want)], want) || !noQuest {
		t.Fatalf("withdraw answer opcodes = %x (no-quest page %v), want %x first, then the no-quest page", ops, noQuest, want)
	}
}

// TestAllianceSupportGrocerRefusesKarma (onFirstTalk): the grocer answers
// a player with karma its PK page, whatever the alliance.
func TestAllianceSupportGrocerRefusesKarma(t *testing.T) {
	t.Parallel()
	w := bootSupport(t, 3, func(srv *gameservertest.Server, objID int32) {
		if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET karma = 100 WHERE obj_Id = ?", objID); err != nil {
			t.Fatal(err)
		}
	})
	atan := w.spawnFolk(t, "atan", atanID, 20, 0, 0)
	if got := w.firstTalk(t, atan.ObjectID()); !shownPage(got, "atan", supportPages(t)["script/feature/KetraOrcSupport/31373-pk.htm"]) {
		t.Fatalf("talk = %q, want 31373-pk.htm", got)
	}
}

// TestAllianceSupportDemotesTheKillersParty (onMyDying, testKetraDemote):
// Leader (Ketra 5, level 5 badge), Mate (Ketra 2, level 2 badge) and Third
// (Varka 3, level 3 Varka badge) are one party; Leader kills a Ketra
// footman. Leader and Mate lose the alliance, each badge going down a
// level; Third keeps the Varka alliance and its badge.
func TestAllianceSupportDemotesTheKillersParty(t *testing.T) {
	t.Parallel()
	datapack.Require(t)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Leader", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		supportScripts(),
		supportItems(),
	)
	leader := srv.SoleObjectID(t)
	mate := srv.SeedCharacterFor(t, "mate", "Mate", 40, 0).ID
	third := srv.SeedCharacterFor(t, "third", "Third", 40, 0).ID
	for _, m := range []struct {
		id       int32
		standing int
		badge    int32
	}{{leader, 5, 7215}, {mate, 2, 7212}, {third, -3, 7223}} {
		seedStanding(t, srv, m.id, m.standing)
		srv.GiveItem(t, m.id, m.badge, 1)
	}
	enterWorld(t, srv)
	members := map[int32]*testsupport.ScriptedClient{leader: srv.Client}
	for _, m := range []struct {
		id            int32
		account, name string
	}{{mate, "mate", "Mate"}, {third, "third", "Third"}} {
		c := srv.DialClient(t, m.account, 1)
		c.Send(encodeRequestGameStart(0))
		readUntil(t, c, serverpackets.OpcodeCharSelected)
		c.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
		srv.ReadQueued(t, c)
		srv.ReadQueued(t, srv.Client)
		srv.Client.Send(encodeJoinParty(m.name))
		srv.ReadQueued(t, c)
		c.Send(encodeAnswerJoinParty(1))
		srv.ReadQueued(t, c)
		srv.ReadQueued(t, srv.Client)
		members[m.id] = c
	}
	for _, c := range members {
		srv.ReadQueued(t, c)
	}

	x, y, z := srv.PlayerPosition(t, leader)
	footman := srv.SpawnHostileNPCTemplateAt(t, proofMonster(footmanID), location.Location{X: x + 40, Y: y, Z: z})
	killOnPlayerQueue(t, srv, leader, footman)

	names := map[int32]string{leader: "Leader", mate: "Mate", third: "Third"}
	for id, w := range map[int32]struct {
		standing int32
		items    map[int32]int
	}{
		leader: {0, map[int32]int{7215: 0, 7214: 1}},
		mate:   {0, map[int32]int{7212: 0, 7211: 1}},
		third:  {-3, map[int32]int{7223: 1, 7222: 0}},
	} {
		for itemID, n := range w.items {
			if got := srv.PlayerItemCount(t, id, itemID); got != n {
				t.Errorf("%s's item %d = %d, want %d", names[id], itemID, got, n)
			}
		}
		var standing int32
		if !srv.RunScript(t, id, "KetraOrcSupport", func(_ *script.Script, p *script.Player) { standing = p.VarkaKetraAlliance() }) {
			t.Fatal("standing read panicked")
		}
		if standing != w.standing {
			t.Errorf("%s's standing = %d, want %d", names[id], standing, w.standing)
		}
	}
}
