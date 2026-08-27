package spawn

import "testing"

func newFixtureTerritory(name string) *Territory {
	return &Territory{Name: name, MinZ: 0, MaxZ: 100, Nodes: []Node{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}}}
}

func newFixtureMaker(name string, territories ...*Territory) *Maker {
	return &Maker{Name: name, Territories: territories, MaximumNPCs: 1}
}

func TestTableTerritoryLookupIsCaseInsensitive(t *testing.T) {
	terr := newFixtureTerritory("Foo")
	table, err := NewTable([]*Territory{terr}, []*Maker{newFixtureMaker("Maker1", terr)})
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}

	got, ok := table.Territory("foo")
	if !ok || got != terr {
		t.Fatalf("Territory(%q) = %v, %v; want %v, true", "foo", got, ok, terr)
	}
	if _, ok := table.Territory("missing"); ok {
		t.Fatalf("Territory(%q) found, want absent", "missing")
	}
}

func TestTableMakerLookupIsCaseInsensitive(t *testing.T) {
	terr := newFixtureTerritory("Foo")
	maker := newFixtureMaker("Bar", terr)
	table, err := NewTable([]*Territory{terr}, []*Maker{maker})
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}

	got, ok := table.Maker("BAR")
	if !ok || got != maker {
		t.Fatalf("Maker(%q) = %v, %v; want %v, true", "BAR", got, ok, maker)
	}
	if _, ok := table.Maker("missing"); ok {
		t.Fatalf("Maker(%q) found, want absent", "missing")
	}
}

func TestTableTerritoryCaseVariantDuplicateKeepsFirstDeclared(t *testing.T) {
	first := newFixtureTerritory("Foo")
	second := newFixtureTerritory("FOO")
	table, err := NewTable([]*Territory{first, second}, []*Maker{newFixtureMaker("Maker1", first)})
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}

	got, ok := table.Territory("foo")
	if !ok || got != first {
		t.Fatalf("Territory(%q) = %v, %v; want first-declared %v, true", "foo", got, ok, first)
	}
}

func TestTableMakerCaseVariantDuplicateIsRejected(t *testing.T) {
	terr := newFixtureTerritory("Foo")
	_, err := NewTable([]*Territory{terr}, []*Maker{newFixtureMaker("Bar", terr), newFixtureMaker("BAR", terr)})
	if err == nil {
		t.Fatalf("NewTable: want error for case-variant duplicate maker name")
	}
}
