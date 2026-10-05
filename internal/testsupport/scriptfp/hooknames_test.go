package scriptfp

import (
	"reflect"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TestEveryHooksFieldNamesAReferenceHook keeps the Go field to reference
// hook naming in step with the engine's Hooks struct: each OnX field must
// name a hook some reference class overrides, or Check would report every
// port of that hook twice.
func TestEveryHooksFieldNamesAReferenceHook(t *testing.T) {
	fps, err := Reference()
	if err != nil {
		t.Fatalf("Reference: %v", err)
	}
	overridden := map[string]bool{}
	for _, fp := range fps {
		for _, h := range fp.Hooks {
			overridden[h.Name] = true
		}
	}
	fields := 0
	for f := range reflect.TypeFor[script.Hooks]().Fields() {
		invoker, ok := strings.CutPrefix(f.Name, "On")
		if !ok || !f.IsExported() {
			continue
		}
		fields++
		if name := referenceHookName(invoker); !overridden[name] {
			t.Errorf("Hooks.%s maps to %s, which no reference class overrides", f.Name, name)
		}
	}
	if fields == 0 {
		t.Fatal("script.Hooks has no OnX field")
	}
	if got := referenceHookName("Event"); got != "onAdvEvent" {
		t.Errorf("OnEvent maps to %s, want onAdvEvent", got)
	}
}
