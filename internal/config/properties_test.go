package config

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
	previous := log.Logger
	log.Logger = zerolog.New(&output)
	t.Cleanup(func() { log.Logger = previous })

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
	previous := log.Logger
	log.Logger = zerolog.New(&output)
	t.Cleanup(func() { log.Logger = previous })

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
