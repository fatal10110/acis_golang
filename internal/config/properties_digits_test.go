package config

import (
	"slices"
	"testing"
)

// TestIntegerReadsAcceptUnicodeDigits pins the integer accessors to the
// reference's ExProperties, which reads them with Integer.parseInt and
// Long.parseLong: any Basic Multilingual Plane decimal digit reads as its
// value. A Java probe (OpenJDK 21.0.11, recorded in #3091) prints
// Integer.parseInt("１２") and Integer.parseInt("١٢") as 12; the other
// digits follow from the same Character.digit(c, 10) call, which reads every
// Unicode decimal digit and reads a fullwidth letter as a value of ten or
// more, so parseInt rejects "ａ".
func TestIntegerReadsAcceptUnicodeDigits(t *testing.T) {
	t.Parallel()
	p, err := ParseString("fw=１２\n" +
		"ar=١٢\n" +
		"dev=-१२\n" +
		"mixed=1２\n" +
		"letter=ａ\n" +
		"list=１,٢;-३\n" +
		"pairs=５７-１００;٦٦٥١-٣\n" +
		"comma=５７-0,5575-٠\n")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}

	for key, want := range map[string]int{"fw": 12, "ar": 12, "dev": -12, "mixed": 12} {
		if got, err := p.Int(key, 0); err != nil || got != want {
			t.Errorf("Int(%s) = (%d, %v), want %d", key, got, err, want)
		}
		if got, ok, err := p.OptionalInt(key); err != nil || !ok || got != want {
			t.Errorf("OptionalInt(%s) = (%d, %t, %v), want %d", key, got, ok, err, want)
		}
		if got, err := p.Int64(key, 0); err != nil || got != int64(want) {
			t.Errorf("Int64(%s) = (%d, %v), want %d", key, got, err, want)
		}
	}

	if _, err := p.Int("letter", 0); err == nil {
		t.Error("Int(letter) accepted a fullwidth letter, want an error")
	}
	if _, err := p.Int64("letter", 0); err == nil {
		t.Error("Int64(letter) accepted a fullwidth letter, want an error")
	}

	if got, err := p.Ints("list", nil); err != nil || !slices.Equal(got, []int{1, 2, -3}) {
		t.Errorf("Ints(list) = (%v, %v), want [1 2 -3]", got, err)
	}
	if got, err := p.Int64s("list", nil); err != nil || !slices.Equal(got, []int64{1, 2, -3}) {
		t.Errorf("Int64s(list) = (%v, %v), want [1 2 -3]", got, err)
	}

	wantPairs := []IntPair{{57, 100}, {6651, 3}}
	if got, err := p.IntPairs("pairs", ""); err != nil || !slices.Equal(got, wantPairs) {
		t.Errorf("IntPairs(pairs) = (%v, %v), want %v", got, err, wantPairs)
	}
	wantComma := []IntPair{{57, 0}, {5575, 0}}
	if got, err := p.IntPairsComma("comma", ""); err != nil || !slices.Equal(got, wantComma) {
		t.Errorf("IntPairsComma(comma) = (%v, %v), want %v", got, err, wantComma)
	}
}

// TestFloatReadsStayASCII pins Float64 to the reference's
// Double.parseDouble, whose FloatingDecimal.readJavaFormatString compares
// each character against '0'-'9' and so throws NumberFormatException for
// "１.５".
func TestFloatReadsStayASCII(t *testing.T) {
	t.Parallel()
	p, err := ParseString("rate=１.５\nlist=1.5,٢\n")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if got, err := p.Float64("rate", 0); err == nil {
		t.Errorf("Float64(rate) = %v, want an error", got)
	}
	if got, err := p.Float64s("list", nil); err == nil {
		t.Errorf("Float64s(list) = %v, want an error", got)
	}
}
