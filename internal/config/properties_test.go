package config

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestIntPairsMalformedValueReturnsEmptyList(t *testing.T) {
	p, err := ParseString("items=57--100\n")
	if err != nil {
		t.Fatal(err)
	}

	got, err := p.IntPairs("items", "")
	if err != nil || got != nil {
		t.Fatalf("IntPairs() = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestIntPairsNonNumericValueReturnsEmptyList(t *testing.T) {
	p, err := ParseString("items=57-nope\n")
	if err != nil {
		t.Fatal(err)
	}

	got, err := p.IntPairs("items", "")
	if err != nil || got != nil {
		t.Fatalf("IntPairs() = (%v, %v), want (nil, nil)", got, err)
	}
}

// TestIntPairsOutOfInt32ReturnsEmptyList: the reference reads each number
// with Integer.parseInt, so one outside int32 is malformed and the tolerant
// list is empty, while the strict comma list fails.
func TestIntPairsOutOfInt32ReturnsEmptyList(t *testing.T) {
	for _, value := range []string{"4294973947-50", "6651-2147483648", "57-1;2147483648-1"} {
		p, err := ParseString("items=" + value + "\n")
		if err != nil {
			t.Fatal(err)
		}
		if got, err := p.IntPairs("items", ""); err != nil || got != nil {
			t.Errorf("IntPairs(%q) = (%v, %v), want (nil, nil)", value, got, err)
		}
	}
	p, err := ParseString("items=4294973947-0\n")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := p.IntPairsComma("items", ""); err == nil {
		t.Errorf("IntPairsComma(4294973947-0) = (%v, nil), want an error", got)
	}
	p, err = ParseString("items=2147483647-2147483647\n")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := p.IntPairs("items", ""); err != nil || len(got) != 1 || got[0] != (IntPair{First: 2147483647, Second: 2147483647}) {
		t.Errorf("IntPairs(int32 bounds) = (%v, %v), want the one pair", got, err)
	}
}

func TestFloat64TrimsWhitespace(t *testing.T) {
	p, err := ParseString("rate= 1.5 \n")
	if err != nil {
		t.Fatal(err)
	}

	got, err := p.Float64("rate", 0)
	if err != nil || got != 1.5 {
		t.Fatalf("Float64() = (%v, %v), want (1.5, nil)", got, err)
	}
}

func TestMissingPropertyLogsWarning(t *testing.T) {
	p, err := ParseString("")
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	previous := warnLog.Load()
	SetLogger(zerolog.New(&output))
	t.Cleanup(func() { warnLog.Store(previous) })

	if got := p.String("missing", "fallback"); got != "fallback" {
		t.Fatalf("String() = %q, want fallback", got)
	}
	if !strings.Contains(output.String(), "missing") {
		t.Fatalf("missing-key warning = %q", output.String())
	}
}

// OptionalInt must not warn for an absent key: MaxConnections is absent
// from every shipped .properties file, so routing it through Int would log
// a missing-property warning on every boot.
func TestOptionalInt(t *testing.T) {
	p, err := ParseString("Present = 16\nBad = nope\n")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}

	var output bytes.Buffer
	previous := warnLog.Load()
	SetLogger(zerolog.New(&output))
	t.Cleanup(func() { warnLog.Store(previous) })

	if n, ok, err := p.OptionalInt("Absent"); n != 0 || ok || err != nil {
		t.Errorf("OptionalInt(absent) = (%d, %v, %v), want (0, false, nil)", n, ok, err)
	}
	if n, ok, err := p.OptionalInt("Present"); n != 16 || !ok || err != nil {
		t.Errorf("OptionalInt(present) = (%d, %v, %v), want (16, true, nil)", n, ok, err)
	}
	if _, _, err := p.OptionalInt("Bad"); err == nil {
		t.Error("OptionalInt(malformed): want error, got nil")
	}

	if strings.Contains(output.String(), "config property missing") {
		t.Errorf("OptionalInt warned for an absent key; log = %q", output.String())
	}
}
