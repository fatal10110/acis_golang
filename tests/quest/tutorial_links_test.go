package quest

import (
	"fmt"
	"os"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// startClass is what the tutorial shows each starting class at the later
// steps: the pages of question marks 2, 9, 13, 24 and 25 and the radar mark
// of question mark 9 (zero for none), beside the guide page and mark of
// question mark 5.
type startClass struct {
	id                   int
	qm2, qm9, qm13, qm24 string
	qm25                 string
	qm5Page              string
	guideMark, qm9Mark   [3]int32
}

var startClasses = []startClass{
	{0, "tutorial_human_fighter008.htm", "tutorial_fighter017.htm", "tutorial_21.htm", "tutorial_newbie003a.htm", "tutorial_newbie002a.htm", "tutorial_11.htm", [3]int32{-71424, 258336, -3109}, [3]int32{-83020, 242553, -3718}},
	{10, "tutorial_human_mage008.htm", "tutorial_mage017.htm", "tutorial_21a.htm", "tutorial_newbie003a.htm", "tutorial_newbie002b.htm", "tutorial_11.htm", [3]int32{-91036, 248044, -3568}, [3]int32{}},
	{18, "tutorial_elf008.htm", "tutorial_fighter017.htm", "tutorial_21b.htm", "tutorial_newbie003b.htm", "tutorial_newbie002c.htm", "tutorial_11.htm", [3]int32{46112, 41200, -3504}, [3]int32{45061, 52468, -2796}},
	{25, "tutorial_elf008.htm", "tutorial_mage017.htm", "tutorial_21c.htm", "tutorial_newbie003b.htm", "tutorial_newbie002c.htm", "tutorial_11.htm", [3]int32{46112, 41200, -3504}, [3]int32{}},
	{31, "tutorial_delf008.htm", "tutorial_fighter017.htm", "tutorial_21g.htm", "tutorial_newbie003c.htm", "tutorial_newbie002e.htm", "tutorial_11.htm", [3]int32{28384, 11056, -4233}, [3]int32{10447, 14620, -4242}},
	{38, "tutorial_delf008.htm", "tutorial_mage017.htm", "tutorial_21h.htm", "tutorial_newbie003c.htm", "tutorial_newbie002d.htm", "tutorial_11.htm", [3]int32{28384, 11056, -4233}, [3]int32{}},
	{44, "tutorial_orc008.htm", "tutorial_orc_fighter017.htm", "tutorial_21d.htm", "tutorial_newbie003d.htm", "tutorial_newbie002f.htm", "tutorial_11.htm", [3]int32{-56736, -113680, -672}, [3]int32{-46389, -113905, -21}},
	{49, "tutorial_orc008.htm", "tutorial_mage017.htm", "tutorial_21e.htm", "tutorial_newbie003d.htm", "tutorial_newbie002f.htm", "tutorial_11.htm", [3]int32{-56736, -113680, -672}, [3]int32{}},
	{53, "tutorial_dwarven_fighter008.htm", "tutorial_dwarven017.htm", "tutorial_21f.htm", "tutorial_newbie003e.htm", "tutorial_newbie002g.htm", "tutorial_11.htm", [3]int32{108567, -173994, -406}, [3]int32{115271, -182692, -1445}},
}

// secondClass is the page of the class transfer links TE26 (first column),
// TE23 and TE24 for a first class, "" for a link with none.
var secondClass = []struct {
	id      int
	a, b, c string
}{
	{1, "tutorial_22w.htm", "", ""},
	{4, "tutorial_22.htm", "tutorial_22aa.htm", "tutorial_22ab.htm"},
	{7, "tutorial_22b.htm", "tutorial_22ba.htm", "tutorial_22bb.htm"},
	{11, "tutorial_22c.htm", "tutorial_22ca.htm", "tutorial_22cb.htm"},
	{15, "tutorial_22d.htm", "tutorial_22da.htm", "tutorial_22db.htm"},
	{19, "tutorial_22e.htm", "tutorial_22ea.htm", "tutorial_22eb.htm"},
	{22, "tutorial_22f.htm", "tutorial_22fa.htm", "tutorial_22fb.htm"},
	{26, "tutorial_22g.htm", "tutorial_22ga.htm", "tutorial_22gb.htm"},
	{29, "tutorial_22h.htm", "", ""},
	{32, "tutorial_22n.htm", "tutorial_22na.htm", "tutorial_22nb.htm"},
	{35, "tutorial_22o.htm", "tutorial_22oa.htm", "tutorial_22ob.htm"},
	{39, "tutorial_22p.htm", "tutorial_22pa.htm", "tutorial_22pb.htm"},
	{42, "tutorial_22q.htm", "", ""},
	{45, "tutorial_22i.htm", "", ""},
	{47, "tutorial_22j.htm", "", ""},
	{50, "tutorial_22k.htm", "tutorial_22ka.htm", "tutorial_22kb.htm"},
	{54, "tutorial_22l.htm", "", ""},
	{56, "tutorial_22m.htm", "", ""},
}

// tutorialAsk is one request of a newbie playing a class and the frames
// the tutorial answers with.
type tutorialAsk struct {
	name  string
	class int
	req   []byte
	want  func(r *levelRig) []wire.Frame
}

func linkReq(link int) []byte {
	return encodeTutorialString(clientpackets.OpcodeRequestTutorialPassCmdToServer, fmt.Sprintf("TE%d", link))
}

func markReq(mark int32) []byte {
	return encodeTutorialNumber(clientpackets.OpcodeRequestTutorialQuestionMark, mark)
}

// absentPages are the pages the tutorial links to that the datapack does
// not ship: the window shows the missing-page notice for them.
var absentPages = map[string]bool{"tutorial_07.htm": true, "tutorial_07a.htm": true, "tutorial_14.htm": true}

// pageFrame is the tutorial window's frame for the page file: the
// datapack's page, or the missing-page notice for a page it does not ship.
// A page missing that is not listed in absentPages fails the test, so the
// notice never stands in for a typo.
func pageFrame(t *testing.T, file string) wire.Frame {
	t.Helper()
	if absentPages[file] {
		if _, err := os.Stat(datapack.Path(t, "data", "html", "script", "feature", "Tutorial", file)); err == nil {
			t.Fatalf("%s is in the datapack now: drop it from absentPages", file)
		}
		return serverpackets.FrameTutorialShowHTML("<html><body>My html is missing:<br>data/html/script/feature/Tutorial/" + file + "</body></html>")
	}
	return tutorialPageFrame(t, file)
}

// page answers with the page file.
func page(t *testing.T, file string) func(*levelRig) []wire.Frame {
	return func(*levelRig) []wire.Frame { return []wire.Frame{pageFrame(t, file)} }
}

// eventPage answers with the client event switched on to mask, then page.
func eventPage(t *testing.T, mask int32, file string) func(*levelRig) []wire.Frame {
	return func(*levelRig) []wire.Frame {
		return []wire.Frame{serverpackets.FrameTutorialEnableClientEvent(mask), pageFrame(t, file)}
	}
}

// markedPage answers with the radar mark (when there is one) and then page.
func markedPage(t *testing.T, mark [3]int32, file string) func(*levelRig) []wire.Frame {
	return func(*levelRig) []wire.Frame {
		var out []wire.Frame
		if mark != [3]int32{} {
			out = radarMark(mark)
		}
		return append(out, pageFrame(t, file))
	}
}

func nothing(*levelRig) []wire.Frame { return nil }

// run sends each request as the class asked for and requires its frames.
func (r *levelRig) ask(asks []tutorialAsk) {
	r.t.Helper()
	for _, a := range asks {
		onCharacter(r.t, r.srv, r.objID, func(c *player.Character) { c.SetClassID(a.class) })
		r.srv.Client.Send(a.req)
		expectFrames(r.t, a.name, r.srv.ReadQueued(r.t, r.srv.Client), a.want(r)...)
	}
}

// TestTutorialWindowLinks: every link of the tutorial window answers as the
// guide does: the page it opens, the client event it switches on for the
// step, and the voice and the Ex step of the second page. A link no step
// answers shows nothing.
func TestTutorialWindowLinks(t *testing.T) {
	t.Parallel()
	r := bootLevelRig(t, 0)
	asks := []tutorialAsk{
		{"TE0", 0, linkReq(0), func(*levelRig) []wire.Frame { return []wire.Frame{serverpackets.FrameTutorialCloseHTML()} }},
		{"TE12", 0, linkReq(12), func(*levelRig) []wire.Frame { return []wire.Frame{serverpackets.FrameTutorialCloseHTML()} }},
		{"TE2", 0, linkReq(2), func(r *levelRig) []wire.Frame {
			return []wire.Frame{
				serverpackets.FrameTutorialEnableClientEvent(1), voiceFrame(r.objID, r.at, "tutorial_voice_003"),
				tutorialPageFrame(t, "tutorial_02.htm"),
			}
		}},
		{"TE3", 0, linkReq(3), eventPage(t, 2, "tutorial_03.htm")},
		{"TE4", 0, linkReq(4), eventPage(t, 4, "tutorial_04.htm")},
		{"TE5", 0, linkReq(5), eventPage(t, 8, "tutorial_05.htm")},
		{"TE6", 0, linkReq(6), eventPage(t, 16, "tutorial_06.htm")},
		{"TE7", 0, linkReq(7), eventPage(t, 0, "tutorial_100.htm")},
		{"TE8", 0, linkReq(8), eventPage(t, 0, "tutorial_101.htm")},
		{"TE9", 0, linkReq(9), eventPage(t, 0, "tutorial_102.htm")},
		{"TE10", 0, linkReq(10), eventPage(t, 0, "tutorial_103.htm")},
		{"TE11", 0, linkReq(11), eventPage(t, 0, "tutorial_104.htm")},
		{"TE25", 0, linkReq(25), page(t, "tutorial_22cc.htm")},
		{"TE27", 0, linkReq(27), page(t, "tutorial_29.htm")},
		{"TE28", 0, linkReq(28), page(t, "tutorial_28.htm")},
		{"TE29", 0, linkReq(29), page(t, "tutorial_07a.htm")},
		{"TE30", 0, linkReq(30), page(t, "tutorial_07a.htm")},
		{"TE13", 0, linkReq(13), nothing},
		{"TE99", 0, linkReq(99), nothing},
	}
	r.ask(asks)
	if got := r.srv.QuestVars(t, r.objID, tutorial)["Ex"]; got != "-5" {
		t.Fatalf("Ex after TE2 = %q, want -5", got)
	}

	r.ask([]tutorialAsk{{"TE1", 0, linkReq(1), func(r *levelRig) []wire.Frame {
		return []wire.Frame{
			serverpackets.FrameTutorialCloseHTML(), serverpackets.FrameTutorialShowQuestionMark(1),
			serverpackets.FramePlaySound(script.SoundTutorial), voiceFrame(r.objID, r.at, "tutorial_voice_006"),
		}
	}}})
	if got := r.srv.QuestVars(t, r.objID, tutorial)["Ex"]; got != "-4" {
		t.Fatalf("Ex after TE1 = %q, want -4", got)
	}
}

// TestTutorialSecondClassLinks: the class transfer links TE26, TE23 and
// TE24 open the page of the first class the player plays and nothing for a
// class they hold no page for.
func TestTutorialSecondClassLinks(t *testing.T) {
	t.Parallel()
	r := bootLevelRig(t, 0)
	var asks []tutorialAsk
	for _, c := range secondClass {
		for _, l := range []struct {
			link int
			page string
		}{{26, c.a}, {23, c.b}, {24, c.c}} {
			want := nothing
			if l.page != "" {
				want = page(t, l.page)
			}
			asks = append(asks, tutorialAsk{fmt.Sprintf("TE%d class %d", l.link, c.id), c.id, linkReq(l.link), want})
		}
	}
	for _, class := range []int{0, 10, 44, 53} {
		for _, link := range []int{26, 23, 24} {
			asks = append(asks, tutorialAsk{fmt.Sprintf("TE%d class %d", link, class), class, linkReq(link), nothing})
		}
	}
	r.ask(asks)
}

// TestTutorialQuestionMarks: every question mark opens the page of the
// player's class, with the radar mark and the memo it sets; a class with
// no tutorial answers nothing where the page depends on it.
func TestTutorialQuestionMarks(t *testing.T) {
	t.Parallel()
	r := bootLevelRig(t, 0)
	var asks []tutorialAsk
	for _, c := range startClasses {
		name := func(mark int32) string { return fmt.Sprintf("QM%d class %d", mark, c.id) }
		asks = append(asks,
			tutorialAsk{name(2), c.id, markReq(2), page(t, c.qm2)},
			tutorialAsk{name(5), c.id, markReq(5), markedPage(t, c.guideMark, c.qm5Page)},
			tutorialAsk{name(9), c.id, markReq(9), markedPage(t, c.qm9Mark, c.qm9)},
			tutorialAsk{name(13), c.id, markReq(13), page(t, c.qm13)},
			tutorialAsk{name(24), c.id, markReq(24), page(t, c.qm24)},
			tutorialAsk{name(25), c.id, markReq(25), page(t, c.qm25)},
		)
	}
	// A class the tutorial does not start from has no page for these.
	for _, mark := range []int32{2, 5, 9, 13, 24, 25} {
		asks = append(asks, tutorialAsk{fmt.Sprintf("QM%d class 4", mark), 4, markReq(mark), nothing})
	}
	for _, p := range []struct {
		mark int32
		file string
	}{
		{4, "tutorial_10.htm"},
		{8, "tutorial_18.htm"},
		{10, "tutorial_19.htm"},
		{15, "tutorial_28.htm"},
		{16, "tutorial_30.htm"},
		{17, "tutorial_27.htm"},
		{19, "tutorial_07.htm"},
		{22, "tutorial_14.htm"},
		{23, "tutorial_24.htm"},
	} {
		asks = append(asks, tutorialAsk{fmt.Sprintf("QM%d", p.mark), 0, markReq(p.mark), page(t, p.file)})
	}
	asks = append(asks,
		tutorialAsk{"QM3", 0, markReq(3), eventPage(t, 1048576, "tutorial_09.htm")},
		tutorialAsk{"QM6", 0, markReq(6), nothing},
		tutorialAsk{"QM99", 0, markReq(99), nothing})
	r.ask(asks)
}

// TestTutorialQuestionMarkMemos: question marks 7 and 12 open the same
// page and set the memo that tells the next enter world which step is
// next.
func TestTutorialQuestionMarkMemos(t *testing.T) {
	t.Parallel()
	r := bootLevelRig(t, 0)
	for _, tc := range []struct {
		mark int32
		memo string
	}{{7, "5"}, {12, "4"}} {
		r.ask([]tutorialAsk{{fmt.Sprintf("QM%d", tc.mark), 0, markReq(tc.mark), page(t, "tutorial_15.htm")}})
		if got := r.srv.QuestVars(t, r.objID, tutorial)["ucMemo"]; got != tc.memo {
			t.Fatalf("ucMemo after QM%d = %q, want %s", tc.mark, got, tc.memo)
		}
	}
}
