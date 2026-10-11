package quest

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	featuretutorial "github.com/fatal10110/acis_golang/internal/gameserver/script/feature/tutorial"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// tutorialGuideID is the tutorial guide the first tutorial step hands a
// player who lacks one.
const tutorialGuideID = 5588

// tutorialPages are the datapack's tutorial window pages, keyed as the page
// cache looks them up.
func tutorialPages(t *testing.T) map[string]string {
	t.Helper()
	dir := datapack.Path(t, "data", "html", "script", "feature", "Tutorial")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	pages := map[string]string{}
	for _, e := range entries {
		data, err := os.ReadFile(dir + "/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		pages["script/feature/Tutorial/"+e.Name()] = string(data)
	}
	return pages
}

// tutorialPageFrame is the TutorialShowHtml of the datapack page file, as
// the page cache holds it: line ends read as '\n', the last line ended.
func tutorialPageFrame(t *testing.T, file string) wire.Frame {
	t.Helper()
	data, err := os.ReadFile(datapack.Path(t, "data", "html", "script", "feature", "Tutorial", file))
	if err != nil {
		t.Fatal(err)
	}
	page := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
	if !strings.HasSuffix(page, "\n") {
		page += "\n"
	}
	return serverpackets.FrameTutorialShowHTML(page)
}

// bootNewbie boots with no character, the tutorial registered from its
// scripts.xml path unless noTutorial, the datapack's tutorial pages and the
// tutorial guide among the items; the starting class grants the guide when
// withGuide.
func bootNewbie(t *testing.T, withGuide, noTutorial bool, extra ...gameservertest.Option) *gameservertest.Server {
	t.Helper()
	items := append(gameservertest.ItemTemplates().All(), &item.Template{
		ID: tutorialGuideID, Name: "Tutorial Guide", Kind: item.KindEtcItem, Duration: -1,
		Destroyable: true, EtcItem: &item.EtcItemDetail{},
	})
	class := gameservertest.ClassTemplate()
	if withGuide {
		class.Items = []player.StarterItem{{ItemID: tutorialGuideID, Count: 1}}
	}
	opts := []gameservertest.Option{
		gameservertest.WithWantChars(0),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithClassTemplate(class),
		gameservertest.WithItemTemplates(item.NewTable(items)),
		gameservertest.WithHTMLPages(tutorialPages(t)),
	}
	if !noTutorial {
		path := "script.feature.Tutorial"
		opts = append(opts, gameservertest.WithScripts([]script.Listing{{Path: path}}, script.Catalog{path: featuretutorial.New}))
	}
	return gameservertest.Boot(t, append(opts, extra...)...)
}

// createNewbie creates the human fighter Newbie through the client and
// returns its object id, read from its characters row.
func createNewbie(t *testing.T, srv *gameservertest.Server) int32 {
	t.Helper()
	c := srv.Client
	c.Send(encodeRequestCharacterCreate("Newbie"))
	readUntil(t, c, serverpackets.OpcodeCharCreateOk)
	readUntil(t, c, serverpackets.OpcodeCharSelectInfo)
	var objID int32
	if err := srv.DB.QueryRow("SELECT obj_Id FROM characters WHERE char_name='Newbie'").Scan(&objID); err != nil {
		t.Fatal(err)
	}
	return objID
}

// enterBurst selects the first character and enters the world, returning
// the EnterWorld burst.
func enterBurst(t *testing.T, srv *gameservertest.Server) [][]byte {
	t.Helper()
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	readUntil(t, c, serverpackets.OpcodeCharSelected)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	return srv.ReadQueued(t, c)
}

// tutorialFrames keeps the tutorial window, sound and radar packets of
// frames, in order.
func tutorialFrames(frames [][]byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeTutorialShowHTML, serverpackets.OpcodeTutorialShowQuestionMark,
			serverpackets.OpcodeTutorialEnableClientEvent, serverpackets.OpcodeTutorialCloseHTML,
			serverpackets.OpcodeRadarControl, serverpackets.OpcodePlaySound:
			out = append(out, f)
		}
	}
	return out
}

// voiceFrame is the tutorial voice file played to the player objID at at.
func voiceFrame(objID int32, at location.Location, file string) wire.Frame {
	return serverpackets.FramePlaySoundAt(serverpackets.Sound{Type: 2, File: file, BindToObject: true, ObjectID: objID, Location: at})
}

