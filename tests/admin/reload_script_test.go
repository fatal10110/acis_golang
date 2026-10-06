package admin

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// judgeID is a civilian NPC one script holds the first talk of.
const judgeID = 30981

// bootJudgeReload boots a game master beside the judge, whose first-talk
// script answers with its page, with a //reload npc that swaps the NPC
// templates for renamed copies.
func bootJudgeReload(t *testing.T) (*gameservertest.Server, *npc.Folk) {
	t.Helper()
	judge := func(name string) *npc.Table {
		tmpl := gameservertest.FolkTemplate("Folk", judgeID)
		tmpl.Name = name
		return npc.NewTable([]*npc.Template{tmpl})
	}
	templates := judge("Judge")
	var flip sync.Mutex
	renamed := false
	reloads := network.DataReloads{NPCs: func() error {
		flip.Lock()
		defer flip.Unlock()
		renamed = !renamed
		if renamed {
			templates.Replace(judge("Renamed Judge"))
		} else {
			templates.Replace(judge("Judge"))
		}
		return nil
	}}
	path := "script.feature.Judge"
	scripts := gameservertest.WithNPCScripts(map[int32]script.NPCKind{judgeID: script.KindFolk}, []script.Listing{{Path: path}}, script.Catalog{
		path: func() script.Script {
			return script.Script{Dir: "feature", Bind: script.Bindings{script.EventFirstTalk: {judgeID}}, Hooks: script.Hooks{
				OnFirstTalk: func(*script.Script, script.FirstTalk) string { return "<html><body>judged</body></html>" },
			}}
		},
	})
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithNPCs(templates), gameservertest.WithNpcSpawns(nil), gameservertest.WithDataReloads(reloads), scripts)
	enterWorld(t, srv.Client)
	f := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", judgeID), location.Location{X: spawnX + 20, Y: spawnY, Z: spawnZ})
	drain(t, srv.Client)
	return srv, f
}

// judged reports whether frames hold the judge script's page.
func judged(frames [][]byte) bool {
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeNpcHtmlMessage && strings.Contains(string(f), "j\x00u\x00d\x00g\x00e\x00d\x00") {
			return true
		}
	}
	return false
}

// talk selects the judge, unless it is c's target already, and talks to
// it.
func talk(t *testing.T, c *testsupport.ScriptedClient, f *npc.Folk, selected bool) [][]byte {
	t.Helper()
	if !selected {
		exchange(t, c, encodeAction(f.ObjectID()))
	}
	return exchange(t, c, encodeAction(f.ObjectID()))
}

// TestNPCReloadKeepsScriptBindings: an NPC bound to a script at boot
// answers through it before and after //reload npc. The registry is built
// once at boot and never rebuilt; the reload swaps the templates only.
func TestNPCReloadKeepsScriptBindings(t *testing.T) {
	t.Parallel()
	srv, judge := bootJudgeReload(t)
	gm := srv.Client
	if !judged(talk(t, gm, judge, false)) {
		t.Fatal("the judge did not answer through its script before the reload")
	}
	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload npc")), "NPCs templates have been reloaded; scripts were not reloaded.")
	if !judged(talk(t, gm, judge, true)) {
		t.Fatal("the judge did not answer through its script after the reload")
	}
}

// TestNPCReloadRacesFirstTalkSafely: one player talks to the judge while a
// game master reloads the NPC templates; under -race every talk answers
// through the script and nothing races.
func TestNPCReloadRacesFirstTalkSafely(t *testing.T) {
	t.Parallel()
	srv, judge := bootJudgeReload(t)
	talker, _ := addPlayer(t, srv, "talker", "Talker", 0)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range 10 {
			if !judged(talk(t, talker, judge, i > 0)) {
				t.Errorf("talk %d did not answer through the script", i)
				return
			}
		}
	}()
	// The talker's approach and the judge's animation reach the game
	// master too; only its messages count.
	want := []string{"NPCs templates have been reloaded; scripts were not reloaded."}
	for range 10 {
		if got := textsIn(t, exchange(t, srv.Client, encodeBuildCmd("reload npc"))); !slices.Equal(got, want) {
			t.Fatalf("//reload npc answered %q, want %q", got, want)
		}
	}
	wg.Wait()
}
