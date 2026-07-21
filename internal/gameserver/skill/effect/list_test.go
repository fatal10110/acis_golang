package effect

import (
	"reflect"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
)

type eventOwner struct {
	events  *[]string
	maxBuff int
}

func (o eventOwner) AddStatFuncs([]basefunc.Func) {
	*o.events = append(*o.events, "owner:add")
}

func (o eventOwner) RemoveStatsByOwner(owner any) {
	e := owner.(*Effect)
	*o.events = append(*o.events, "owner:remove:"+e.Template.Name)
}

func (o eventOwner) MaxBuffCount() int {
	if o.maxBuff == 0 {
		return 20
	}
	return o.maxBuff
}

func newEffect(name string, id modelskill.ID, stackType string, stackOrder float64, debuff bool) *Effect {
	e := &Effect{
		Skill: Skill{
			ID:     id,
			Debuff: debuff,
		},
		Template: modelskill.EffectTemplate{
			Name:       name,
			StackType:  stackType,
			StackOrder: stackOrder,
		},
		Type: TypeBuff,
	}
	e.OnStart = func(*Effect) bool {
		e.Template.Value++
		return true
	}
	return e
}

// flagGatedEffect returns a named debuff carrying flag and marked
// RejectsIfAffected, matching how New() builds a Stun/Root/Sleep/Fear
// effect: it must never be added while the owner already carries flag from
// any currently held effect.
func flagGatedEffect(name string, id modelskill.ID, flag Flag, events *[]string) *Effect {
	e := namedEffect(name, id, "none", 0, true, events)
	e.Flag = flag
	e.RejectsIfAffected = true
	return e
}

func namedEffect(name string, id modelskill.ID, stackType string, stackOrder float64, debuff bool, events *[]string) *Effect {
	e := newEffect(name, id, stackType, stackOrder, debuff)
	e.OnStart = func(*Effect) bool {
		*events = append(*events, name+":start")
		return true
	}
	e.OnExit = func(*Effect) {
		*events = append(*events, name+":exit")
	}
	e.OnStopTask = func(*Effect) {
		*events = append(*events, name+":stop")
	}
	return e
}

func effectNames(effects []*Effect) []string {
	names := make([]string, len(effects))
	for i, e := range effects {
		names[i] = e.Template.Name
	}
	return names
}

func requireEvents(t *testing.T, got []string, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v, want %#v", got, want)
	}
}

func requireNames(t *testing.T, got []*Effect, want []string) {
	t.Helper()
	if names := effectNames(got); !reflect.DeepEqual(names, want) {
		t.Fatalf("effects = %#v, want %#v", names, want)
	}
}

// The event and ordering expectations below were generated with a Java probe
// that keeps EffectList's stack insertion, replacement, and activation
// branches intact while replacing actor/network dependencies with log hooks.

func TestListReplacesLowerOrderStackedEffect(t *testing.T) {
	var events []string
	list := NewList(eventOwner{events: &events})

	weak := namedEffect("weak", 1, "speed", 1, false, &events)
	strong := namedEffect("strong", 2, "speed", 2, false, &events)

	list.Add(weak)
	list.Add(strong)

	requireEvents(t, events, []string{
		"weak:start",
		"owner:add",
		"owner:remove:weak",
		"weak:exit",
		"strong:start",
		"owner:add",
	})
	if weak.InUse() {
		t.Fatal("weaker effect stayed active after stronger replacement")
	}
	if !strong.InUse() {
		t.Fatal("stronger effect is not active")
	}
	requireNames(t, list.All(), []string{"strong"})
}

func TestListReactivatesNextStackedEffectWhenCancellationDisabled(t *testing.T) {
	var events []string
	list := NewList(eventOwner{events: &events}, WithCancelLesser(false))

	weak := namedEffect("weak", 1, "speed", 1, false, &events)
	strong := namedEffect("strong", 2, "speed", 2, false, &events)

	list.Add(weak)
	list.Add(strong)
	list.Remove(strong)

	requireEvents(t, events, []string{
		"weak:start",
		"owner:add",
		"owner:remove:weak",
		"weak:exit",
		"strong:start",
		"owner:add",
		"owner:remove:strong",
		"strong:exit",
		"weak:start",
		"owner:add",
	})
	if !weak.InUse() {
		t.Fatal("next stacked effect was not reactivated")
	}
	if strong.InUse() {
		t.Fatal("removed effect stayed active")
	}
	requireNames(t, list.All(), []string{"weak"})
}

