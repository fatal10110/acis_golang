package script

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// TestMakerHooksListsAgree checks the hand-kept maker lists against the
// fields: an invoker per field that calls it only when set, and With
// overlaying exactly that field.
func TestMakerHooksListsAgree(t *testing.T) {
	typ := reflect.TypeFor[MakerHooks]()
	for i := range typ.NumField() {
		f := typ.Field(i)
		name := strings.TrimPrefix(f.Name, "On")
		method, ok := reflect.TypeFor[*MakerHooks]().MethodByName(name)
		if !ok {
			t.Errorf("MakerHooks.%s has no invoker %s", f.Name, name)
			continue
		}
		args := []reflect.Value{reflect.ValueOf(&Maker{}), reflect.New(f.Type.In(1)).Elem()}

		var empty MakerHooks
		method.Func.Call(append([]reflect.Value{reflect.ValueOf(&empty)}, args...))

		called := false
		var set MakerHooks
		reflect.ValueOf(&set).Elem().Field(i).Set(reflect.MakeFunc(f.Type, func([]reflect.Value) []reflect.Value {
			called = true
			return nil
		}))
		method.Func.Call(append([]reflect.Value{reflect.ValueOf(&set)}, args...))
		if !called {
			t.Errorf("invoker %s does not call MakerHooks.%s", name, f.Name)
		}

		got := MakerHooks{}.With(set)
		for j := range typ.NumField() {
			if isSet := !reflect.ValueOf(got).Field(j).IsNil(); isSet != (j == i) {
				t.Errorf("With(%s) sets %s = %v", f.Name, typ.Field(j).Name, isSet)
			}
		}
	}
}

// fakeGroup is a spawn.Group that only names its maker.
type fakeGroup struct{ def *spawn.Maker }

func (g fakeGroup) Maker() *spawn.Maker                   { return g.def }
func (fakeGroup) Alive() int                              { return 0 }
func (fakeGroup) Held() bool                              { return false }
func (fakeGroup) Listed(string) bool                      { return false }
func (fakeGroup) Spawns() []spawn.GroupSpawn              { return nil }
func (fakeGroup) DeleteAll()                              {}
func (fakeGroup) SendEvent(string, string, int, int)      {}
func (fakeGroup) After(time.Duration, func())             {}
func (fakeGroup) Every(time.Duration, func()) *sim.Ticker { return nil }

// New gives each npcmaker a maker of its own, the fallback's for a type the
// catalog lacks; a panicking hook is recovered, and the next call runs.
func TestMakersNewPerNpcmakerWithFallback(t *testing.T) {
	var got []string
	ctor := func(tag string) func() Maker {
		return func() Maker {
			seen := 0
			return Maker{MakerHooks: MakerHooks{OnScriptEvent: func(_ *Maker, e MakerEvent) {
				seen++
				if e.Name == "boom" {
					panic("boom")
				}
				got = append(got, tag+":"+e.Name+":"+string(rune('0'+seen)))
			}}}
		}
	}
	r := NewMakers(MakerCatalog{"event_maker": ctor("event")}, ctor("default"), zerolog.Nop())
	if !r.Registered("event_maker") || r.Registered("random_spawn") {
		t.Fatal("Registered disagrees with the catalog")
	}
	g := fakeGroup{def: &spawn.Maker{Name: "m"}}
	a, b := r.New("event_maker"), r.New("event_maker")
	c := r.New("random_spawn")
	a.ScriptEvent(g, "x", 0, 0)
	a.ScriptEvent(g, "boom", 0, 0)
	a.ScriptEvent(g, "y", 0, 0)
	b.ScriptEvent(g, "x", 0, 0)
	c.ScriptEvent(g, "x", 0, 0)
	want := []string{"event:x:1", "event:y:3", "event:x:1", "default:x:1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("hook calls = %v, want %v", got, want)
	}
}
