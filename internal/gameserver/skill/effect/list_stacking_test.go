package effect

import (
	"testing"
)

// iconEventOwner is an eventOwner that also records each icon refresh, for
// tests that pin where the refresh falls among a removal's other steps.
type iconEventOwner struct{ eventOwner }

func (o iconEventOwner) UpdateEffectIcons() {
	*o.events = append(*o.events, "icons")
}

// The removal-order tests below pin the reference's FINISHING sequence
// (AbstractEffect.scheduleEffect, AbstractEffect.java:308-320): the
// EffectList removal pass (EffectList.removeEffectFromQueue, java:499-585:
// stat removal, next stack member's activation, then the worn-off /
// disappeared / aborted message) and its icon refresh (queueRunner,
// java:465-495) all finish before the effect's own onExit runs.

func TestListRemoveRunsExitHookAfterMessageAndIcons(t *testing.T) {
	var events []string
	list := newTestList(iconEventOwner{eventOwner{events: &events}})
	e := namedEffect("stun", 5, "none", 0, true, &events)
	e.Template.Icon = true
	e.Template.Count = 5

	list.Add(e)
	events = nil
	list.Remove(e)

	requireEvents(t, events, []string{
		"owner:remove:stun",
		"disappeared:5:0",
		"icons",
		"stun:exit",
	})
}

func TestListRemovePromotesNextStackMemberBeforeMessageAndExit(t *testing.T) {
	var events []string
	list := newTestList(iconEventOwner{eventOwner{events: &events}}, WithEnv(Env{KeepLesser: true}))
	weak := namedEffect("weak", 1, "speed", 1, false, &events)
	strong := namedEffect("strong", 2, "speed", 2, false, &events)
	strong.Template.Icon = true
	strong.Template.Count = 5

	list.Add(weak)
	list.Add(strong)
	events = nil
	list.Remove(strong)

	requireEvents(t, events, []string{
		"owner:remove:strong",
		"weak:start",
		"owner:add",
		"disappeared:2:0",
		"icons",
		"strong:exit",
	})
}

// An exit hook that ends further effects (ImmobileUntilAttacked stopping
// its skill's other effects) announces them only after its own effect's
// message and icon refresh have gone out.
func TestListRemoveExitHookRemovalsFollowOwnAnnouncement(t *testing.T) {
	var events []string
	list := newTestList(iconEventOwner{eventOwner{events: &events}})
	sibling := namedEffect("sibling", 4501, "none", 0, true, &events)
	sibling.Type = TypeDebuff
	sibling.Template.Icon = true
	sibling.Template.Count = 5
	self := namedEffect("self", 4501, "none", 0, true, &events)
	self.Template.Icon = true
	self.Template.Count = 5
	self.OnExit = func(*Effect) {
		events = append(events, "self:exit")
		list.Remove(sibling)
	}

	list.Add(self)
	list.Add(sibling)
	events = nil
	list.Remove(self)

	requireEvents(t, events, []string{
		"owner:remove:self",
		"disappeared:4501:0",
		"icons",
		"self:exit",
		"owner:remove:sibling",
		"disappeared:4501:0",
		"icons",
		"sibling:exit",
	})
}

// A recast of an identical buff runs the old effect's exit hook inside the
// add pass, then starts the newcomer, and only then removes the old
// effect's stats and announces it, ahead of the icon refresh.
func TestListIdenticalReplacementDefersOldRemoval(t *testing.T) {
	var events []string
	list := newTestList(iconEventOwner{eventOwner{events: &events}})
	old := namedEffect("old", 1204, "none", 0, false, &events)
	old.Template.Icon = true
	old.Template.Count = 5
	fresh := namedEffect("fresh", 1204, "none", 0, false, &events)
	fresh.Template.Icon = true
	fresh.Template.Count = 5

	list.Add(old)
	events = nil
	list.Add(fresh)

	requireEvents(t, events, []string{
		"old:stop",
		"old:exit",
		"fresh:start",
		"owner:add",
		"owner:remove:old",
		"disappeared:1204:0",
		"icons",
	})
	requireNames(t, list.All(), []string{"fresh"})
}

// A cap eviction retires the evicted buff the same way: exit hook first,
// newcomer start, then the evicted buff's removal and message.
func TestListCapEvictionDefersEvictedRemoval(t *testing.T) {
	var events []string
	list := newTestList(iconEventOwner{eventOwner{events: &events, maxBuff: 1}})
	first := buffSlotEffect("first", 1, &events)
	first.Template.Count = 5
	second := buffSlotEffect("second", 2, &events)

	list.Add(first)
	events = nil
	list.Add(second)

	requireEvents(t, events, []string{
		"first:stop",
		"first:exit",
		"second:start",
		"owner:add",
		"owner:remove:first",
		"disappeared:1:0",
		"icons",
	})
	requireNames(t, list.All(), []string{"second"})
}

