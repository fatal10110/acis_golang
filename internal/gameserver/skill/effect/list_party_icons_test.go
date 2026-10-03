package effect

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestPartyIconEntriesReportNextActionNotCountdown pins
// AbstractEffect.addPartySpelledIcon (AbstractEffect.java:355-365) against
// addIcon (:340-353): the party's icon list has no repeat-count branch, so
// a count=7/time=2 effect reports the 2 s left until its next tick, where
// the owner's own list counts down the whole 14 s. A permanent effect
// reports -1 in both.
func TestPartyIconEntriesReportNextActionNotCountdown(t *testing.T) {
	list := newTestList(nil)
	if list.HasHeld() {
		t.Fatal("HasHeld() = true on a new list")
	}
	repeat := &Effect{Skill: Skill{ID: 21, Level: 1}, Template: modelskill.EffectTemplate{Name: "dot", Time: 2, Count: 7, Icon: true}}
	repeat.OnStart = func(*Effect) bool { return true }
	permanent := &Effect{Skill: Skill{ID: 22, Level: 1}, Template: modelskill.EffectTemplate{Name: "buff", Time: -1, Icon: true}}
	permanent.OnStart = func(*Effect) bool { return true }
	list.Add(repeat)
	list.Add(permanent)

	now := list.now()
	own, party := list.IconEntries(now), list.PartyIconEntries(now)
	if len(own) != 2 || len(party) != 2 {
		t.Fatalf("IconEntries = %+v, PartyIconEntries = %+v, want two each", own, party)
	}
	if own[0].ID != 21 || own[0].Duration != 14_000 {
		t.Fatalf("own repeat entry = %+v, want id 21, 14000 ms", own[0])
	}
	if party[0].ID != 21 || party[0].Duration != 2_000 {
		t.Fatalf("party repeat entry = %+v, want id 21, 2000 ms", party[0])
	}
	if own[1].Duration != -1 || party[1].ID != 22 || party[1].Duration != -1 {
		t.Fatalf("permanent entries = %+v / %+v, want -1 in both", own[1], party[1])
	}

	list.Remove(repeat)
	list.Remove(permanent)
	if !list.HasHeld() {
		t.Fatal("HasHeld() = false after the list held effects")
	}
}
