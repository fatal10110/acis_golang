package catalog

import (
	"maps"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TestJoinRejectsDuplicates: the join keeps every entry of disjoint files
// and refuses a key two files hold.
func TestJoinRejectsDuplicates(t *testing.T) {
	ctor := func() script.Script { return script.Script{} }
	got, err := join("script", map[string]script.Catalog{"a": {"quest.A": ctor}, "b": {"quest.B": ctor}})
	if err != nil {
		t.Fatal(err)
	}
	if keys := slices.Sorted(maps.Keys(got)); !slices.Equal(keys, []string{"quest.A", "quest.B"}) {
		t.Fatalf("joined %v, want quest.A and quest.B", keys)
	}
	_, err = join("script", map[string]script.Catalog{"a": {"quest.A": ctor}, "b": {"quest.A": ctor}})
	if err == nil || err.Error() != "script quest.A is in catalogs a and b" {
		t.Fatalf("a path in two files joined with error %v", err)
	}
	maker := func() script.Maker { return script.Maker{} }
	if _, err := join("maker type", map[string]script.MakerCatalog{"a": {"x": maker}, "b": {"x": maker}}); err == nil {
		t.Fatal("a maker type in two files joined")
	}
}