// A buff that becomes its stack group's head is announced as felt, whether
// the group was empty or a lower-order buff led it; a displaced head is
// announced first, and the icon refresh follows both.
func TestListStackDisplacementAnnouncesDisappearedThenFelt(t *testing.T) {
	var events []string
	list := newTestList(iconEventOwner{eventOwner{events: &events}})
	weak := namedEffect("weak", 2011, "speed_up", 1, false, &events)
	weak.Template.Icon = true
	strong := namedEffect("strong", 2034, "speed_up", 2, false, &events)
	strong.Template.Icon = true

	list.Add(weak)
	requireEvents(t, events, []string{"weak:start", "owner:add", "felt:2011:0", "icons"})

	events = nil
	list.Add(strong)

	requireEvents(t, events, []string{
		"owner:remove:weak",
		"weak:exit",
		"disappeared:2011:0",
		"strong:start",
		"owner:add",
		"felt:2034:0",
		"icons",
	})
	requireNames(t, list.All(), []string{"strong"})
}

// A stack change of effects without icons, a newcomer that loses its stack
// group, and a newcomer whose start is rejected announce nothing.
func TestListStackDisplacementSilentCases(t *testing.T) {
	t.Run("no icon", func(t *testing.T) {
		var events []string
		list := newTestList(iconEventOwner{eventOwner{events: &events}})
		list.Add(namedEffect("weak", 1, "speed", 1, false, &events))
		events = nil
		list.Add(namedEffect("strong", 2, "speed", 2, false, &events))
		requireEvents(t, events, []string{"owner:remove:weak", "weak:exit", "strong:start", "owner:add", "icons"})
	})
	t.Run("no stack group", func(t *testing.T) {
		var events []string
		list := newTestList(iconEventOwner{eventOwner{events: &events}})
		buff := namedEffect("buff", 1, "none", 0, false, &events)
		buff.Template.Icon = true
		list.Add(buff)
		requireEvents(t, events, []string{"buff:start", "owner:add", "icons"})
	})
	t.Run("lower newcomer", func(t *testing.T) {
		var events []string
		list := newTestList(iconEventOwner{eventOwner{events: &events}}, WithEnv(Env{KeepLesser: true}))
		strong := namedEffect("strong", 2, "speed", 2, false, &events)
		strong.Template.Icon = true
		weak := namedEffect("weak", 1, "speed", 1, false, &events)
		weak.Template.Icon = true
		list.Add(strong)
		events = nil
		list.Add(weak)
		requireEvents(t, events, []string{"icons"})
	})
	t.Run("rejected start", func(t *testing.T) {
		var events []string
		list := newTestList(iconEventOwner{eventOwner{events: &events}})
		weak := namedEffect("weak", 1, "speed", 1, false, &events)
		weak.Template.Icon = true
		strong := namedEffect("strong", 2, "speed", 2, false, &events)
		strong.Template.Icon = true
		strong.OnStart = func(*Effect) bool {
			events = append(events, "strong:start")
			return false
		}
		list.Add(weak)
		events = nil
		list.Add(strong)
		requireEvents(t, events, []string{"owner:remove:weak", "weak:exit", "disappeared:1:0", "strong:start", "icons"})
	})
}

// Recasting an identical stacked buff replaces the stack head: the old
// buff's exit hook runs when it is retired and again when it loses the stack
// head, after its stat removal (#2763: AbstractEffect.scheduleEffect's
// FINISHING pass leaves _inUse set, so EffectList.addEffectFromQueue's
// setInUse(false) on the old head calls onExit a second time,
// EffectList.java:624-629, 757-765). It is announced as displaced and the
// recast as felt. With lesser effects cancelled the old buff is already gone
// when its removal runs, so nothing more is sent; kept, it is removed and
// announced a second time, with no third exit hook.
func TestListIdenticalStackedRecastAnnouncesHeadChange(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []Option
		tail []string
	}{
		{name: "cancel lesser", tail: []string{"icons"}},
		{name: "keep lesser", opts: []Option{WithEnv(Env{KeepLesser: true})}, tail: []string{"disappeared:1086:0", "icons"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events []string
			list := newTestList(iconEventOwner{eventOwner{events: &events}}, tc.opts...)
			old := namedEffect("old", 1086, "speed_up", 2, false, &events)
			old.Template.Icon = true
			old.Template.Count = 5
			fresh := namedEffect("fresh", 1086, "speed_up", 2, false, &events)
			fresh.Template.Icon = true
			fresh.Template.Count = 5

			list.Add(old)
			events = nil
			list.Add(fresh)

			want := append([]string{
				"old:stop",
				"old:exit",
				"owner:remove:old",
				"old:exit",
				"disappeared:1086:0",
				"fresh:start",
				"owner:add",
				"felt:1086:0",
			}, tc.tail...)
			requireEvents(t, events, want)
			requireNames(t, list.All(), []string{"fresh"})
		})
	}
}

