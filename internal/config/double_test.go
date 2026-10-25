package config

import (
	"errors"
	"math"
	"strconv"
	"testing"
)

func TestFloat64LiteralGrammar(t *testing.T) {
	inf, ninf := math.Inf(1), math.Inf(-1)
	accepted := []struct {
		in   string
		want float64
	}{
		{"1.", 1},
		{"0.7", 0.7},
		{".5", 0.5},
		{"-0", math.Copysign(0, -1)},
		{"+2.5e3", 2500},
		{"2.5E-1", 0.25},
		{"1e+2", 100},
		// Type suffixes do not change the value.
		{"1d", 1},
		{"1.5f", 1.5},
		{"2D", 2},
		{"3F", 3},
		{"1.1f", 1.1},
		{"1.e5d", 1e5},
		{"0d", 0},
		// Overflow saturates, underflow rounds to zero.
		{"1e400", inf},
		{"-1e400", ninf},
		{"1e99999999999999999999", inf},
		{"1e-400", 0},
		// Hexadecimal needs the binary exponent.
		{"0x1p-2", 0.25},
		{"0X1.8P1", 3},
		{"0x.8p1", 1},
		{"0x1.p1", 2},
		{"-0xAp0d", -10},
		{"0x1fp0f", 31},
		{"0x1p2000", inf},
		// Exact special spellings, signed or not.
		{"Infinity", inf},
		{"+Infinity", inf},
		{"-Infinity", ninf},
		// Surrounding spaces and control characters are dropped.
		{" 1.5 ", 1.5},
		{"\t2\x01", 2},
		{"Infinity ", inf},
	}
	for _, tc := range accepted {
		got, err := parseDouble(tc.in)
		if err != nil || got != tc.want || math.Signbit(got) != math.Signbit(tc.want) {
			t.Errorf("parseDouble(%q) = (%v, %v), want (%v, nil)", tc.in, got, err, tc.want)
		}
	}
	for _, in := range []string{"NaN", "+NaN", "-NaN", " NaN "} {
		if got, err := parseDouble(in); err != nil || !math.IsNaN(got) {
			t.Errorf("parseDouble(%q) = (%v, %v), want (NaN, nil)", in, got, err)
		}
	}

	rejected := []string{
		"", " ", "+", "-", ".", "d", ".d", "e5", "1e", "1e+", "1ef",
		"1dd", "1fd", "1.2.3", "1 2", "+ 1", "--1", "1_000", "1,5",
		"inf", "+Inf", "-inf", "infinity", "INFINITY", "Infinityd", "nan", "NAN", "NaNd",
		"0x", "0x1", "0x1f", "0xd", "0x.p1", "0xp1", "0x1p", "0x1pA", "0x_1p0", "0x1_0p0", "0x1.8.0p1",
		"\u00a01", "1\u00a0", "\u0661",
	}
	for _, in := range rejected {
		got, err := parseDouble(in)
		if !errors.Is(err, strconv.ErrSyntax) {
			t.Errorf("parseDouble(%q) = (%v, %v), want a syntax error", in, got, err)
		}
	}
}

func TestFloat64sUsesLiteralGrammar(t *testing.T) {
	p, err := ParseString("rates=1d, 1e400 ,0x1p-2\nbad=1,inf\n")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Float64s("rates", nil)
	if err != nil || len(got) != 3 || got[0] != 1 || !math.IsInf(got[1], 1) || got[2] != 0.25 {
		t.Errorf("Float64s(rates) = (%v, %v), want ([1 +Inf 0.25], nil)", got, err)
	}
	if got, err := p.Float64s("bad", nil); err == nil {
		t.Errorf("Float64s(bad) = (%v, nil), want an error", got)
	}
	if got, err := p.Float64("bad", 0); err == nil {
		t.Errorf("Float64(bad) = (%v, nil), want an error", got)
	}
}