// burstTail requires the burst to end with want, its closing ActionFailed
// last.
func burstTail(t *testing.T, burst [][]byte, want ...wire.Frame) {
	t.Helper()
	want = append(want, serverpackets.FrameActionFailed())
	if len(burst) < len(want) {
		t.Fatalf("burst has %d frames, want at least %d", len(burst), len(want))
	}
	expectFrames(t, "burst tail", burst[len(burst)-len(want):], want...)
}

// TestNewCharacterTutorialFirstSteps creates a human fighter, which starts
// with the tutorial state STARTED, and follows its first tutorial steps.
// The first enter world arms the 10 s step and enables no client event;
// the step plays the class's first voice and opens its first page, the
// guide being held already; 30 s later the second voice plays. A relog
// keeps the rows and shows the question mark with the tutorial sound and
// voice; clicking it marks the newbie guide on the radar, plays its voice
// and opens its page.
func TestNewCharacterTutorialFirstSteps(t *testing.T) {
	t.Parallel()
	srv := bootNewbie(t, true, false)
	objID := createNewbie(t, srv)
	srv.FlushPersistence(t)
	if got, want := readJournal(t, srv.DB), []journalRow{{objID, tutorial, "<state>", val("STARTED")}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rows after the creation = %v, want %v", got, want)
	}

	burstTail(t, enterBurst(t, srv), serverpackets.FrameTutorialEnableClientEvent(0))
	if got, want := srv.QuestVars(t, objID, tutorial), map[string]string{"<state>": "STARTED", "Ex": "-2", "ucMemo": "1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tutorial after the first enter = %v, want %v", got, want)
	}
	var at location.Location
	onCharacter(t, srv, objID, func(c *player.Character) { at = c.CurrentLocation() })

	srv.Advance(t, 9*time.Second)
	if got := tutorialFrames(srv.ReadQueued(t, srv.Client)); len(got) != 0 {
		t.Fatalf("before 10 s sent %x, want nothing", got)
	}
	srv.Advance(t, time.Second)
	expectFrames(t, "first step", srv.ReadQueued(t, srv.Client),
		voiceFrame(objID, at, "tutorial_voice_001a"), tutorialPageFrame(t, "tutorial_human_fighter001.htm"))
	if n := srv.PlayerItemCount(t, objID, tutorialGuideID); n != 1 {
		t.Fatalf("guides = %d, want the one the class grants", n)
	}
	srv.Advance(t, 30*time.Second)
	expectFrames(t, "second step", srv.ReadQueued(t, srv.Client), voiceFrame(objID, at, "tutorial_voice_002"))

	restart(t, srv)
	srv.FlushPersistence(t)
	want := []journalRow{
		{objID, tutorial, "<state>", val("STARTED")},
		{objID, tutorial, "Ex", val("0")},
		{objID, tutorial, "ucMemo", val("1")},
	}
	if got := readJournal(t, srv.DB); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows at the relog = %v, want %v", got, want)
	}
	burstTail(t, enterBurst(t, srv),
		serverpackets.FrameTutorialShowQuestionMark(1),
		serverpackets.FramePlaySound(script.SoundTutorial),
		voiceFrame(objID, at, "tutorial_voice_006"),
		serverpackets.FrameTutorialEnableClientEvent(0))

	srv.Client.Send(encodeTutorialNumber(clientpackets.OpcodeRequestTutorialQuestionMark, 1))
	guide := serverpackets.Radar{X: -71424, Y: 258336, Z: -3109}
	add, show := guide, guide
	add.Show, add.Type = 2, 2
	show.Show, show.Type = 0, 1
	expectFrames(t, "question mark 1", srv.ReadQueued(t, srv.Client),
		serverpackets.FrameRadarControl(add), serverpackets.FrameRadarControl(show),
		voiceFrame(objID, at, "tutorial_voice_007"), tutorialPageFrame(t, "tutorial_human_fighter007.htm"))
	if got, want := srv.QuestVars(t, objID, tutorial), map[string]string{"<state>": "STARTED", "Ex": "-5", "ucMemo": "2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tutorial after question mark 1 = %v, want %v", got, want)
	}
}

