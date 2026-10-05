package script

import (
	"reflect"
	"strings"
	"testing"
)

// hookFields returns the exported func fields of Hooks, in order.
func hookFields(t *testing.T) []reflect.StructField {
	t.Helper()
	var out []reflect.StructField
	typ := reflect.TypeFor[Hooks]()
	for i := range typ.NumField() {
		if f := typ.Field(i); f.IsExported() {
			if f.Type.Kind() != reflect.Func {
				t.Fatalf("Hooks.%s is not a func", f.Name)
			}
			out = append(out, f)
		}
	}
	return out
}

// withHook returns Hooks with only field f set, to a func that records its
// call in *called and answers "x" when it returns a string.
func withHook(f reflect.StructField, called *bool) Hooks {
	var h Hooks
	fn := reflect.MakeFunc(f.Type, func([]reflect.Value) []reflect.Value {
		*called = true
		if f.Type.NumOut() == 1 {
			return []reflect.Value{reflect.ValueOf("x")}
		}
		return nil
	})
	reflect.ValueOf(&h).Elem().FieldByIndex(f.Index).Set(fn)
	return h
}

// TestHooksListsAgree checks the hand-kept lists against the fields: one
// hook per field in field order with the reference's method name, an
// invoker per field that calls it only when set, With overlaying exactly
// that field, and set reporting exactly that hook.
func TestHooksListsAgree(t *testing.T) {
	fields := hookFields(t)
	if len(fields) != int(hookCount) {
		t.Fatalf("Hooks has %d hook fields, hookCount is %d", len(fields), hookCount)
	}
	for i, f := range fields {
		h := hook(i)
		name := strings.TrimPrefix(f.Name, "On")
		want := "on" + name
		if h == hookEvent {
			want = "onAdvEvent"
		}
		if got := h.String(); got != want {
			t.Errorf("hook %d (%s) is named %q, want %q", i, f.Name, got, want)
		}

		method, ok := reflect.TypeFor[*Hooks]().MethodByName(name)
		if !ok {
			t.Errorf("Hooks has no invoker %s for %s", name, f.Name)
			continue
		}
		payload := f.Type.In(1)
		if method.Type.NumIn() != 3 || method.Type.In(1) != reflect.TypeFor[*Script]() || method.Type.In(2) != payload || method.Type.NumOut() != f.Type.NumOut() {
			t.Errorf("invoker %s has type %s, want func(*Hooks, *Script, %s) like %s", name, method.Type, payload, f.Type)
			continue
		}
		args := func(h *Hooks) []reflect.Value {
			return []reflect.Value{reflect.ValueOf(h), reflect.ValueOf(&Script{}), reflect.Zero(payload)}
		}

		var empty Hooks
		out := method.Func.Call(args(&empty))
		if len(out) == 1 && out[0].String() != "" {
			t.Errorf("unset %s answered %q, want \"\"", f.Name, out[0].String())
		}

		var called bool
		set := withHook(f, &called)
		out = method.Func.Call(args(&set))
		if !called {
			t.Errorf("invoker %s did not call %s", name, f.Name)
		}
		if len(out) == 1 && out[0].String() != "x" {
			t.Errorf("invoker %s answered %q, want the hook's \"x\"", name, out[0].String())
		}

		if got := set.set(); got != 1<<h {
			t.Errorf("set() with only %s = %b, want %b", f.Name, got, hookSet(1)<<h)
		}

		var parentCalled bool
		parent := withHook(fields[(i+1)%len(fields)], &parentCalled)
		derived := parent.With(set)
		if got, want := derived.set(), hookSet(1<<h|1<<hook((i+1)%len(fields))); got != want {
			t.Errorf("With(%s) over a parent set = %b, want %b", f.Name, got, want)
		}
		if got := derived.ownSet(); got != 1<<h {
			t.Errorf("With(%s) owns %b, want only %s", f.Name, got, f.Name)
		}
		called = false
		method.Func.Call(args(&derived))
		if !called {
			t.Errorf("With did not carry %s over", f.Name)
		}
	}
}

// TestBehaviorBindsItsOwnIDsToSetHooks checks the bound set: a behavior is
// bound, for its own ids only, to exactly the events whose hooks are set
// after With; the parent's ids are not inherited, and the talk hook binds
// nothing by itself.
func TestBehaviorBindsItsOwnIDsToSetHooks(t *testing.T) {
	parent := Script{Behavior: true, NPCs: []int32{1}, Hooks: Hooks{
		OnAttacked: func(*Script, Attacked) {},
		OnTalk:     func(*Script, Talk) string { return "" },
	}}
	child := Script{Behavior: true, NPCs: []int32{2, 3}, Hooks: parent.Hooks.With(Hooks{
		OnSeeSpell:              func(*Script, SeeSpell) {},
		OnAbnormalStatusChanged: func(*Script, AbnormalStatusChanged) {},
	})}
	got := boundOf(&child, allTemplates)
	want := Bindings{
		EventAttacked:              {2, 3},
		EventSeeSpell:              {2, 3},
		EventAbnormalStatusChanged: {2, 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bound = %v, want %v", got, want)
	}

	// Every name-bound event is reached by exactly its hook.
	for ev := range npcEventCount {
		d := npcEvents[ev]
		f := hookFields(t)[d.hook]
		var called bool
		s := Script{Behavior: true, NPCs: []int32{7}, Hooks: withHook(f, &called)}
		b := boundOf(&s, allTemplates)
		wantEvents := map[NPCEvent]bool{}
		for other := range npcEventCount {
			if npcEvents[other].byHook && npcEvents[other].hook == d.hook {
				wantEvents[other] = true
			}
		}
		for other := range npcEventCount {
			if got := len(b[other]) > 0; got != wantEvents[other] {
				t.Errorf("behavior setting only %s: bound to %s = %v, want %v", f.Name, other, got, wantEvents[other])
			}
		}
	}

	// A script that is not a behavior binds only what Bind lists.
	quest := Script{QuestID: 1, NPCs: []int32{9}, Hooks: Hooks{OnAttacked: func(*Script, Attacked) {}}, Bind: Bindings{EventTalked: {4}}}
	if got := boundOf(&quest, allTemplates); !reflect.DeepEqual(got, Bindings{EventTalked: {4}}) {
		t.Fatalf("quest bound = %v, want only its Bind", got)
	}
}

func allTemplates(int32) (NPCKind, bool) { return KindHostile, true }
