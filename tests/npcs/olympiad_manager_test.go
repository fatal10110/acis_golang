package npcs

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The Olympiad scenarios play Talker, a level 76 Duelist (class 88), at the
// Grand Olympiad Manager.
const (
	grandOlympiadManager = 31688
	duelistClass         = 88
	noblesseGatePass     = 6651
	// overweightItemObject is the item filling Talker's one inventory slot.
	overweightItemObject = 0x7f310001
)

// Olympiad system messages, by reference id.
const (
	smRegisteredClassified     = 1503
	smRegisteredNoClass        = 1504
	smDeletedFromWaitingList   = 1505
	smNotOnWaitingList         = 1506
	smOlympiadNotInProgress    = 1651
	smAlreadyOnClassList       = 1689
	smAlreadyOnAllClassesList  = 1690
	smGameRequestCannotBeMade  = 1803
	smOlympiadInventoryFull    = 1691
	smYouPickedUpS2S1          = 29
	olympiadDefaultStartPoints = 18
)

// olympiadManagerPages are the shipped Olympiad pages.
func olympiadManagerPages(t *testing.T) map[string]string {
	t.Helper()
	dir := datapack.Path(t, "data", "html", "olympiad")
	matches, err := filepath.Glob(filepath.Join(dir, "*.htm"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("glob olympiad pages: %v (%d)", err, len(matches))
	}
	pages := map[string]string{}
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		pages["olympiad/"+filepath.Base(path)] = string(raw)
	}
	return pages
}

// duelistTemplate is the fixture class body as the Duelist.
func duelistTemplate() gameservertest.Option {
	tmpl := gameservertest.ClassTemplate()
	tmpl.ID = duelistClass
	tmpl.Skills = nil
	return gameservertest.WithClassTemplates(tmpl)
}

// gatePassTemplates is the fixture item catalog with the Noblesse Gate Pass.
func gatePassTemplates() gameservertest.Option {
	templates := append(gameservertest.ItemTemplates().All(), &item.Template{
		ID: noblesseGatePass, Name: "Noblesse Gate Pass", Kind: item.KindEtcItem, Duration: -1, Stackable: true,
		Dropable: true, Tradable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{},
	})
	return gameservertest.WithItemTemplates(item.NewTable(templates))
}

// nobleDuelist makes Talker a noble Duelist, then runs extra.
func nobleDuelist(t *testing.T, extra ...string) gameservertest.Option {
	return gameservertest.WithOlympiadSeed(seedStatements(t, append([]string{
		`UPDATE characters SET nobless = 1, classid = 88, base_class = 88 WHERE char_name = 'Talker'`,
	}, extra...)...))
}

// nobleRecord seeds Talker's record for the running cycle.
func nobleRecord(points int, rewarded bool) string {
	flag := 0
	if rewarded {
		flag = 1
	}
	return fmt.Sprintf(`INSERT INTO olympiad_nobles (char_id, class_id, olympiad_points, competitions_done, competitions_won, competitions_lost, competitions_drawn, rewarded)
		SELECT obj_Id, 88, %d, 0, 0, 0, 0, %d FROM characters WHERE char_name = 'Talker'`, points, flag)
}

// wantOlympiadPage is the shipped Olympiad page name as manager shows it,
// loaded through the page cache:
// each placeholder of fill, given as placeholder and value pairs, replaced,
// then %objectId%.
func wantOlympiadPage(t *testing.T, manager *npc.Folk, name string, fill ...string) string {
	t.Helper()
	raw, ok := olympiadManagerPages(t)["olympiad/"+name]
	if !ok {
		t.Fatalf("no shipped olympiad/%s", name)
	}
	// The page as the server's page cache holds it.
	page, _ := gameservertest.HTMLCache(t, map[string]string{"olympiad/" + name: raw}).Get("olympiad/" + name)
	for i := 0; i+1 < len(fill); i += 2 {
		page = strings.ReplaceAll(page, fill[i], fill[i+1])
	}
	return strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(manager.ObjectID())))
}

// bootOlympiadManager enters the world as Talker at level 76, seeded and
// configured by extra, beside a Grand Olympiad Manager it has selected.
func bootOlympiadManager(t *testing.T, extra ...gameservertest.Option) (*folkWorld, *npc.Folk) {
	t.Helper()
	pages := olympiadManagerPages(t)
	maps.Copy(pages, subclassPages(t))
	opts := append([]gameservertest.Option{noBypassReuse, duelistTemplate(), gatePassTemplates()}, extra...)
	w := bootFolkWorldAs(t, gameservertest.WithCharacter("Talker", 76, 0), pages, opts...)
	manager := w.spawnFolk(t, folkTemplate("OlympiadManagerNpc", grandOlympiadManager), 30)
	w.selectFolk(t, manager)
	return w, manager
}