// TestTutorialFirstStepGivesAMissingGuide: a new character whose class
// grants no guide gets one from the first tutorial step, ahead of the
// step's voice and page.
func TestTutorialFirstStepGivesAMissingGuide(t *testing.T) {
	t.Parallel()
	srv := bootNewbie(t, false, false)
	objID := createNewbie(t, srv)
	enterBurst(t, srv)
	var at location.Location
	onCharacter(t, srv, objID, func(c *player.Character) { at = c.CurrentLocation() })
	srv.Advance(t, 10*time.Second)
	got := tutorialFrames(srv.ReadQueued(t, srv.Client))
	expectFrames(t, "first step", got, voiceFrame(objID, at, "tutorial_voice_001a"), tutorialPageFrame(t, "tutorial_human_fighter001.htm"))
	if n := srv.PlayerItemCount(t, objID, tutorialGuideID); n != 1 {
		t.Fatalf("guides = %d, want 1", n)
	}
}

// TestTutorialLowHPThenSitting: below 30% HP a newbie is shown question
// mark 10 and the sit event is enabled; sitting down then opens the
// sitting page, once.
func TestTutorialLowHPThenSitting(t *testing.T) {
	t.Parallel()
	srv := bootNewbie(t, true, false)
	objID := createNewbie(t, srv)
	enterBurst(t, srv)
	var at location.Location
	onCharacter(t, srv, objID, func(c *player.Character) {
		at = c.CurrentLocation()
		c.SetHP(c.MaxHPValue() * 0.2)
	})
	expectFrames(t, "low HP", tutorialFrames(srv.ReadQueued(t, srv.Client)),
		serverpackets.FrameTutorialShowQuestionMark(10),
		serverpackets.FrameTutorialEnableClientEvent(8388608),
		serverpackets.FramePlaySound(script.SoundTutorial),
		voiceFrame(objID, at, "tutorial_voice_017"))

	srv.Client.Send(encodeTutorialSit())
	expectFrames(t, "sitting", tutorialFrames(srv.ReadQueued(t, srv.Client)),
		serverpackets.FrameTutorialEnableClientEvent(0),
		serverpackets.FramePlaySound(script.SoundTutorial),
		voiceFrame(objID, at, "tutorial_voice_018"),
		tutorialPageFrame(t, "tutorial_21z.htm"))
	if got := srv.QuestVars(t, objID, tutorial); got["HP"] != "1" || got["sit"] != "1" {
		t.Fatalf("tutorial after sitting = %v, want HP 1 and sit 1", got)
	}
}

// TestCreationWithoutTutorialWritesNoState: with no tutorial registered a
// new character gets no tutorial state.
func TestCreationWithoutTutorialWritesNoState(t *testing.T) {
	t.Parallel()
	srv := bootNewbie(t, true, true)
	createNewbie(t, srv)
	srv.FlushPersistence(t)
	if got := readJournal(t, srv.DB); len(got) != 0 {
		t.Fatalf("rows after the creation = %v, want none", got)
	}
	burst := enterBurst(t, srv)
	if got := tutorialFrames(burst); len(got) != 0 {
		t.Fatalf("burst tutorial frames = %x, want none", got)
	}
}

// TestTutorialLevelAndClassPages: a human fighter reaching level 5 is
// shown question mark 9 with the fighter's voice; the mystic page of
// question mark 11 is the human one and the newbie page of question mark
// 26 the fighter's.
func TestTutorialLevelAndClassPages(t *testing.T) {
	t.Parallel()
	levels := flatLevels(t)
	srv := bootNewbie(t, true, false, gameservertest.WithLevels(levels))
	objID := createNewbie(t, srv)
	enterBurst(t, srv)
	var at location.Location
	gain := func() {
		onCharacter(t, srv, objID, func(c *player.Character) {
			at = c.CurrentLocation()
			c.AddExpAndSp(levels.RequiredExpForLevel(5)-c.Exp, 0)
		})
	}
	gain()
	expectFrames(t, "level 5", tutorialFrames(srv.ReadQueued(t, srv.Client)),
		serverpackets.FrameTutorialShowQuestionMark(9),
		serverpackets.FramePlaySound(script.SoundTutorial),
		voiceFrame(objID, at, "tutorial_voice_014"))
	if got := srv.QuestVars(t, objID, tutorial)["lvl"]; got != "5" {
		t.Fatalf("lvl = %q, want 5", got)
	}

	for _, tc := range []struct {
		mark int32
		page string
	}{{11, "tutorial_mage020.htm"}, {26, "tutorial_newbie004b.htm"}} {
		srv.Client.Send(encodeTutorialNumber(clientpackets.OpcodeRequestTutorialQuestionMark, tc.mark))
		expectFrames(t, tc.page, srv.ReadQueued(t, srv.Client), tutorialPageFrame(t, tc.page))
	}
}
