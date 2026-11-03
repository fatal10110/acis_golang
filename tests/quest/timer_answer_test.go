package quest

import (
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// The timer answer world: a real quest whose timer hook answers by the
// timer's name, and a civilian NPC to bind its timers to.
const (
	timerAnswerQuest = "Q960_TimerAnswer"
	timerAnswerNPCID = 30983
	timerAnswerChat  = "The tomb is open."
	timerAnswerFile  = "<html><body>file %objectId%</body></html>"
	timerAnswerHTML  = "<html><body>whole %objectId%</body></html>"
)

// timerAnswerWorld is a booted character in the world beside the NPC.
type timerAnswerWorld struct {
	srv    *gameservertest.Server
	player int32
	folk   *npc.Folk
	// fired counts the timer hook's runs.
	fired *atomic.Int32
}

func bootTimerAnswers(t *testing.T) *timerAnswerWorld {
	t.Helper()
	fired := &atomic.Int32{}
	catalog := script.Catalog{
		"quest." + timerAnswerQuest: func() script.Script {
			return script.Script{Title: "Timer Answer", QuestID: 960, Hooks: script.Hooks{
				OnTimer: func(_ *script.Script, e script.Timer) string {
					fired.Add(1)
					switch e.Name {
					case "file":
						return "page.htm"
					case "html":
						return timerAnswerHTML
					case "chat":
						return timerAnswerChat
					}
					return ""
				},
			}}
		},
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Timer", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithHTMLPages(map[string]string{"script/quest/" + timerAnswerQuest + "/page.htm": timerAnswerFile}),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{timerAnswerNPCID: script.KindFolk},
			[]script.Listing{{Path: "quest." + timerAnswerQuest}}, catalog),
	)
	objID := srv.SoleObjectID(t)
	enterWorld(t, srv)
	x, y, z := srv.PlayerPosition(t, objID)
	folk := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", timerAnswerNPCID), location.Location{X: x + 20, Y: y, Z: z})
	srv.ReadQueued(t, srv.Client)
	return &timerAnswerWorld{srv: srv, player: objID, folk: folk, fired: fired}
}

// fire starts the one-shot timer name bound to the NPC when withNPC and
// to the player when withPlayer, lets it fire and returns what the client
// was sent, rendered.
func (w *timerAnswerWorld) fire(t *testing.T, name string, withNPC, withPlayer bool) []string {
	t.Helper()
	before := w.fired.Load()
	w.srv.RunScript(t, w.player, timerAnswerQuest, func(s *script.Script, p *script.Player) {
		var n *script.NPC
		if withNPC {
			n = script.NPCOf(w.folk)
		}
		if !withPlayer {
			p = nil
		}
		if !s.StartTimer(name, n, p, 100*time.Millisecond) {
			t.Errorf("timer %s refused", name)
		}
	})
	w.srv.AdvanceUntil(t, "the timer "+name+" fires", func() bool { return w.fired.Load() > before })
	var out []string
	for _, f := range w.srv.ReadQueued(t, w.srv.Client) {
		if !slices.Contains(traceRenders, f[0]) {
			continue
		}
		line, err := scriptcontract.Packet(f, map[int32]string{w.folk.ObjectID(): "folk"})
		if err != nil {
			t.Fatalf("frame %x: %v", f, err)
		}
		out = append(out, line)
	}
	return out
}

// A timer bound to a player that answers a page file or a whole page opens
// it in a dialog window, then releases the client: through no NPC (object
// id 0, %objectId% left as is) or through the timer's NPC (its object id,
// filled in). A page file comes as the HTML cache loads it, with its line
// end.
func TestTimerAnswerShowsPage(t *testing.T) {
	t.Parallel()
	w := bootTimerAnswers(t)

	cases := []struct {
		name    string
		withNPC bool
		want    []string
	}{
		{"file", false, []string{`S NpcHtmlMessage obj=0 item=0 html="<html><body>file %objectId%</body></html>\n"`, "S ActionFailed"}},
		{"html", false, []string{`S NpcHtmlMessage obj=0 item=0 html="<html><body>whole %objectId%</body></html>"`, "S ActionFailed"}},
		{"file", true, []string{`S NpcHtmlMessage obj=folk item=0 html="<html><body>file {folk}</body></html>\n"`, "S ActionFailed"}},
		{"html", true, []string{`S NpcHtmlMessage obj=folk item=0 html="<html><body>whole {folk}</body></html>"`, "S ActionFailed"}},
	}
	for _, c := range cases {
		if got := w.fire(t, c.name, c.withNPC, true); !slices.Equal(got, c.want) {
			t.Fatalf("timer %s (npc %v):\n got %q\nwant %q", c.name, c.withNPC, got, c.want)
		}
	}
}

// A timer bound to a player that answers any other text sends it as a chat
// line; a timer bound to no player shows nothing, whatever it answers; an
// empty answer sends nothing.
func TestTimerAnswerShowsChatLine(t *testing.T) {
	t.Parallel()
	w := bootTimerAnswers(t)

	chat := fmt.Sprintf("S SystemMessage id=%d 0:%s", serverpackets.SystemMessageS1, scriptcontract.Quote(timerAnswerChat))
	if got := w.fire(t, "chat", false, true); !slices.Equal(got, []string{chat}) {
		t.Fatalf("chat answer: got %q, want %q", got, []string{chat})
	}
	if got := w.fire(t, "chat", true, true); !slices.Equal(got, []string{chat}) {
		t.Fatalf("chat answer through the NPC: got %q, want %q", got, []string{chat})
	}
	for _, name := range []string{"chat", "file"} {
		if got := w.fire(t, name, true, false); len(got) != 0 {
			t.Fatalf("timer %s bound to no player sent %q", name, got)
		}
		if got := w.fire(t, name, false, false); len(got) != 0 {
			t.Fatalf("timer %s bound to no one sent %q", name, got)
		}
	}
	if got := w.fire(t, "silent", false, true); len(got) != 0 {
		t.Fatalf("empty answer sent %q", got)
	}
}