func TestListOrdersBuffsBeforeTogglesThenDebuffs(t *testing.T) {
	list := NewList(nil)
	first := newEffect("first", 1, "none", 0, false)
	toggle := newEffect("toggle", 2, "none", 0, false)
	toggle.Skill.Toggle = true
	second := newEffect("second", 3, "none", 0, false)
	debuff := newEffect("debuff", 4, "none", 0, true)
	debuff.Type = TypeDebuff

	list.Add(first)
	list.Add(toggle)
	list.Add(second)
	list.Add(debuff)

	requireNames(t, list.All(), []string{"first", "second", "toggle", "debuff"})
}

func TestListReplacesIdenticalBuffButRejectsIdenticalDebuff(t *testing.T) {
	var events []string
	list := NewList(eventOwner{events: &events})

	buff1 := namedEffect("buff1", 1, "none", 0, false, &events)
	buff2 := namedEffect("buff2", 1, "none", 0, false, &events)
	debuff1 := namedEffect("debuff1", 2, "hex", 3, true, &events)
	debuff1.Type = TypeDebuff
	debuff2 := namedEffect("debuff2", 2, "hex", 3, true, &events)
	debuff2.Type = TypeDebuff

	list.Add(buff1)
	list.Add(buff2)
	list.Add(debuff1)
	list.Add(debuff2)

	requireEvents(t, events, []string{
		"buff1:start",
		"owner:add",
		"buff1:stop",
		"owner:remove:buff1",
		"buff1:exit",
		"buff2:start",
		"owner:add",
		"debuff1:start",
		"owner:add",
		"debuff2:stop",
	})
	requireNames(t, list.All(), []string{"buff2", "debuff1"})
	if !buff2.InUse() || !debuff1.InUse() {
		t.Fatal("replacement effects are not active")
	}
	if debuff2.InUse() {
		t.Fatal("rejected identical debuff became active")
	}
}

// buffSlotEffect returns a named, non-stacking buff-slot-family effect
// (the family of skill types that occupy an owner's limited buff slots and
// are shown as an icon).
func buffSlotEffect(name string, id modelskill.ID, events *[]string) *Effect {
	e := namedEffect(name, id, "none", 0, false, events)
	e.Skill.SkillType = "BUFF"
	e.Template.Icon = true
	return e
}

func TestListEvictsOldestBuffSlotEffectAtCapacity(t *testing.T) {
	var events []string
	list := NewList(eventOwner{events: &events, maxBuff: 2})

	first := buffSlotEffect("first", 1, &events)
	second := buffSlotEffect("second", 2, &events)
	third := buffSlotEffect("third", 3, &events)

	list.Add(first)
	list.Add(second)
	list.Add(third)

	requireNames(t, list.All(), []string{"second", "third"})
	if first.InUse() {
		t.Fatal("evicted buff stayed active")
	}
	found := false
	for _, ev := range events {
		if ev == "first:stop" {
			found = true
		}
	}
	if !found {
		t.Fatal("evicted buff's task was never stopped")
	}
}

func TestListDropsHerbEffectAtCapacityWithoutEvicting(t *testing.T) {
	var events []string
	list := NewList(eventOwner{events: &events, maxBuff: 1})

	real := buffSlotEffect("real", 1, &events)
	herb := namedEffect("herb", 2, "none", 0, false, &events)
	herb.Herb = true

	list.Add(real)
	list.Add(herb)

	requireEvents(t, events, []string{
		"real:start",
		"owner:add",
		"herb:stop",
	})
	requireNames(t, list.All(), []string{"real"})
	if herb.InUse() {
		t.Fatal("dropped herb effect became active")
	}
}

func TestListSkipsCapEvictionForIncomingStackingBuff(t *testing.T) {
	var events []string
	list := NewList(eventOwner{events: &events, maxBuff: 2})

	unrelated := buffSlotEffect("unrelated", 1, &events)
	weak := namedEffect("weak", 2, "speed", 1, false, &events)
	weak.Skill.SkillType = "BUFF"
	weak.Template.Icon = true
	strong := namedEffect("strong", 3, "speed", 2, false, &events)
	strong.Skill.SkillType = "BUFF"
	strong.Template.Icon = true

	list.Add(unrelated)
	list.Add(weak)
	list.Add(strong)

	requireNames(t, list.All(), []string{"unrelated", "strong"})
	if !unrelated.InUse() {
		t.Fatal("unrelated buff was displaced by cap eviction instead of the stack-group replacement")
	}
	if weak.InUse() {
		t.Fatal("weaker stacked buff stayed active")
	}
	if !strong.InUse() {
		t.Fatal("stronger stacked buff is not active")
	}
}

