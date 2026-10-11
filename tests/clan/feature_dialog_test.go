package clan

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/alliance"
	featureclan "github.com/fatal10110/acis_golang/internal/gameserver/script/feature/clan"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// featurePages are the datapack pages of the clan and alliance dialogs and
// the village master's own chat page, which links to both.
func featurePages(t *testing.T) map[string]string {
	t.Helper()
	pages := map[string]string{}
	add := func(rel string) {
		data, err := os.ReadFile(datapack.Path(t, append([]string{"data", "html"}, strings.Split(rel, "/")...)...))
		if err != nil {
			t.Fatal(err)
		}
		pages[rel] = string(data)
	}
	add("villagemaster/" + strconv.Itoa(masterID) + ".htm")
	for _, dir := range []string{"Clan", "Alliance"} {
		entries, err := os.ReadDir(datapack.Path(t, "data", "html", "script", "feature", dir))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			add("script/feature/" + dir + "/" + e.Name())
		}
	}
	return pages
}

// bootFeatureWorld is bootClanWorld with the clan and alliance dialogs
// registered from their scripts.xml paths, the master's chat page the
// datapack's, linking to them.
func bootFeatureWorld(t *testing.T) *clanWorld {
	t.Helper()
	list := []script.Listing{{Path: "script.feature.Alliance"}, {Path: "script.feature.Clan"}}
	catalog := script.Catalog{"script.feature.Alliance": alliance.New, "script.feature.Clan": featureclan.New}
	return bootClanWorld(t, 10, 0, 0,
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{masterID: script.KindFolk}, list, catalog),
		gameservertest.WithHTMLPages(featurePages(t)))
}

// talk has c select and talk to the master, and requires the master's own
// chat page among the answer, which may also carry the approach and the
// master's talk animation.
func (w *clanWorld) talk(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(encodeAction(w.master.ObjectID(), w.at))
	w.srv.ReadQueued(t, c)
	c.Send(encodeAction(w.master.ObjectID(), w.at))
	if got, want := pageText(t, w.srv.ReadQueued(t, c)), w.page(t, "villagemaster/"+strconv.Itoa(masterID)+".htm"); got != want {
		t.Fatalf("talk page = %q, want the master's chat page", got)
	}
}

// send has c send command and returns the answer.
func (w *clanWorld) send(t *testing.T, c *testsupport.ScriptedClient, command string) [][]byte {
	t.Helper()
	c.Send(encodeBypass(command))
	return w.srv.ReadQueued(t, c)
}

// wantPage requires frames to be the datapack page file shown through the
// master, then failures ActionFailed packets.
func (w *clanWorld) wantPage(t *testing.T, frames [][]byte, file string, failures int) {
	t.Helper()
	want := []byte{serverpackets.OpcodeNpcHtmlMessage}
	want = append(want, bytes.Repeat([]byte{serverpackets.OpcodeActionFailed}, failures)...)
	if got := opcodes(frames); !bytes.Equal(got, want) {
		t.Fatalf("answer for %s = %x, want %x", file, got, want)
	}
	if got := pageText(t, frames); got != w.page(t, file) {
		t.Fatalf("page = %q, want %s", got, file)
	}
}

// page returns the datapack page file as the master shows it: line ends
// read as the page cache keeps them, and the master's object id filled in.
func (w *clanWorld) page(t *testing.T, file string) string {
	t.Helper()
	data, err := os.ReadFile(datapack.Path(t, append([]string{"data", "html"}, strings.Split(file, "/")...)...))
	if err != nil {
		t.Fatal(err)
	}
	page := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
	if !strings.HasSuffix(page, "\n") {
		page += "\n"
	}
	return strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(w.master.ObjectID())))
}

// openDialog has c talk to the master and follow its link to the dialog of
// the feature name, which answers with its first page.
func (w *clanWorld) openDialog(t *testing.T, c *testsupport.ScriptedClient, name, first string) {
	t.Helper()
	w.talk(t, c)
	frames := w.send(t, c, "npc_"+strconv.Itoa(int(w.master.ObjectID()))+"_Quest "+name)
	w.wantPage(t, frames, "script/feature/"+name+"/"+first, 2)
}

// clanEvent is one link of the clan dialog.
type clanEvent struct {
	// via is the open page linking to event when the first page does not.
	via, event string
	// refused is the page a player who leads no clan gets.
	refused string
}

