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
// aborts sent right before it, goes after the rest of its task's packets,
// as does a cast with the player as caster, with its skill's teleport; a
// later task's packets stay after them.
func TestDeferPlayerActionsMovesThePlayersTeleportAndCastToTheEndOfItsTask(t *testing.T) {
	const (
		tp      = "TeleportToLocation obj=player loc=1,2,3 fast=0"
		cast    = "MagicSkillUse caster=player target=player skill=2100-1 hit=0 reuse=0 success=0"
		npcCast = "MagicSkillUse caster=npc target=player skill=4339-1 hit=6000 reuse=1 success=0"
		say     = "NpcSay obj=npc type=0 npc=30048 text=a"
	)
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
		{
			"cast",
			[]string{cast, "NpcHtmlMessage obj=npc", "ActionFailed"},
			[]string{"NpcHtmlMessage obj=npc", "ActionFailed", cast},
		},
		{
			"cast with its teleport",
			[]string{cast, "ActionFailed", tp, say},
			[]string{say, cast, "ActionFailed", tp},
		},
		{
			"cast, then no teleport",
			[]string{cast, "ActionFailed", say},
			[]string{"ActionFailed", say, cast},
		},
		{
			"teleport, then cast",
			[]string{"ActionFailed", tp, say, cast, "SystemMessage id=302"},
			[]string{say, "SystemMessage id=302", "ActionFailed", tp, cast},
		},
		{"an NPC's cast", []string{npcCast, say}, []string{npcCast, say}},
	} {
		if got, err := deferPlayerActions(slices.Clone(tc.task), 0); err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("%s: deferPlayerActions = %q, %v, want %q", tc.name, got, err, tc.want)
		}
	}
}

// teleportTrace is a scenario trace of a talk that teleports the player
// (object 500): the teleport's abort, the teleport, then the talk's own
// ActionFailed; the next tick's hook teleports the player again, then
// sends a message.
const teleportTrace = `# test trace
# player object 500; format: README.md in this directory.
C RequestBypassToServer Quest X 1
S ActionFailed 25
S TeleportToLocation 28f401000001000000020000000300000000000000
S ActionFailed 25
T 500
S ActionFailed 25
S TeleportToLocation 28f401000001000000020000000300000000000000
S SystemMessage 642e01000001000000030000007b060000
`

// The talk, the client packet's own run on the player's queue, replays in
// the reference's order: its teleport runs inside the call. The tick's
// hook, which may run on another queue, replays with its teleport after the
// rest of its packets, where the Go run sends it.
func TestCheckTraceDefersThePlayersTeleportOffItsQueue(t *testing.T) {
	const (
		tp  = "  S TeleportToLocation obj=player loc=1,2,3 fast=0\n"
		af  = "  S ActionFailed\n"
		msg = "  S SystemMessage id=302 3:1659\n"
	)
	sc := traceScenario(t, "bypass Quest X 1\n"+af+tp+af+msg+af+tp)
	if _, err := checkTrace(sc, []byte(teleportTrace), nil); err != nil {
		t.Fatal(err)
	}
	for name, steps := range map[string]string{
		"the talk's teleport deferred":  af + af + tp + msg + af + tp,
		"the tick's in the reference's": af + tp + af + af + tp + msg,
	} {
		_, err := checkTrace(traceScenario(t, "bypass Quest X 1\n"+steps), []byte(teleportTrace), nil)
		if err == nil || !strings.Contains(err.Error(), "scenario packet") {
			t.Errorf("%s: checkTrace error = %v, want a packet difference", name, err)
		}
	}
}

// A task whose hook ran on the player's own queue keeps the reference's
// in-call order, and one of another queue defers its player's teleport and
// cast; without a queue line a client packet's own run is the player's
// queue and every other task another, and a queue line names the queue of
// each task that teleports or casts with the player.
func TestStepPacketsDefersOnlyTheTasksOfAnotherQueue(t *testing.T) {
	const (
		tp   = "TeleportToLocation obj=player loc=1,2,3 fast=0"
		cast = "MagicSkillUse caster=player target=player skill=2100-1 hit=0 reuse=0 success=0"
		msg  = "SystemMessage id=302"
	)
	tasks := func() []traceTask {
		return []traceTask{
			{sent: []string{"ActionFailed", tp, msg}, own: true},
			{sent: []string{msg}},
			{sent: []string{cast, msg}},
			{sent: []string{"ActionFailed", tp, msg}},
		}
	}
	for _, tc := range []struct {
		name   string
		queues []string
		want   []string
	}{
		{
			"default", nil,
			[]string{"ActionFailed", tp, msg, msg, msg, cast, msg, "ActionFailed", tp},
		},
		{
			"ticks on the player's queue",
			[]string{"own", "own", "own"},
			[]string{"ActionFailed", tp, msg, msg, cast, msg, "ActionFailed", tp, msg},
		},
		{
			"client run raising another queue's hook",
			[]string{"other", "own", "other"},
			[]string{msg, "ActionFailed", tp, msg, cast, msg, msg, "ActionFailed", tp},
		},
	} {
		got, err := stepPackets(tasks(), tc.queues)
		if err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("%s: stepPackets = %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
	for _, queues := range [][]string{{"own", "own"}, {"own", "own", "own", "own"}} {
		if _, err := stepPackets(tasks(), queues); err == nil || !strings.Contains(err.Error(), "queue line names") {
			t.Errorf("queues %q: stepPackets error = %v, want a task count difference", queues, err)
		}
	}
}

// A step's queue line puts the tick's hook on the player's own queue: its
// teleport replays in the reference's in-call order, and a queue line naming
// another number of tasks than the trace's fails.
func TestCheckTraceKeepsAnOwnQueueTicksTeleportInCall(t *testing.T) {
	const (
		tp  = "  S TeleportToLocation obj=player loc=1,2,3 fast=0\n"
		af  = "  S ActionFailed\n"
		msg = "  S SystemMessage id=302 3:1659\n"
	)
	sc := traceScenario(t, "bypass Quest X 1\n  queue own own\n"+af+tp+af+af+tp+msg)
	if _, err := checkTrace(sc, []byte(teleportTrace), nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, steps, want string }{
		{"deferred anyway", "  queue own own\n" + af + tp + af + msg + af + tp, "scenario packet"},
		{"one task named", "  queue own\n" + af + tp + af + af + tp + msg, "queue line names 1 tasks, the trace has 2"},
	} {
		_, err := checkTrace(traceScenario(t, "bypass Quest X 1\n"+tc.steps), []byte(teleportTrace), nil)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: checkTrace error = %v, want %q", tc.name, err, tc.want)
		}
	}
}

// A queue line names own or other per task, once per step.
func TestParseRejectsAMalformedQueueLine(t *testing.T) {
	for _, queue := range []string{"  queue\n", "  queue own mine\n", "  queue own\n  queue other\n"} {
		if _, err := Parse("q.scenario", []byte("character A 1\nbypass Quest X 1\n"+queue)); err == nil || !strings.Contains(err.Error(), "queue") {
			t.Errorf("%q: Parse error = %v, want a queue line error", queue, err)
		}
	}
}