// nobleRoutes are the links a noble follows from the manager's main page,
// noble_main.htm, to the page offering each OlympiadNoble choice.
var nobleRoutes = map[string][]string{
	"1":  nil,
	"2":  {"Chat 10"},
	"3":  {"Chat 10"},
	"4":  {"Chat 10", "Chat 11"},
	"5":  {"Chat 10", "Chat 12"},
	"6":  {"Chat 14"},
	"10": {"Chat 14", "OlympiadNoble 6"},
}

// offer talks to manager and follows the links to the page offering
// command: an OlympiadNoble choice, or a class ranking.
func (w *folkWorld) offer(t *testing.T, manager *npc.Folk, command string) {
	t.Helper()
	route := []string{"Chat 13"}
	if choice, ok := strings.CutPrefix(command, "OlympiadNoble "); ok {
		route = nobleRoutes[choice]
	}
	if _, ok := firstOpcode(w.talk(t, manager, false), serverpackets.OpcodeNpcHtmlMessage); !ok {
		t.Fatal("talking to the manager opened no page")
	}
	for _, link := range route {
		if _, ok := firstOpcode(w.bypass(t, npcCommand(manager, link)), serverpackets.OpcodeNpcHtmlMessage); !ok {
			t.Fatalf("%s opened no page on the way to %s", link, command)
		}
	}
}

// systemMessageIDs returns the ids of the system messages among frames.
func systemMessageIDs(frames [][]byte) []int32 {
	var ids []int32
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage {
			ids = append(ids, systemMessageID(f))
		}
	}
	return ids
}

// assertNoble opens the page offering the manager's OlympiadNoble choice,
// sends it, and checks it answers with exactly the system messages ids and
// no page.
func (w *folkWorld) assertNoble(t *testing.T, manager *npc.Folk, choice string, ids ...int32) {
	t.Helper()
	w.offer(t, manager, "OlympiadNoble "+choice)
	w.assertNobleAgain(t, manager, choice, ids...)
}

// assertNobleAgain is assertNoble from the page already open.
func (w *folkWorld) assertNobleAgain(t *testing.T, manager *npc.Folk, choice string, ids ...int32) {
	t.Helper()
	frames := w.bypass(t, npcCommand(manager, "OlympiadNoble "+choice))
	if _, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); ok {
		t.Fatalf("OlympiadNoble %s answer = %x, want no page", choice, opcodes(frames))
	}
	if got := systemMessageIDs(frames); !slices.Equal(got, ids) {
		t.Fatalf("OlympiadNoble %s messages = %v, want %v", choice, got, ids)
	}
}

// managerPage opens the page offering command, sends it and returns the
// one page the manager answers with.
func (w *folkWorld) managerPage(t *testing.T, manager *npc.Folk, command string) string {
	t.Helper()
	w.offer(t, manager, command)
	objectID, page := pageOf(t, w.bypass(t, npcCommand(manager, command)))
	if objectID != manager.ObjectID() {
		t.Fatalf("%s page from %d, want the manager %d", command, objectID, manager.ObjectID())
	}
	return page
}

// TestOlympiadRegistration pins OlympiadManager.registerNoble and
// unRegisterNoble driven from the Grand Olympiad Manager during a
// competition window: a classed registration is taken, a second of either
// kind is refused naming the list held, the waiting list counts the
// classes registered for and the non-classed registrations; leaving works
// once, and the class keeps counting on the waiting list once emptied, as
// the reference's map keeps its key. The first registration gives the
// noble its record with the starting points.
func TestOlympiadRegistration(t *testing.T) {
	t.Parallel()
	w, manager := bootOlympiadManager(t, nobleDuelist(t), gameservertest.WithOlympiadCompetition(time.Hour))

	w.assertNoble(t, manager, "5", smRegisteredClassified)
	if n, ok := w.srv.Olympiad.Noble(w.player); !ok || n.Points != olympiadDefaultStartPoints || n.ClassID != duelistClass || n.Name != "Talker" {
		t.Fatalf("record after registering = %+v, %v; want Talker's, class 88, 18 points", n, ok)
	}
	w.assertNobleAgain(t, manager, "5", smAlreadyOnClassList)
	w.assertNoble(t, manager, "4", smAlreadyOnClassList)
	if got, want := w.managerPage(t, manager, "OlympiadNoble 2"), wantOlympiadPage(t, manager, "noble_registered.htm", "%listClassed%", "1", "%listNonClassed%", "0"); got != want {
		t.Fatalf("waiting list page = %q, want %q", got, want)
	}
	if got, want := w.managerPage(t, manager, "OlympiadNoble 3"), wantOlympiadPage(t, manager, "noble_points1.htm", "%points%", "18"); got != want {
		t.Fatalf("points page = %q, want %q", got, want)
	}

	w.assertNoble(t, manager, "1", smDeletedFromWaitingList)
	w.assertNobleAgain(t, manager, "1", smNotOnWaitingList)
	if w.srv.Olympiad.IsRegistered(w.player, duelistClass) {
		t.Fatal("still registered after leaving")
	}

	w.assertNoble(t, manager, "4", smRegisteredNoClass)
	w.assertNoble(t, manager, "5", smAlreadyOnAllClassesList)
	if got, want := w.managerPage(t, manager, "OlympiadNoble 2"), wantOlympiadPage(t, manager, "noble_registered.htm", "%listClassed%", "1", "%listNonClassed%", "1"); got != want {
		t.Fatalf("waiting list page = %q, want %q", got, want)
	}
}

