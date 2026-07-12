package effect

import (
	"reflect"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
)

type eventOwner struct {
	events *[]string
}

func (o eventOwner) AddStatFuncs([]basefunc.Func) {
	*o.events = append(*o.events, "owner:add")
}

func (o eventOwner) RemoveStatsByOwner(owner any) {
	e := owner.(*Effect)
	*o.events = append(*o.events, "owner:remove:"+e.Template.Name)
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
