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