// TestOlympiadRegistrationOutsideTheWindow pins the period checks: outside
// a competition window both registering and leaving are refused as not in
// progress; with less than ten minutes of the window left, registering is
// refused as too late, before the noble is given a record.
func TestOlympiadRegistrationOutsideTheWindow(t *testing.T) {
	t.Parallel()
	t.Run("validation", func(t *testing.T) {
		t.Parallel()
		w, manager := bootOlympiadManager(t, nobleDuelist(t), gameservertest.WithOlympiadValidation())
		w.assertNoble(t, manager, "4", smOlympiadNotInProgress)
		w.assertNoble(t, manager, "5", smOlympiadNotInProgress)
		w.assertNoble(t, manager, "1", smOlympiadNotInProgress)
	})
	t.Run("closing", func(t *testing.T) {
		t.Parallel()
		w, manager := bootOlympiadManager(t, nobleDuelist(t), gameservertest.WithOlympiadCompetition(5*time.Minute))
		w.assertNoble(t, manager, "4", smGameRequestCannotBeMade)
		if _, ok := w.srv.Olympiad.Noble(w.player); ok {
			t.Fatal("a refused registration gave the noble a record")
		}
	})
}

// TestOlympiadRegistrationRefusals pins the manager's gate on a noble
// short of a third occupation, answered with noble_cant_thirdclass.htm, and
// registerNoble's refusal of a record without points, answered with
// noble_nopoints1.htm.
func TestOlympiadRegistrationRefusals(t *testing.T) {
	t.Parallel()
	t.Run("second occupation", func(t *testing.T) {
		t.Parallel()
		w, manager := bootOlympiadManager(t, gameservertest.WithOlympiadSeed(seedStatements(t,
			`UPDATE characters SET nobless = 1, classid = 2, base_class = 2 WHERE char_name = 'Talker'`)),
			gameservertest.WithClassTemplates(gladiatorTemplate()), gameservertest.WithOlympiadCompetition(time.Hour))
		if got, want := w.managerPage(t, manager, "OlympiadNoble 4"), wantOlympiadPage(t, manager, "noble_cant_thirdclass.htm"); got != want {
			t.Fatalf("page = %q, want %q", got, want)
		}
		if w.srv.Olympiad.IsRegistered(w.player, 2) {
			t.Fatal("a Gladiator registered")
		}
	})
	t.Run("no points", func(t *testing.T) {
		t.Parallel()
		w, manager := bootOlympiadManager(t, nobleDuelist(t, nobleRecord(0, false)), gameservertest.WithOlympiadCompetition(time.Hour))
		if got, want := w.managerPage(t, manager, "OlympiadNoble 4"), wantOlympiadPage(t, manager, "noble_nopoints1.htm"); got != want {
			t.Fatalf("page = %q, want %q", got, want)
		}
		if w.srv.Olympiad.IsRegistered(w.player, duelistClass) {
			t.Fatal("a noble without points registered")
		}
	})
	t.Run("overweight", func(t *testing.T) {
		t.Parallel()
		w, manager := bootOlympiadManager(t, nobleDuelist(t,
			`INSERT INTO items (owner_id, object_id, item_id, count, loc, loc_data)
				SELECT obj_Id, `+strconv.Itoa(overweightItemObject)+`, `+strconv.Itoa(noblesseGatePass)+`, 1, 'INVENTORY', 0
				FROM characters WHERE char_name = 'Talker'`),
			gameservertest.WithInventorySlots(1, 1), gameservertest.WithOlympiadCompetition(time.Hour))
		w.assertNoble(t, manager, "4", smOlympiadInventoryFull)
		w.assertNoble(t, manager, "5", smOlympiadInventoryFull)
		if w.srv.Olympiad.IsRegistered(w.player, duelistClass) {
			t.Fatal("a noble with a full inventory registered")
		}
	})
}