// runWithDeadlockGuard runs fn and fails t if it doesn't return within a
// short timeout, the symptom a reentrant hook self-deadlocking on List.mu
// would produce.
func runWithDeadlockGuard(t *testing.T, name string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s deadlocked", name)
	}
}

func TestListOnStartHookCanReenterAddWithoutDeadlock(t *testing.T) {
	list := NewList(nil)
	followUp := newEffect("followup", 2, "none", 0, false)

	reentrant := newEffect("reentrant", 1, "none", 0, false)
	reentrant.OnStart = func(*Effect) bool {
		list.Add(followUp)
		return true
	}

	runWithDeadlockGuard(t, "List.Add", func() {
		list.Add(reentrant)
	})

	requireNames(t, list.All(), []string{"reentrant", "followup"})
	if !followUp.InUse() {
		t.Fatal("effect added from within a reentrant OnStart hook never activated")
	}
}

func TestListOnExitHookCanReenterAddWithoutDeadlock(t *testing.T) {
	list := NewList(nil)
	var followUp *Effect

	reentrant := newEffect("reentrant", 1, "none", 0, false)
	reentrant.OnExit = func(*Effect) {
		followUp = newEffect("followup", 2, "none", 0, false)
		list.Add(followUp)
	}
	list.Add(reentrant)

	runWithDeadlockGuard(t, "List.Remove", func() {
		list.Remove(reentrant)
	})

	if followUp == nil || !followUp.InUse() {
		t.Fatal("effect added from within a reentrant OnExit hook never activated")
	}
}

func TestListFlagsAggregatesActiveEffectFlagsAndDropsThemOnRemoval(t *testing.T) {
	list := NewList(nil)

	stun := newEffect("stun", 1, "none", 0, true)
	stun.Flag = FlagStunned
	root := newEffect("root", 2, "none", 0, true)
	root.Flag = FlagRooted
	fear := newEffect("fear", 3, "none", 0, true)
	fear.Flag = FlagFear
	paralyze := newEffect("paralyze", 4, "none", 0, true)
	paralyze.Flag = FlagParalyzed

	if got := list.Flags(); got != 0 {
		t.Fatalf("Flags() on an empty list = %#x, want 0", got)
	}

	list.Add(stun)
	if !list.IsAffected(FlagStunned) {
		t.Fatal("IsAffected(FlagStunned) = false after adding a stun effect")
	}
	if list.IsAffected(FlagRooted) || list.IsAffected(FlagFear) || list.IsAffected(FlagParalyzed) {
		t.Fatal("IsAffected reported a flag from an effect never added")
	}

	list.Add(root)
	list.Add(fear)
	list.Add(paralyze)

	for _, flag := range []Flag{FlagStunned, FlagRooted, FlagFear, FlagParalyzed} {
		if !list.IsAffected(flag) {
			t.Fatalf("IsAffected(%#x) = false, want true with all four effects active", flag)
		}
	}
	if want := FlagStunned | FlagRooted | FlagFear | FlagParalyzed; list.Flags() != want {
		t.Fatalf("Flags() = %#x, want %#x", list.Flags(), want)
	}

	list.Remove(stun)
	if list.IsAffected(FlagStunned) {
		t.Fatal("IsAffected(FlagStunned) still true after its effect was removed")
	}
	if !list.IsAffected(FlagRooted) || !list.IsAffected(FlagFear) || !list.IsAffected(FlagParalyzed) {
		t.Fatal("removing one flagged effect cleared an unrelated flag")
	}

	list.Remove(root)
	list.Remove(fear)
	list.Remove(paralyze)
	if got := list.Flags(); got != 0 {
		t.Fatalf("Flags() after removing every effect = %#x, want 0", got)
	}
}

