package effect

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// Reference: EffectProtectionBlessing.onStart returns false and its onExit
// calls Playable.stopProtectionBlessing (Playable.java:277-285). setInUse(true)
// marks the effect in use but leaves _startConditionsCorrect false
// (AbstractEffect.java:159-168), so EffectList skips its stat funcs and the
// felt message (EffectList.java:768-774) and scheduleEffect's FINISHING case
// skips onExit (AbstractEffect.java:319). Only setInUse(false), when a recast
// takes its stack group's head (EffectList.java:756-765), runs onExit.

// protectedSummon records Blessing of Protection stops into a shared event
// log, so they interleave with the list owner's events.
type protectedSummon struct {
	blessedSummon
	events *[]string
	exited []*Effect
}

func (s *protectedSummon) StopProtectionBlessing(e *Effect) {
	s.exited = append(s.exited, e)
	*s.events = append(*s.events, "stop-protection-blessing")
}

// newProtectionBlessing builds skill 5182's effect as the datapack declares
// it (5100-5199.xml: time 3600, stackType pk_protect, stackOrder 1).
func newProtectionBlessing(t *testing.T, target Actor) *Effect {
	t.Helper()
	e, err := New(
		Skill{ID: 5182, Level: 1, SkillType: "BUFF", StackType: "pk_protect"},
		modelskill.EffectTemplate{Name: "ProtectionBlessing", Time: 3600, Count: 1, StackType: "pk_protect", StackOrder: 1, Icon: true},
	)
	if err != nil {
		t.Fatalf("New(ProtectionBlessing): %v", err)
	}
	e.Effected = target
	return e
}

// TestProtectionBlessingEndingRunsNoExitHook ends a held Blessing of
// Protection the ways the reference finishes an effect: none of them runs its
// exit hook, so no appearance refresh is asked for.
func TestProtectionBlessingEndingRunsNoExitHook(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		end  func(*List, *Effect)
	}{
		{name: "remove", end: func(l *List, e *Effect) { l.Remove(e) }},
		{name: "stop by type", end: func(l *List, _ *Effect) { l.StopByType(TypeProtectionBless) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var events []string
			list := newTestList(eventOwner{events: &events})
			target := &protectedSummon{events: &events}
			e := newProtectionBlessing(t, target)

			list.Add(e)
			if !list.IsAffected(FlagProtectionBlessing) {
				t.Fatal("Blessing of Protection held but its flag is not set")
			}
			if !e.InUse() {
				t.Fatal("Blessing of Protection not in use after landing")
			}
			// No stat funcs and no felt message: the start never counted.
			requireEvents(t, events, nil)

			tt.end(list, e)
			if list.IsAffected(FlagProtectionBlessing) {
				t.Fatal("Blessing of Protection flag still set after it ended")
			}
			// The list still drops the (empty) stat funcs and reports the
			// early removal; no stop comes between them.
			requireEvents(t, events, []string{"owner:remove:ProtectionBlessing", "disappeared:5182:1"})
		})
	}
}

// TestProtectionBlessingRecastRunsExitHookOnce recasts Blessing of Protection
// onto its holder. The old effect's exit() skips onExit, but the newcomer
// taking the stack head calls setInUse(false) on it, which does: one stop,
// between the old effect's stat removal and its disappeared message. The
// newcomer's own start adds no stat funcs and sends no felt message.
func TestProtectionBlessingRecastRunsExitHookOnce(t *testing.T) {
	t.Parallel()
	var events []string
	list := newTestList(eventOwner{events: &events})
	target := &protectedSummon{events: &events}
	old := newProtectionBlessing(t, target)
	recast := newProtectionBlessing(t, target)

	list.Add(old)
	list.Add(recast)

	requireEvents(t, events, []string{
		"owner:remove:ProtectionBlessing",
		"stop-protection-blessing",
		"disappeared:5182:1",
	})
	if len(target.exited) != 1 || target.exited[0] != old {
		t.Fatalf("exit hook ran for %v, want only the replaced effect", target.exited)
	}
	if held := list.All(); len(held) != 1 || held[0] != recast {
		t.Fatalf("held effects = %v, want only the recast", held)
	}
	if !list.IsAffected(FlagProtectionBlessing) {
		t.Fatal("recast Blessing of Protection flag not set")
	}
}

// TestProtectionBlessingExitReachesPlayer pins the exit hook's player branch:
// the replaced blessing is handed to the player's own stop.
func TestProtectionBlessingExitReachesPlayer(t *testing.T) {
	t.Parallel()
	target := &liveEffectTarget{isPlayer: true}
	e := newProtectionBlessing(t, target)
	e.OnExit(e)
	requireEvents(t, target.events, []string{"stop-protection-bless"})
}
