package effect

import (
	"strconv"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// Reference: EffectCharmOfLuck.onExit / EffectPhoenixBless.onExit call
// Playable.stopCharmOfLuck / stopPhoenixBlessing(this) (Playable.java:
// 256-300): removeEffect(effect), then updateAbnormalEffect(). removeEffect
// only queues the removal (Creature.removeEffect -> EffectList.queueEffect);
// when onExit runs from addEffectFromQueue's setInUse(false) on a displaced
// stack head (EffectList.java:756-765), queueRunner drains that removal after
// the insertion, before its single updateEffectIcons (EffectList.java:
// 467-490). With EFFECT_CANCELING on, the displaced head already left the
// queue and the buff list (EffectList.java:728-734), so the removal finds
// nothing (EffectList.java:541-585).

// droppingSummon is a summon holding list that records its blessing stops
// into the list owner's event log.
type droppingSummon struct {
	blessedSummon
	list   *List
	events *[]string
}

func (s *droppingSummon) EffectList() *List { return s.list }

func (s *droppingSummon) StopCharmOfLuck(*Effect) {
	*s.events = append(*s.events, "stop:CharmOfLuck")
}

func (s *droppingSummon) StopPhoenixBlessing(*Effect) {
	*s.events = append(*s.events, "stop:PhoenixBless")
}

func newBlessing(t *testing.T, target Actor, name string, id modelskill.ID, stackType string, order float64) *Effect {
	t.Helper()
	e, err := New(
		Skill{ID: id, Level: 1, SkillType: "BUFF", StackType: stackType},
		modelskill.EffectTemplate{Name: name, Time: 3600, Count: 1, StackType: stackType, StackOrder: order, Icon: true},
	)
	if err != nil {
		t.Fatalf("New(%s): %v", name, err)
	}
	e.Effector, e.Effected = target, target
	return e
}

// TestDisplacedBlessingDropsItself lands a stronger blessing of the same
// stack type on a held one (Charm of Luck 1325 order 6 then 2168 order 8,
// both reduce_drop_penalty; Phoenix Blessing 438 then 1410, both
// resurrection_special). With lesser effects kept the displaced blessing's
// exit hook removes it after the newcomer's start: it is announced
// disappeared a second time and leaves the list before the one icon
// refresh. With them cancelled (the default) it is already gone.
func TestDisplacedBlessingDropsItself(t *testing.T) {
	t.Parallel()
	blessings := []struct {
		name, stack  string
		weak, strong modelskill.ID
	}{
		{name: "CharmOfLuck", stack: "reduce_drop_penalty", weak: 1325, strong: 2168},
		{name: "PhoenixBless", stack: "resurrection_special", weak: 438, strong: 1410},
	}
	for _, b := range blessings {
		for _, tc := range []struct {
			name string
			opts []Option
			tail []string
		}{
			{name: "cancel lesser", tail: []string{"icons"}},
			{name: "keep lesser", opts: []Option{WithEnv(Env{KeepLesser: true})}, tail: []string{"disappeared:" + itoa(b.weak) + ":1", "icons"}},
		} {
			t.Run(b.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				var events []string
				list := newTestList(iconEventOwner{eventOwner{events: &events}}, tc.opts...)
				target := &droppingSummon{list: list, events: &events}
				weak := newBlessing(t, target, b.name, b.weak, b.stack, 6)
				strong := newBlessing(t, target, b.name, b.strong, b.stack, 8)

				list.Add(weak)
				events = nil
				list.Add(strong)

				want := append([]string{
					"owner:remove:" + b.name,
					"stop:" + b.name,
					"disappeared:" + itoa(b.weak) + ":1",
					"owner:add",
					"felt:" + itoa(b.strong) + ":1",
				}, tc.tail...)
				requireEvents(t, events, want)
				if held := list.All(); len(held) != 1 || held[0] != strong {
					t.Fatalf("held effects = %v, want only the newcomer", held)
				}

				// Once the newcomer ends nothing is promoted back.
				events = nil
				list.Remove(strong)
				requireEvents(t, events, []string{
					"owner:remove:" + b.name,
					"disappeared:" + itoa(b.strong) + ":1",
					"icons",
					"stop:" + b.name,
				})
				if held := list.All(); len(held) != 0 {
					t.Fatalf("held effects after the newcomer ended = %v, want none", held)
				}
			})
		}
	}
}

// TestBlessingDropIgnoresEndedEffect ends a blessing the ordinary way: its
// exit hook runs after it left the list, so the drop it asks for changes
// nothing and adds no icon refresh.
func TestBlessingDropIgnoresEndedEffect(t *testing.T) {
	t.Parallel()
	var events []string
	list := newTestList(iconEventOwner{eventOwner{events: &events}})
	target := &droppingSummon{list: list, events: &events}
	e := newBlessing(t, target, "CharmOfLuck", 1325, "reduce_drop_penalty", 6)
	list.Add(e)
	events = nil

	list.Remove(e)
	requireEvents(t, events, []string{"owner:remove:CharmOfLuck", "disappeared:1325:1", "icons", "stop:CharmOfLuck"})
}

func itoa(id modelskill.ID) string { return strconv.Itoa(int(id)) }
