package quest

import (
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPCs of the binding probes: each behavior and the talk probe hold
// their own.
const (
	probeNearID = 30006
	probeFarID  = 30007
)

// bindingProbes boots the talk probe, bound to talk through the far NPC
// only, and two behaviors of one shape, A on the near NPC and B on the far
// one; talked and ranB record their hooks running.
func bindingProbes(t *testing.T, talked, ranB *atomic.Bool) *dialogWorld {
	t.Helper()
	page := func(text string) string { return "<html><body>" + text + "</body></html>" }
	behavior := func(id int32, ran *atomic.Bool, text string) func() script.Script {
		return func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{id}, Bind: script.Bindings{script.EventTalked: {id}}, Hooks: script.Hooks{
				OnTalk: func(*script.Script, script.Talk) string { return page(text) },
				OnEvent: func(*script.Script, script.Event) string {
					if ran != nil {
						ran.Store(true)
					}
					return page(text)
				},
			}}
		}
	}
	catalog := script.Catalog{
		"script.feature.TalkProbe": func() script.Script {
			return script.Script{
				Bind: script.Bindings{script.EventTalked: {probeFarID}},
				Hooks: script.Hooks{OnTalk: func(*script.Script, script.Talk) string {
					talked.Store(true)
					return page("talk probe")
				}},
			}
		},
		"script.ai.ProbeA": behavior(probeNearID, nil, "probe a"),
		"script.ai.ProbeB": behavior(probeFarID, ranB, "probe b"),
	}
	w := bootSeamQuest(t, 20, nil, nil,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			gameservertest.FolkTemplate("Folk", probeNearID), gameservertest.FolkTemplate("Folk", probeFarID),
		})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{probeNearID: script.KindFolk, probeFarID: script.KindFolk},
			[]script.Listing{{Path: "script.feature.TalkProbe"}, {Path: "script.ai.ProbeA"}, {Path: "script.ai.ProbeB"}}, catalog),
	)
	w.spawnFolk(t, "near", probeNearID, 0, -40, 0)
	w.spawnFolk(t, "far", probeFarID, 0, 40, 0)
	return w
}

// showsText reports whether the rendered lines carry text.
func showsText(lines []string, text string) bool {
	return strings.Contains(strings.Join(lines, "\n"), text)
}

// TestQuestLinkOfAnUnboundScriptShowsNoQuestPage: "Quest <name>" on an NPC
// the named script does not talk through answers the no-quest page and
// runs no talk hook; on the NPC it is bound to, the hook answers (#5160).
func TestQuestLinkOfAnUnboundScriptShowsNoQuestPage(t *testing.T) {
	t.Parallel()
	var talked, ranB atomic.Bool
	w := bindingProbes(t, &talked, &ranB)
	near, far := w.npcs["near"], w.npcs["far"]

	w.interact(t, near)
	w.offerAll(t)
	got := w.bypass(t, "npc_"+strconv.Itoa(int(near))+"_Quest TalkProbe")
	if !showsText(got, "You are either not on a quest that involves this NPC") || showsText(got, "talk probe") {
		t.Fatalf("unbound link = %q, want the no-quest page", got)
	}
	if talked.Load() {
		t.Fatal("the unbound link ran the talk hook")
	}

	w.interact(t, far)
	w.offerAll(t)
	if got := w.bypass(t, "npc_"+strconv.Itoa(int(far))+"_Quest TalkProbe"); !showsText(got, "talk probe") {
		t.Fatalf("bound link = %q, want the probe's page", got)
	}
	if !talked.Load() {
		t.Fatal("the bound link ran no talk hook")
	}
}

// TestQuestEventRunsOnlyTheBoundBehavior: through an NPC holding behavior
// A, an event naming behavior B, of the same shape, runs nothing; one
// naming A runs A's hook (#5160).
func TestQuestEventRunsOnlyTheBoundBehavior(t *testing.T) {
	t.Parallel()
	var talked, ranB atomic.Bool
	w := bindingProbes(t, &talked, &ranB)

	w.interact(t, w.npcs["near"])
	w.offerAll(t)
	if got := w.bypass(t, "Quest ProbeB go"); len(got) != 0 {
		t.Fatalf("B's event through A's NPC = %q, want nothing", got)
	}
	if ranB.Load() {
		t.Fatal("B's event hook ran through A's NPC")
	}
	w.offerAll(t)
	if got := w.bypass(t, "Quest ProbeA go"); !showsText(got, "probe a") {
		t.Fatalf("A's event through A's NPC = %q, want A's page", got)
	}
}
