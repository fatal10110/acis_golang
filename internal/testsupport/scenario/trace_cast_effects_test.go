package scenario

import (
	"slices"
	"strings"
	"testing"
)

// A queue line's other+n moves the n lines right after each cast with the
// player as caster along with it: the cast's effect packets, which the Go run
// sends after the rest of the hook's task, where the trace has them in the
// call.
func TestDeferPlayerActionsMovesACastsCountedEffects(t *testing.T) {
	const (
		cast = "MagicSkillUse caster=player target=player skill=2100-1 hit=0 reuse=0 success=0"
		sys  = "SystemMessage id=1606"
		say  = "NpcSay obj=npc type=0 npc=30048 text=a"
		tp   = "TeleportToLocation obj=player loc=1,2,3 fast=0"
	)
	for _, tc := range []struct {
		name    string
		task    []string
		effects int
		want    []string
	}{
		{"one message", []string{cast, sys, say}, 1, []string{say, cast, sys}},
		{"the message stays the hook's without a count", []string{cast, sys, say}, 0, []string{sys, say, cast}},
		{"the count replaces the abort and teleport rule", []string{cast, sys, "ActionFailed", tp, say}, 3, []string{say, cast, sys, "ActionFailed", tp}},
		{"a message past the count stays", []string{cast, sys, say, sys}, 1, []string{say, sys, cast, sys}},
		{"each cast takes its own", []string{cast, sys, say, cast, sys}, 1, []string{say, cast, sys, cast, sys}},
		{"the last line of the task", []string{say, cast, sys}, 1, []string{say, cast, sys}},
	} {
		got, err := deferPlayerActions(slices.Clone(tc.task), tc.effects)
		if err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("%s: deferPlayerActions = %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
	if _, err := deferPlayerActions([]string{say, cast, sys}, 2); err == nil || !strings.Contains(err.Error(), "moves 2 lines") {
		t.Errorf("a count past the task's end: error = %v", err)
	}
}

// other+n in a step's queue line reaches the tasks it names, and own tasks
// keep the reference's in-call order whatever the count.
func TestStepPacketsMovesACastsEffectsOfAnotherQueuesTask(t *testing.T) {
	const (
		cast = "MagicSkillUse caster=player target=player skill=2100-1 hit=0 reuse=0 success=0"
		sys  = "SystemMessage id=1606"
		msg  = "SystemMessage id=302"
	)
	tasks := []traceTask{{sent: []string{cast, sys, msg}}, {sent: []string{cast, sys, msg}}}
	got, err := stepPackets(tasks, []string{"other+1", "own"})
	want := []string{msg, cast, sys, cast, sys, msg}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("stepPackets = %q, %v; want %q", got, err, want)
	}
}

func TestParseReadsAQueueLineWithCastEffects(t *testing.T) {
	sc, err := Parse("q.scenario", []byte("character A 1\nbypass Quest X 1\n  queue own other+2 other\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := sc.Steps[0].Queues; !slices.Equal(got, []string{"own", "other+2", "other"}) {
		t.Fatalf("queues = %q", got)
	}
	for _, queue := range []string{"other+", "other+0", "other+x", "other+02", "own+1", "other2", "other+-1"} {
		if _, err := Parse("q.scenario", []byte("character A 1\nbypass Quest X 1\n  queue "+queue+"\n")); err == nil || !strings.Contains(err.Error(), "queue") {
			t.Errorf("queue %q: Parse error = %v, want a queue line error", queue, err)
		}
	}
}
