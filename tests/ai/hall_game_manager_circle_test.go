package ai

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/hall"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// circleManagerIDs are the hot springs game managers of circles 1 to 4.
var circleManagerIDs = []int32{35596, 35597, 35598, 35599}

// bootCircles boots the character beside the four circles' managers, a
// gourd, the event listener and the bomb caller, with the game manager and its four circles
// registered in scripts.xml order around one set of shared values, and the
// extra hostile scripts.
func bootCircles(t *testing.T, extra script.Catalog) *warriorWorld {
	t.Helper()
	watering := hall.NewWateringGame()
	paths := []string{
		"script.ai.individual.DefaultNpc",
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManager",
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManagerCircle1",
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManagerCircle2",
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManagerCircle3",
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManagerCircle4",
		"test.HallListener",
	}
	catalog := script.Catalog{
		paths[0]: defaultnpc.New,
		paths[1]: func() script.Script { return hall.GameManager(watering) },
		paths[2]: func() script.Script { return hall.Circle1(watering) },
		paths[3]: func() script.Script { return hall.Circle2(watering) },
		paths[4]: func() script.Script { return hall.Circle3(watering) },
		paths[5]: func() script.Script { return hall.Circle4(watering) },
		paths[6]: hallListener,
	}
	kinds := map[int32]script.NPCKind{gourdID: script.KindFolk, hallListenerID: script.KindHostile}
	templates := []*npc.Template{gameservertest.FolkTemplate("Folk", int(gourdID)), agitTemplate(hallListenerID)}
	for _, id := range circleManagerIDs {
		kinds[id] = script.KindFolk
		templates = append(templates, gameservertest.FolkTemplate("Folk", int(id)))
	}
	for path, ctor := range extra {
		catalog[path] = ctor
		paths = append(paths, path)
	}
	kinds[bombCallerID] = script.KindHostile
	templates = append(templates, agitTemplate(bombCallerID))
	list := make([]script.Listing, len(paths))
	for i, p := range paths {
		list[i] = script.Listing{Path: p}
	}
	pages := managerPages()
	pages[managerDir+"35597/watering_manager001.htm"] = "<html><body>circle 2 manager001</body></html>"
	bomb := modelskill.NewTable([]modelskill.Definition{{
		ID: npcUltraBomb, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "BUFF", CastRange: 600, HitTime: 500, StaticHitTime: true, StaticReuse: true,
	}})
	opts := []gameservertest.Option{
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithHTMLPages(pages),
		gameservertest.WithNpcSpawnSkills(bomb),
	}
	return bootHalls(t, 0, 0, location.Location{}, raidBoot{opts: opts}, templates...)
}

// spawnCircles spawns the four circles' managers, 100 east of the
// character, 30 apart, the first two nearest.
func (w *warriorWorld) spawnCircles(t *testing.T) []*npc.Folk {
	t.Helper()
	var managers []*npc.Folk
	for i, id := range circleManagerIDs {
		managers = append(managers, w.spawnFolk(t, id, 100, []int{0, 30, -30, 60}[i]))
	}
	return managers
}

// TestGameManagerCirclesReportTheirGourd ports the circles' created and
// see-creature hooks: at its first look, 10 s after it appears, each
// circle's manager reports the gourd it sees to the NPCs within 8000,
// circle 1 through script event 10002, circle 2 through 10003, circle 3
// through 10004 and circle 4 through 10005, with the gourd's object id;
// circles 2 to 4 do not also report through circle 1's event. A manager
// reports once: its look at 60 s reports nothing more. Every manager runs
// the word game (each circle sets it) and shouts at its first look, but
// the shared circle is the last one built, 4, so no manager draws a word.
func TestGameManagerCirclesReportTheirGourd(t *testing.T) {
	t.Parallel()
	w := bootCircles(t, nil)
	listener := w.spawn(t, hallListenerID, -100, 100)
	gourd := w.spawnFolk(t, gourdID, 150, 0)
	managers := w.spawnCircles(t)

	var frames [][]byte
	w.srv.AdvanceUntil(t, "the circles' first look", func() bool {
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
		return len(says(frames, listener.ObjectID())) >= 4
	})
	w.srv.Advance(t, time.Second)
	frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
	for i, m := range managers {
		if got := len(framesOf(frames, serverpackets.OpcodeNpcSay, m.ObjectID())); got != 1 {
			t.Fatalf("circle %d manager said %d lines at its first look, want its one shout", i+1, got)
		}
	}
	w.srv.Advance(t, 60*time.Second)
	frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)

	got := says(frames, listener.ObjectID())
	var want []string
	for _, ev := range []int32{10002, 10003, 10004, 10005} {
		want = append(want, fmt.Sprintf("event %d %d 0", ev, gourd.ObjectID()))
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("listener heard %q, want %q", got, want)
	}
	if got := screenTexts(frames); len(got) != 0 {
		t.Fatalf("screen messages %q, want no word drawn", got)
	}
}

// TestGameManagerCirclesShareTheLastCircle ports the circles' shared
// circle and talk bindings: script event 4 with 50000, the fourth
// circle's bomb call, makes every circle's manager cast its bomb on itself,
// as each reads the circle the last one built set. The first and second
// circles' managers answer a talk with their own first pages.
func TestGameManagerCirclesShareTheLastCircle(t *testing.T) {
	t.Parallel()
	w := bootCircles(t, script.Catalog{"test.BombCaller": hallCaller(bombCallerID, 4, 50000, 0)})
	managers := w.spawnCircles(t)

	w.managerTalk(t, managers[0]) // selects it
	wantPage(t, "circle 1 talk", w.managerTalk(t, managers[0]), "manager001")
	w.managerTalk(t, managers[1]) // selects it
	wantPage(t, "circle 2 talk", w.managerTalk(t, managers[1]), "circle 2 manager001")

	_, frames := w.spawnSeen(t, bombCallerID, location.Location{X: w.at.X - 100, Y: w.at.Y + 100, Z: w.at.Z})
	w.srv.AdvanceUntil(t, "every manager's bomb", func() bool {
		frames = append(frames, cycle(t, w.srv)...)
		for _, m := range managers {
			if len(folkCasts(frames, m.ObjectID(), npcUltraBomb)) == 0 {
				return false
			}
		}
		return true
	})
	for i, m := range managers {
		if got := folkCasts(frames, m.ObjectID(), npcUltraBomb); !slices.Equal(got, []int32{m.ObjectID()}) {
			t.Fatalf("circle %d manager bomb casts = %v, want one on itself", i+1, got)
		}
	}
}
