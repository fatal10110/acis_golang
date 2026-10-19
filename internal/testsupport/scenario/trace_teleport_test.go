package scenario

import (
	"encoding/hex"
	"slices"
	"strings"
	"testing"
)

// A step compares the player's own teleport, where it lands, and leaves
// another creature's to the world.
func TestRenderComparesThePlayersTeleportOnly(t *testing.T) {
	frame, err := hex.DecodeString("288404001049b7ffff0aa7010004f6ffff00000000")
	if err != nil {
		t.Fatal(err)
	}
	line, ok, err := render(frame, map[int32]string{0x10000484: "player"})
	if err != nil || !ok || line != "TeleportToLocation obj=player loc=-18615,108298,-2556 fast=0" {
		t.Fatalf("the player's teleport renders %q, %v, %v", line, ok, err)
	}
	for name, roles := range map[string]map[int32]string{
		"an NPC's":       {0x10000484: "gatekeeper"},
		"an unknown's":   {},
		"another role's": {0x10000485: "player"},
	} {
		if line, ok, err := render(frame, roles); ok || err != nil {
			t.Fatalf("%s teleport renders %q, %v, %v; want not compared", name, line, ok, err)
		}
	}
}

// The trace's teleport of the player, with the ActionFailed lines its
// aborts sent right before it, goes after the rest of its task's packets;
// a later task's packets stay after it.
func TestDeferTeleportsMovesThePlayersTeleportToTheEndOfItsTask(t *testing.T) {
	const tp = "TeleportToLocation obj=player loc=1,2,3 fast=0"
	for _, tc := range []struct {
		name       string
		task, want []string
	}{
		{
			"talk",
			[]string{"SystemMessage id=302", "ActionFailed", "ActionFailed", tp, "ActionFailed"},
			[]string{"SystemMessage id=302", "ActionFailed", "ActionFailed", "ActionFailed", tp},
		},
		{
			"page after",
			[]string{"ActionFailed", tp, "NpcHtmlMessage obj=npc", "ActionFailed"},
			[]string{"NpcHtmlMessage obj=npc", "ActionFailed", "ActionFailed", tp},
		},
		{"last already", []string{"ActionFailed", tp}, []string{"ActionFailed", tp}},
		{
			"an NPC's",
			[]string{"TeleportToLocation obj=npc loc=1,2,3 fast=0", "ActionFailed"},
			[]string{"TeleportToLocation obj=npc loc=1,2,3 fast=0", "ActionFailed"},
		},
		{"none", []string{"ActionFailed"}, []string{"ActionFailed"}},
	} {
		if got := deferTeleports(slices.Clone(tc.task)); !slices.Equal(got, tc.want) {
			t.Errorf("%s: deferTeleports = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// teleportTrace is a scenario trace of a talk that teleports the player
// (object 500): the teleport's abort, the teleport, then the talk's own
// ActionFailed; the next tick sends one more.
const teleportTrace = `# test trace
# player object 500; format: README.md in this directory.
C RequestBypassToServer Quest X 1
S ActionFailed 25
S TeleportToLocation 28f401000001000000020000000300000000000000
S ActionFailed 25
T 500
S ActionFailed 25
`

// The trace replays with the teleport after the rest of the talk's
// packets, where the Go run sends it, and before the next tick's.
func TestCheckTraceDefersThePlayersTeleport(t *testing.T) {
	const tp = "  S TeleportToLocation obj=player loc=1,2,3 fast=0\n"
	sc := traceScenario(t, "bypass Quest X 1\n  S ActionFailed\n  S ActionFailed\n"+tp+"  S ActionFailed\n")
	if _, err := checkTrace(sc, []byte(teleportTrace), nil); err != nil {
		t.Fatal(err)
	}
	for name, steps := range map[string]string{
		"the reference's order": "bypass Quest X 1\n  S ActionFailed\n" + tp + "  S ActionFailed\n  S ActionFailed\n",
		"after the next tick's": "bypass Quest X 1\n  S ActionFailed\n  S ActionFailed\n  S ActionFailed\n" + tp,
	} {
		_, err := checkTrace(traceScenario(t, steps), []byte(teleportTrace), nil)
		if err == nil || !strings.Contains(err.Error(), "scenario packet") {
			t.Errorf("%s: checkTrace error = %v, want a packet difference", name, err)
		}
	}
}
