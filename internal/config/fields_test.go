package config

import (
	"reflect"
	"testing"
)

func TestFieldsDefaultsAndValues(t *testing.T) {
	p, err := ParseString("intKey = 42\nstrKey = hello\nboolKey = true\n")
	if err != nil {
		t.Fatal(err)
	}
	f := NewFields(p, "test")

	if got := f.Int("intKey", 0); got != 42 {
		t.Fatalf("Int(intKey) = %d, want 42", got)
	}
	if got := f.Int("missing", 7); got != 7 {
		t.Fatalf("Int(missing) = %d, want default 7", got)
	}
	if got := f.String("strKey", "def"); got != "hello" {
		t.Fatalf("String(strKey) = %q, want hello", got)
	}
	if got := f.String("missing", "def"); got != "def" {
		t.Fatalf("String(missing) = %q, want def", got)
	}
	if got := f.Bool("boolKey", false); got != true {
		t.Fatalf("Bool(boolKey) = %v, want true", got)
	}
	if got := f.Bool("missing", true); got != true {
		t.Fatalf("Bool(missing) = %v, want default true", got)
	}
	if err := f.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
}

func TestFieldsMalformedSticksFirstError(t *testing.T) {
	p, err := ParseString("bad = notanumber\nother = 5\n")
	if err != nil {
		t.Fatal(err)
	}
	f := NewFields(p, "test prefix")

	if got := f.Int("bad", 3); got != 3 {
		t.Fatalf("Int(bad) = %d, want default 3 on parse failure", got)
	}
	if f.Err() == nil {
		t.Fatal("Err() = nil, want recorded parse error")
	}

	// Once an error is recorded, later calls are no-ops returning their
	// default, even for otherwise-valid keys.
	if got := f.Int("other", 99); got != 99 {
		t.Fatalf("Int(other) after sticky error = %d, want default 99", got)
	}
}

func TestFieldsIntPairsCommaAndSemicolon(t *testing.T) {
	p, err := ParseString("comma = 57-0,5575-0,6673-10\nsemi = 1-2;3-4\n")
	if err != nil {
		t.Fatal(err)
	}
	f := NewFields(p, "test")

	got := f.IntPairs("comma", "")
	want := []IntPair{{57, 0}, {5575, 0}, {6673, 10}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IntPairs(comma) = %#v, want %#v", got, want)
	}

	got = f.IntPairs("semi", "")
	want = []IntPair{{1, 2}, {3, 4}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IntPairs(semi) = %#v, want %#v", got, want)
	}

	got = f.IntPairs("missing", "9-9")
	want = []IntPair{{9, 9}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IntPairs(missing) = %#v, want default %#v", got, want)
	}

	if f.Err() != nil {
		t.Fatalf("Err() = %v, want nil", f.Err())
	}
}

func TestFieldsIntPairsMalformed(t *testing.T) {
	p, err := ParseString("bad = 1-2-3\n")
	if err != nil {
		t.Fatal(err)
	}
	f := NewFields(p, "test")

	if got := f.IntPairs("bad", ""); got != nil {
		t.Fatalf("IntPairs(bad) = %#v, want nil", got)
	}
	if f.Err() == nil {
		t.Fatal("Err() = nil, want recorded parse error")
	}
}

func TestFieldsNilProperties(t *testing.T) {
	f := NewFields(nil, "test")
	if got := f.Int("x", 5); got != 5 {
		t.Fatalf("Int on nil Properties = %d, want default 5", got)
	}
	if got := f.IntPairs("x", "1-1"); !reflect.DeepEqual(got, []IntPair{{1, 1}}) {
		t.Fatalf("IntPairs on nil Properties = %#v, want default", got)
	}
}