// clanEvents are the clan dialog's links.
func clanEvents() []clanEvent {
	return []clanEvent{
		{"", "9000-02.htm", "9000-02.htm"},
		{"", "9000-03.htm", "9000-03-no.htm"},
		{"", "9000-04.htm", "9000-04-no.htm"},
		{"", "9000-05.htm", "9000-05-no.htm"},
		{"", "9000-06.htm", "9000-06.htm"},
		{"9000-06.htm", "9000-07.htm", "9000-07-no.htm"},
		{"9000-06.htm", "9000-08.htm", "9000-07-no.htm"},
		{"", "9000-12.htm", "9000-12.htm"},
		{"9000-12.htm", "9000-12a.htm", "9000-07-no.htm"},
		{"9000-12.htm", "9000-12b.htm", "9000-12b.htm"},
		{"", "9000-13.htm", "9000-13.htm"},
		{"9000-13.htm", "9000-13a.htm", "9000-07-no.htm"},
		{"9000-13.htm", "9000-13b.htm", "9000-07-no.htm"},
		{"", "9000-14.htm", "9000-14.htm"},
		{"9000-14.htm", "9000-14a.htm", "9000-07-no.htm"},
		{"9000-14.htm", "9000-15.htm", "9000-07-no.htm"},
	}
}

// clanEvent opens the clan dialog for c and follows its links to event,
// which answers with the page file.
func (w *clanWorld) clanEvent(t *testing.T, c *testsupport.ScriptedClient, via, event, file string) {
	t.Helper()
	w.openDialog(t, c, "Clan", "9000-01.htm")
	if via != "" {
		w.wantPage(t, w.send(t, c, "Quest Clan "+via), "script/feature/Clan/"+via, 1)
	}
	w.wantPage(t, w.send(t, c, "Quest Clan "+event), "script/feature/Clan/"+file, 1)
}

// allyEvent opens the alliance dialog for c and follows its link to event,
// which answers with the page file.
func (w *clanWorld) allyEvent(t *testing.T, c *testsupport.ScriptedClient, event, file string) {
	t.Helper()
	w.openDialog(t, c, "Alliance", "9001-01.htm")
	w.wantPage(t, w.send(t, c, "Quest Alliance "+event), "script/feature/Alliance/"+file, 1)
}

// refusedAll follows every link of both dialogs as c, who leads no clan:
// each leader's page answers with the page refusing it, each open page
// with itself.
func (w *clanWorld) refusedAll(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	for _, e := range clanEvents() {
		w.clanEvent(t, c, e.via, e.event, e.refused)
	}
	for _, event := range []string{"9001-02.htm", "9001-03.htm"} {
		w.allyEvent(t, c, event, "9001-04.htm")
	}
}

// TestClanAndAllianceDialogsWithoutAClan follows a village master's Clan
// and Alliance links as a player in no clan: each opens its dialog's first
// page, a leader's page is refused with the page saying so, and an open
// page is answered.
func TestClanAndAllianceDialogsWithoutAClan(t *testing.T) {
	w := bootFeatureWorld(t)
	w.refusedAll(t, w.member)
}

// TestClanAndAllianceDialogsForMemberAndLeader founds a clan through the
// page the clan dialog opens and recruits a member. The member is refused
// every leader's page as a player in no clan is; the leader gets every
// page it asks for, and reaches the nomination command through the page
// the dialog opened.
func TestClanAndAllianceDialogsForMemberAndLeader(t *testing.T) {
	w := bootFeatureWorld(t)
	master := "npc_" + strconv.Itoa(int(w.master.ObjectID())) + "_"

	w.clanEvent(t, w.leader, "", "9000-02.htm", "9000-02.htm")
	frames := w.send(t, w.leader, master+"create_clan Knights")
	list, ok := firstOpcode(frames, serverpackets.OpcodePledgeShowMemberListAll)
	if !ok {
		t.Fatalf("create_clan answer = %x, want PledgeShowMemberListAll", opcodes(frames))
	}
	r := wire.NewReader(list[1:])
	r.ReadInt32()
	clanID := r.ReadInt32()
	w.recruit(t)
	w.srv.ReadQueued(t, w.leader)
	w.srv.ReadQueued(t, w.member)

	w.refusedAll(t, w.member)

	for _, e := range clanEvents() {
		w.clanEvent(t, w.leader, e.via, e.event, e.event)
	}
	for _, event := range []string{"9001-02.htm", "9001-03.htm"} {
		w.allyEvent(t, w.leader, event, event)
	}

	w.clanEvent(t, w.leader, "9000-06.htm", "9000-07.htm", "9000-07.htm")
	if page := pageText(t, w.send(t, w.leader, master+"change_clan_leader Recruit")); !strings.Contains(page, "is a success") {
		t.Fatalf("nomination page = %q, want 9000-07-success", page)
	}
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT new_leader_id FROM clan_data WHERE clan_id = ?`, clanID); got != int64(w.memberID) {
		t.Fatalf("stored nominee = %d, want %d", got, w.memberID)
	}
}