// gladiatorTemplate is the fixture class body as the Gladiator.
func gladiatorTemplate() *player.Template {
	tmpl := gameservertest.ClassTemplate()
	tmpl.ID = gladiatorClass
	tmpl.Skills = nil
	return tmpl
}

// TestOlympiadNoblessePasses pins the points' trade for Noblesse Gate
// Passes outside the competition: 6 offers it for 50 points or more, 10
// pays points times 1000 passes, picked up, and marks the record rewarded
// with no points, stored at once; settling again from the same page pays
// nothing. Below 50 points, 6 answers noble_nopoints2.htm; during the
// competition 10 pays nothing.
func TestOlympiadNoblessePasses(t *testing.T) {
	t.Parallel()
	t.Run("trade", func(t *testing.T) {
		t.Parallel()
		w, manager := bootOlympiadManager(t, nobleDuelist(t, nobleRecord(120, false)), gameservertest.WithOlympiadValidation())
		if got, want := w.managerPage(t, manager, "OlympiadNoble 6"), wantOlympiadPage(t, manager, "noble_settle.htm"); got != want {
			t.Fatalf("offer page = %q, want %q", got, want)
		}
		frames := w.bypass(t, npcCommand(manager, "OlympiadNoble 10"))
		msg, ok := firstOpcode(frames, serverpackets.OpcodeSystemMessage)
		if !ok {
			t.Fatalf("trade answer = %x, want the pick-up message", opcodes(frames))
		}
		r := wire.NewReader(msg[1:])
		if id, n := r.ReadInt32(), r.ReadInt32(); id != smYouPickedUpS2S1 || n != 2 {
			t.Fatalf("message = %x, want 29 with two parameters", msg)
		}
		if typ, itemID := r.ReadInt32(), r.ReadInt32(); typ != serverpackets.SystemMessageParamItemName || itemID != noblesseGatePass {
			t.Fatalf("message item = %d %d, want the Noblesse Gate Pass", typ, itemID)
		}
		if typ, count := r.ReadInt32(), r.ReadInt32(); typ != serverpackets.SystemMessageParamItemNumber || count != 120000 {
			t.Fatalf("message count = %d %d, want 120000", typ, count)
		}
		if got := w.savedCount(t, noblesseGatePass); got != 120000 {
			t.Fatalf("passes held = %d, want 120000", got)
		}
		w.srv.FlushPersistence(t)
		var points, rewarded int
		if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT olympiad_points, rewarded FROM olympiad_nobles WHERE char_id = ?`, w.player).Scan(&points, &rewarded); err != nil || points != 0 || rewarded != 1 {
			t.Fatalf("stored record = %d points, rewarded %d, %v; want 0 and 1", points, rewarded, err)
		}

		// The settle page is still open: a second click pays nothing.
		if ids := systemMessageIDs(w.bypass(t, npcCommand(manager, "OlympiadNoble 10"))); len(ids) != 0 {
			t.Fatalf("second trade messages = %v, want none", ids)
		}
		if got := w.savedCount(t, noblesseGatePass); got != 120000 {
			t.Fatalf("passes held after a second trade = %d, want 120000", got)
		}
		if got, want := w.managerPage(t, manager, "OlympiadNoble 6"), wantOlympiadPage(t, manager, "noble_nopoints2.htm"); got != want {
			t.Fatalf("offer page once rewarded = %q, want %q", got, want)
		}
	})
	t.Run("too few points", func(t *testing.T) {
		t.Parallel()
		w, manager := bootOlympiadManager(t, nobleDuelist(t, nobleRecord(49, false)), gameservertest.WithOlympiadValidation())
		if got, want := w.managerPage(t, manager, "OlympiadNoble 6"), wantOlympiadPage(t, manager, "noble_nopoints2.htm"); got != want {
			t.Fatalf("offer page = %q, want %q", got, want)
		}
	})
	t.Run("during the competition", func(t *testing.T) {
		t.Parallel()
		w, manager := bootOlympiadManager(t, nobleDuelist(t, nobleRecord(120, false)), gameservertest.WithOlympiadCompetition(time.Hour))
		w.assertNoble(t, manager, "10")
		if n, _ := w.srv.Olympiad.Noble(w.player); n.Points != 120 || n.Rewarded {
			t.Fatalf("record = %+v, want 120 points, not rewarded", n)
		}
	})
}

// TestOlympiadClassRanking pins "Olympiad 2_<class>": the month's ten best
// of the class with five matches or more, numbered, the places left over
// blank.
func TestOlympiadClassRanking(t *testing.T) {
	t.Parallel()
	w, manager := bootOlympiadManager(t, nobleDuelist(t,
		`INSERT INTO olympiad_nobles_eom (char_id, class_id, olympiad_points, competitions_done, competitions_won, competitions_lost, competitions_drawn)
			SELECT obj_Id, 88, 40, 6, 3, 3, 0 FROM characters WHERE char_name = 'Talker'`))
	fill := []string{"%place1%", "1", "%rank1%", "Talker"}
	for i := 2; i <= 10; i++ {
		fill = append(fill, "%place"+strconv.Itoa(i)+"%", "", "%rank"+strconv.Itoa(i)+"%", "")
	}
	if got := w.managerPage(t, manager, "Olympiad 2_88"); got != wantOlympiadPage(t, manager, "noble_ranking.htm", fill...) {
		t.Fatalf("ranking page = %q, want Talker first, the other places blank", got)
	}
}

// TestOlympiadRegistrationLeftOnLogout pins Player.deleteMe's
// removeDisconnectedCompetitor: a registered noble leaving the world is
// off the waiting list.
func TestOlympiadRegistrationLeftOnLogout(t *testing.T) {
	t.Parallel()
	w, manager := bootOlympiadManager(t, nobleDuelist(t), gameservertest.WithOlympiadCompetition(time.Hour))
	w.assertNoble(t, manager, "4", smRegisteredNoClass)
	w.c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestRestart).Bytes())
	drainFrames(t, w.c)
	w.srv.Settle(t)
	if w.srv.Olympiad.IsRegistered(w.player, duelistClass) {
		t.Fatal("still registered after leaving the world")
	}
}

// TestOlympiadRegistrationSubclassCommand pins VillageMaster's Subclass
// branch: a registered noble sending any Subclass command is taken off the
// waiting list, told so, before the command runs.
func TestOlympiadRegistrationSubclassCommand(t *testing.T) {
	t.Parallel()
	w, manager := bootOlympiadManager(t, nobleDuelist(t), gameservertest.WithOlympiadCompetition(time.Hour),
		gameservertest.WithSubclassRules(true, 0))
	w.assertNoble(t, manager, "5", smRegisteredClassified)
	master := w.spawnFolk(t, folkTemplate("VillageMaster", villageMasterID), 60)
	w.selectFolk(t, master)
	w.talk(t, master, false)
	w.bypass(t, npcCommand(master, "Link villagemaster/SubClass.htm"))
	frames := w.bypass(t, npcCommand(master, "Subclass 1"))
	if ids := systemMessageIDs(frames); len(ids) == 0 || ids[0] != smDeletedFromWaitingList {
		t.Fatalf("Subclass 1 messages = %v, want 1505 first", ids)
	}
	if w.srv.Olympiad.IsRegistered(w.player, duelistClass) {
		t.Fatal("still registered after a subclass command")
	}
}

// TestOlympiadRegistrationBlocksObserving pins Npc.onBypassFeedback's
// observe: a player waiting for an Olympiad match is released without a
// word and does not move; once off the waiting list, it observes.
func TestOlympiadRegistrationBlocksObserving(t *testing.T) {
	t.Parallel()
	groups, err := gamexml.LoadObserverGroups(datapack.Path(t, "data", "xml", "observerGroups.xml"))
	if err != nil {
		t.Fatalf("load observer groups: %v", err)
	}
	w, manager := bootOlympiadManager(t, nobleDuelist(t), gameservertest.WithOlympiadCompetition(time.Hour),
		gameservertest.WithObserverGroups(groups))
	tower := w.spawnFolk(t, folkTemplate("Folk", 31031), 60)
	tower.SetObserverGroups([]int{618})
	observe := func() [][]byte {
		w.selectFolk(t, tower)
		w.talk(t, tower, false)
		w.bypass(t, npcCommand(tower, "observe_group 618"))
		return w.bypass(t, npcCommand(tower, "observe 632"))
	}

	w.assertNoble(t, manager, "4", smRegisteredNoClass)
	frames := observe()
	if got := string(opcodes(frames)); got != string([]byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("observe while registered = %x, want only ActionFailed", got)
	}

	w.selectFolk(t, manager)
	w.assertNoble(t, manager, "1", smDeletedFromWaitingList)
	if _, ok := firstOpcode(observe(), serverpackets.OpcodeObserverStart); !ok {
		t.Fatal("observe once off the waiting list did not start observing")
	}
}