func TestListRejectsSecondFlagGatedEffectOfEachKindWhileFirstIsActive(t *testing.T) {
	tests := []struct {
		name string
		flag Flag
	}{
		{"stun", FlagStunned},
		{"root", FlagRooted},
		{"sleep", FlagSleep},
		{"fear", FlagFear},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var events []string
			list := NewList(eventOwner{events: &events})

			first := flagGatedEffect(tt.name+"1", 1, tt.flag, &events)
			second := flagGatedEffect(tt.name+"2", 2, tt.flag, &events)

			list.Add(first)
			list.Add(second)

			requireEvents(t, events, []string{
				tt.name + "1:start",
				"owner:add",
				tt.name + "2:stop",
			})
			if !first.InUse() {
				t.Fatal("first flag-gated effect was displaced instead of the second one being rejected")
			}
			if second.InUse() {
				t.Fatal("second flag-gated effect was added while its own flag was already active")
			}
			requireNames(t, list.All(), []string{tt.name + "1"})
		})
	}
}

func TestListRejectsFlagGatedEffectWhenFlagIsSetByADifferentEffectKind(t *testing.T) {
	var events []string
	list := NewList(eventOwner{events: &events})

	// stunSelf carries FlagStunned but, like the reference StunSelf effect,
	// is not itself flag-gated: it only ever blocks other Stunned-flag
	// effects, it never rejects itself.
	stunSelf := namedEffect("stunself", 1, "none", 0, false, &events)
	stunSelf.Flag = FlagStunned

	stun := flagGatedEffect("stun", 2, FlagStunned, &events)

	list.Add(stunSelf)
	list.Add(stun)

	requireEvents(t, events, []string{
		"stunself:start",
		"owner:add",
		"stun:stop",
	})
	if !stunSelf.InUse() {
		t.Fatal("stunself was displaced instead of the incoming stun being rejected")
	}
	if stun.InUse() {
		t.Fatal("stun was added despite FlagStunned already being set by a different effect kind")
	}
	requireNames(t, list.All(), []string{"stunself"})
}

func TestListDoesNotFlagGateParalyzeOrPetrificationEffects(t *testing.T) {
	var events []string
	list := NewList(eventOwner{events: &events})

	// Paralyze and Petrification both carry FlagParalyzed but, unlike
	// Stun/Root/Sleep/Fear, neither is flag-gated: a second one proceeds
	// through the ordinary buff/debuff handling instead of being rejected
	// outright.
	paralyze1 := namedEffect("paralyze1", 1, "none", 0, true, &events)
	paralyze1.Flag = FlagParalyzed
	paralyze2 := namedEffect("paralyze2", 2, "none", 0, true, &events)
	paralyze2.Flag = FlagParalyzed
	petrification := namedEffect("petrification", 3, "none", 0, true, &events)
	petrification.Flag = FlagParalyzed

	list.Add(paralyze1)
	list.Add(paralyze2)
	list.Add(petrification)

	if !paralyze1.InUse() {
		t.Fatal("first paralyze effect is not active")
	}
	if !paralyze2.InUse() {
		t.Fatal("second paralyze effect was rejected despite FlagParalyzed not being flag-gated")
	}
	if !petrification.InUse() {
		t.Fatal("petrification was rejected despite FlagParalyzed not being flag-gated")
	}
	requireNames(t, list.All(), []string{"paralyze1", "paralyze2", "petrification"})
}

func TestListRejectsSameSkillRecastOfFlagGatedEffectBeforeIdenticalDebuffLogic(t *testing.T) {
	var events []string
	list := NewList(eventOwner{events: &events})

	first := flagGatedEffect("stun", 7, FlagStunned, &events)
	recast := flagGatedEffect("stun", 7, FlagStunned, &events)
	// Line up every field the identical-debuff-reject branch compares, so
	// that branch alone (absent the flag gate) would produce the very same
	// "reject the incoming effect" outcome. The flag gate must still be
	// what actually fires: it runs before that branch is ever reached, and
	// it rejects based on the flag alone, not a same-skill/same-stack match.
	recast.Type = first.Type
	recast.Template.StackOrder = first.Template.StackOrder
	recast.Template.StackType = first.Template.StackType

	list.Add(first)
	list.Add(recast)

	requireEvents(t, events, []string{
		"stun:start",
		"owner:add",
		"stun:stop",
	})
	if !first.InUse() {
		t.Fatal("original stun was removed/exited instead of the recast being rejected")
	}
	if recast.InUse() {
		t.Fatal("recast stun became active")
	}
	requireNames(t, list.All(), []string{"stun"})
}
