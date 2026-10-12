package handleops

import (
	"go/types"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const fixturePkg = ScriptPkg + "/testdata/handleops"

// TestScriptFollowsEveryCallShape: the scan reports, per hook, the
// kind-specific operations each fixture script reaches, whatever way its
// hook calls them, and fails on a call it cannot follow.
func TestScriptFollowsEveryCallShape(t *testing.T) {
	prog, err := Load(filepath.Join("..", "..", ".."), "./internal/gameserver/script/testdata/handleops")
	if err != nil {
		t.Fatal(err)
	}
	scan := func(ctor string) (map[string][]string, error) {
		fn, ok := prog.pkgs[fixturePkg].Scope().Lookup(ctor).(*types.Func)
		if !ok {
			t.Fatalf("the fixture declares no %s", ctor)
		}
		return prog.Script([]*types.Func{fn})
	}

	for _, tc := range []struct {
		ctor string
		want map[string][]string
	}{
		{"Direct", map[string][]string{"OnAttacked": {"NPC.HP"}}},
		{"LocalVar", map[string][]string{"OnAttacked": {"NPC.Rooted"}}},
		{"Field", map[string][]string{"OnAttacked": {"NPC.MaxHP"}}},
		{"Map", map[string][]string{"OnAttacked": {"NPC.AbortAll"}}},
		{"Range", map[string][]string{"OnAttacked": {"NPC.Paralyzed"}}},
		{"Returned", map[string][]string{"OnAttacked": {"NPC.InCombat"}}},
		{"Invoker", map[string][]string{"OnAttacked": {"NPC.HP"}, "OnTalk": {"NPC.HP"}}},
		{"Engine", map[string][]string{"OnAttacked": {"NPC.HP"}, "OnTalk": {"NPC.HP"}}},
		{"Wrapped", map[string][]string{"OnAttacked": {"NPC.HP"}, "OnTimer": {"NPC.HP"}}},
		{"Dispatch", map[string][]string{"OnPartyAttacked": {"NPC.HP"}}},
	} {
		t.Run(tc.ctor, func(t *testing.T) {
			got, err := scan(tc.ctor)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ops = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("Param", func(t *testing.T) {
		got, err := scan("Param")
		if err == nil || !strings.Contains(err.Error(), "fixture.go") || !strings.Contains(err.Error(), ": f") {
			t.Fatalf("ops = %v, err = %v; want an unknown-origin error at the parameter call", got, err)
		}
	})
	t.Run("Unmade", func(t *testing.T) {
		got, err := scan("Unmade")
		if err == nil || !strings.Contains(err.Error(), "complex64") {
			t.Fatalf("ops = %v, err = %v; want an unknown-origin error at the field call", got, err)
		}
	})
}
