package script

import (
	"slices"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// TestUnregisteredBehaviorHoldsItsIDs: a behavior listed after another that
// has no script still removes the earlier one from the (NPC, event) lists it
// claims (Config.Claims), so an id left with no behavior keeps its
// stand-ins. The earlier behavior keeps its other bindings, a plain
// script keeps its registration, and a behavior listed after the
// unregistered one binds the id again.
func TestUnregisteredBehaviorHoldsItsIDs(t *testing.T) {
	parent := behavior(1, 2)
	q := quest(Bindings{EventAttacked: {2}})
	cases := []struct {
		name    string
		list    []Listing
		catalog Catalog
		claims  map[string]Bindings
		// behaves2 is whether a behavior is bound to id 2, false for the
		// stand-ins; attacked2 and talk2 are its attacked and first-talk
		// lists.
		behaves2  bool
		attacked2 []string
		talk2     []string
	}{
		{
			"missing child", listOf("ai.Parent", "quest.Q", "ai.Child"),
			Catalog{"ai.Parent": parent, "quest.Q": q},
			map[string]Bindings{"ai.Child": {EventAttacked: {2}, EventFirstTalk: {2}}},
			false,
			[]string{"quest.Q"},
			nil,
		},
		{
			"missing child without a claim", listOf("ai.Parent", "quest.Q", "ai.Child"),
			Catalog{"ai.Parent": parent, "quest.Q": q},
			nil, true,
			[]string{"ai.Parent", "quest.Q"},
			[]string{"ai.Parent"},
		},
		{
			"ported child", listOf("ai.Parent", "quest.Q", "ai.Child"),
			Catalog{"ai.Parent": parent, "quest.Q": q, "ai.Child": behavior(2)},
			map[string]Bindings{"ai.Child": {EventAttacked: {2}, EventFirstTalk: {2}}},
			true,
			[]string{"quest.Q", "ai.Child"},
			[]string{"ai.Child"},
		},
		{
			"claim on some events", listOf("ai.Parent", "quest.Q", "ai.Child"),
			Catalog{"ai.Parent": parent, "quest.Q": q},
			map[string]Bindings{"ai.Child": {EventFirstTalk: {2}}},
			true,
			[]string{"ai.Parent", "quest.Q"},
			nil,
		},
		{
			"behavior listed after the missing child", listOf("ai.Parent", "quest.Q", "ai.Child", "ai.Later"),
			Catalog{"ai.Parent": parent, "quest.Q": q, "ai.Later": behavior(2)},
			map[string]Bindings{"ai.Child": {EventAttacked: {2}, EventFirstTalk: {2}}},
			true,
			[]string{"quest.Q", "ai.Later"},
			[]string{"ai.Later"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Build(c.list, c.catalog, Config{KindOf: allTemplates, Log: zerolog.Nop(), Claims: c.claims, raises: raiseAll})
			if !r.Behaves(1) || !slices.Equal(pathsOf(r.scripts(1, EventAttacked)), []string{"ai.Parent"}) {
				t.Errorf("id 1: Behaves %v, attacked %q; want ai.Parent", r.Behaves(1), pathsOf(r.scripts(1, EventAttacked)))
			}
			if got := r.Behaves(2); got != c.behaves2 {
				t.Errorf("Behaves(2) = %v, want %v", got, c.behaves2)
			}
			if got := pathsOf(r.scripts(2, EventAttacked)); !slices.Equal(got, c.attacked2) {
				t.Errorf("attacked on 2 = %q, want %q", got, c.attacked2)
			}
			if got := pathsOf(r.scripts(2, EventFirstTalk)); !slices.Equal(got, c.talk2) {
				t.Errorf("first talk on 2 = %q, want %q", got, c.talk2)
			}
			var dump strings.Builder
			if err := r.Dump(&dump); err != nil {
				t.Fatal(err)
			}
			if !c.behaves2 && !strings.Contains(dump.String(), "fold npc ATTACKED,FIRST_TALK ai.Parent 1\n") {
				t.Errorf("dump does not fold id 1 alone to the parent:\n%s", dump.String())
			}
		})
	}
}