// A buff retired by an identical recast still occupies its slot while the
// newcomer is placed, so a herb recast at capacity drops the newcomer after
// the old herb has already been retired.
func TestListIdenticalHerbRecastAtCapacityDropsBoth(t *testing.T) {
	var events []string
	list := newTestList(iconEventOwner{eventOwner{events: &events, maxBuff: 1}})
	old := buffSlotEffect("old", 2278, &events)
	old.Herb = true
	old.Template.Count = 5
	fresh := buffSlotEffect("fresh", 2278, &events)
	fresh.Herb = true

	list.Add(old)
	events = nil
	list.Add(fresh)

	requireEvents(t, events, []string{
		"old:stop",
		"old:exit",
		"fresh:stop",
		"owner:remove:old",
		"disappeared:2278:0",
		"icons",
	})
	requireNames(t, list.All(), []string{})
}

// An identical recast at full buff slots still counts the retired buff, so
// the cap eviction runs too. It walks the held buffs in order: an older
// other buff ahead of the recast one is evicted as well, while reaching the
// already-retired buff first uses up the eviction: it runs that buff's exit
// hook a second time, because retirement leaves it in use, but not its
// stop-task hook, and the other buff survives (#2763: a second exit() on an
// effect still _inUse reruns onExit, and stopEffectTask is a no-op once the
// task is gone, AbstractEffect.java:229-240, 310-320).
func TestListIdenticalRecastAtCapacityEvictsInHeldOrder(t *testing.T) {
	for _, tc := range []struct {
		name   string
		order  []string
		events []string
		held   []string
	}{
		{
			name:  "other buff first",
			order: []string{"a", "b"},
			events: []string{
				"b:stop",
				"b:exit",
				"a:stop",
				"a:exit",
				"b2:start",
				"owner:add",
				"owner:remove:b",
				"disappeared:2:0",
				"owner:remove:a",
				"disappeared:1:0",
				"icons",
			},
			held: []string{"b2"},
		},
		{
			name:  "recast buff first",
			order: []string{"b", "a"},
			events: []string{
				"b:stop",
				"b:exit",
				"b:exit",
				"b2:start",
				"owner:add",
				"owner:remove:b",
				"disappeared:2:0",
				"icons",
			},
			held: []string{"a", "b2"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events []string
			list := newTestList(iconEventOwner{eventOwner{events: &events, maxBuff: 2}})
			byName := map[string]*Effect{
				"a": buffSlotEffect("a", 1, &events),
				"b": buffSlotEffect("b", 2, &events),
			}
			for _, name := range tc.order {
				byName[name].Template.Count = 5
				list.Add(byName[name])
			}
			recast := buffSlotEffect("b2", 2, &events)
			recast.Template.Count = 5

			events = nil
			list.Add(recast)

			requireEvents(t, events, tc.events)
			requireNames(t, list.All(), tc.held)
			if byName["b"].InUse() || !recast.InUse() {
				t.Fatal("recast did not replace the retired buff")
			}
		})
	}
}

// A restored effect activates and refreshes icons like a cast one, but the
// owner gets none of the stack-change or expiry messages: the reference
// restores effects before the player has a client.
func TestListAddRestoredSendsNoStackMessages(t *testing.T) {
	t.Run("stack head change", func(t *testing.T) {
		var events []string
		list := newTestList(iconEventOwner{eventOwner{events: &events}}, WithEnv(Env{KeepLesser: true}))
		weak := namedEffect("weak", 201, "speed_up", 1, false, &events)
		weak.Template.Icon = true
		strong := namedEffect("strong", 202, "speed_up", 2, false, &events)
		strong.Template.Icon = true

		list.AddRestored(weak)
		list.AddRestored(strong)

		requireEvents(t, events, []string{
			"weak:start",
			"owner:add",
			"icons",
			"owner:remove:weak",
			"weak:exit",
			"strong:start",
			"owner:add",
			"icons",
		})
		requireNames(t, list.All(), []string{"weak", "strong"})
	})
	t.Run("identical replacement", func(t *testing.T) {
		var events []string
		list := newTestList(iconEventOwner{eventOwner{events: &events}})
		old := namedEffect("old", 1204, "none", 0, false, &events)
		old.Template.Icon = true
		old.Template.Count = 5
		fresh := namedEffect("fresh", 1204, "none", 0, false, &events)
		fresh.Template.Icon = true
		fresh.Template.Count = 5

		list.AddRestored(old)
		events = nil
		list.AddRestored(fresh)

		requireEvents(t, events, []string{
			"old:stop",
			"old:exit",
			"fresh:start",
			"owner:add",
			"owner:remove:old",
			"icons",
		})
	})
	t.Run("later cast still announces", func(t *testing.T) {
		var events []string
		list := newTestList(iconEventOwner{eventOwner{events: &events}})
		restored := namedEffect("restored", 201, "speed_up", 1, false, &events)
		restored.Template.Icon = true
		cast := namedEffect("cast", 202, "speed_up", 2, false, &events)
		cast.Template.Icon = true

		list.AddRestored(restored)
		events = nil
		list.Add(cast)

		requireEvents(t, events, []string{
			"owner:remove:restored",
			"restored:exit",
			"disappeared:201:0",
			"cast:start",
			"owner:add",
			"felt:202:0",
			"icons",
		})
	})
}
