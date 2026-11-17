package config

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func captureConfigLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	previous := warnLog.Load()
	SetLogger(zerolog.New(&output))
	t.Cleanup(func() { warnLog.Store(previous) })
	return &output
}

// TestIntPairsTakesEntriesAsWritten: the ';' list reads each entry and
// number untrimmed, so a space after the separator makes the whole list
// empty, and a missing key falls back to def without a missing-key log.
func TestIntPairsTakesEntriesAsWritten(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  []IntPair
	}{
		{"6651-50;6652-1", []IntPair{{6651, 50}, {6652, 1}}},
		{"6651-50; 6652-1", nil},
		{"6651 -50", nil},
		{"6651-50-5", nil},
		{"", nil},
		{";", nil},
		{"6651-50;", []IntPair{{6651, 50}}},
		{";6651-50", nil},
	} {
		p, err := ParseString("items=" + tc.value + "\n")
		if err != nil {
			t.Fatal(err)
		}
		if got := p.IntPairs("items", "57-1"); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("IntPairs(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}

	output := captureConfigLog(t)
	p, err := ParseString("")
	if err != nil {
		t.Fatal(err)
	}
	if got := p.IntPairs("OlyClassedReward", "6651-50"); !reflect.DeepEqual(got, []IntPair{{6651, 50}}) {
		t.Errorf("IntPairs(absent) = %v, want the default pair", got)
	}
	if strings.Contains(output.String(), "config property missing") {
		t.Errorf("IntPairs logged an absent key: %q", output.String())
	}
}

// TestIntPairsCommaTakesEntriesAsWritten: the ',' list reads each entry and
// number untrimmed and fails on a malformed one; parts after the second
// are ignored, and an empty value is malformed.
func TestIntPairsCommaTakesEntriesAsWritten(t *testing.T) {
	for _, tc := range []struct {
		value   string
		want    []IntPair
		wantErr bool
	}{
		{"57-0,5575-0", []IntPair{{57, 0}, {5575, 0}}, false},
		{"57-0, 5575-0", nil, true},
		{"57-0 ", nil, true},
		{"57-0-5", []IntPair{{57, 0}}, false},
		{"57--5", nil, true},
		{"57", nil, true},
		{"", nil, true},
		{",", []IntPair{}, false},
		{"57-0,", []IntPair{{57, 0}}, false},
	} {
		p, err := ParseString("items=" + tc.value + "\n")
		if err != nil {
			t.Fatal(err)
		}
		got, err := p.IntPairsComma("items", "")
		if (err != nil) != tc.wantErr || !tc.wantErr && !reflect.DeepEqual(got, tc.want) {
			t.Errorf("IntPairsComma(%q) = (%v, %v), want (%v, error %v)", tc.value, got, err, tc.want, tc.wantErr)
		}
	}
}

// TestIntGettersRejectOutsideInt32: Int, OptionalInt and Ints read a 32-bit
// int, so a value one past either bound is an error.
func TestIntGettersRejectOutsideInt32(t *testing.T) {
	p, err := ParseString("max=2147483647\nmin=-2147483648\nover=2147483648\nunder=-2147483649\nlist=1,2147483648\nbounds=2147483647,-2147483648\n")
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]int{"max": 2147483647, "min": -2147483648} {
		if got, err := p.Int(key, 0); err != nil || got != want {
			t.Errorf("Int(%s) = (%d, %v), want (%d, nil)", key, got, err, want)
		}
		if got, ok, err := p.OptionalInt(key); err != nil || !ok || got != want {
			t.Errorf("OptionalInt(%s) = (%d, %v, %v), want (%d, true, nil)", key, got, ok, err, want)
		}
	}
	for _, key := range []string{"over", "under"} {
		if got, err := p.Int(key, 0); err == nil {
			t.Errorf("Int(%s) = %d, want an error", key, got)
		}
		if got, _, err := p.OptionalInt(key); err == nil {
			t.Errorf("OptionalInt(%s) = %d, want an error", key, got)
		}
	}
	if got, err := p.Ints("list", nil); err == nil {
		t.Errorf("Ints(list) = %v, want an error", got)
	}
	if got, err := p.Ints("bounds", nil); err != nil || !reflect.DeepEqual(got, []int{2147483647, -2147483648}) {
		t.Errorf("Ints(bounds) = (%v, %v), want the two bounds", got, err)
	}
	if got, err := p.Int64("over", 0); err != nil || got != 2147483648 {
		t.Errorf("Int64(over) = (%d, %v), want (2147483648, nil)", got, err)
	}
}

// TestListGettersSplitLikeTheConfigFormat: a value made only of delimiters
// has no elements, a leading empty element is kept, every trailing one is
// dropped, an empty value is one empty element, and the vertical tab is a
// delimiter.
func TestListGettersSplitLikeTheConfigFormat(t *testing.T) {
	cases := []struct {
		value string
		want  []string
	}{
		{",", []string{}},
		{" ;, ", []string{}},
		{"a,,b", []string{"a", "b"}},
		{",a", []string{"", "a"}},
		{"a,", []string{"a"}},
		{"a\x0Bb", []string{"a", "b"}},
		{"", []string{""}},
	}
	for _, tc := range cases {
		p, err := ParseString("k=" + tc.value + "\n")
		if err != nil {
			t.Fatal(err)
		}
		if got := p.Strings("k", nil); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Strings(%q) = %q, want %q", tc.value, got, tc.want)
		}
		if got := p.Bools("k", nil); len(got) != len(tc.want) {
			t.Errorf("Bools(%q) = %v, want %d elements", tc.value, got, len(tc.want))
		}
	}

	numeric := []struct {
		value   string
		want    int
		wantErr bool
	}{
		{",", 0, false},
		{"1,,2", 2, false},
		{",1", 0, true},
		{"1,", 1, false},
		{"1\x0B2", 2, false},
		{"", 0, true},
	}
	for _, tc := range numeric {
		p, err := ParseString("k=" + tc.value + "\n")
		if err != nil {
			t.Fatal(err)
		}
		ints, err := p.Ints("k", nil)
		if (err != nil) != tc.wantErr || err == nil && len(ints) != tc.want {
			t.Errorf("Ints(%q) = (%v, %v), want %d elements, error %v", tc.value, ints, err, tc.want, tc.wantErr)
		}
		int64s, err := p.Int64s("k", nil)
		if (err != nil) != tc.wantErr || err == nil && len(int64s) != tc.want {
			t.Errorf("Int64s(%q) = (%v, %v), want %d elements, error %v", tc.value, int64s, err, tc.want, tc.wantErr)
		}
		floats, err := p.Float64s("k", nil)
		if (err != nil) != tc.wantErr || err == nil && len(floats) != tc.want {
			t.Errorf("Float64s(%q) = (%v, %v), want %d elements, error %v", tc.value, floats, err, tc.want, tc.wantErr)
		}
	}
}
