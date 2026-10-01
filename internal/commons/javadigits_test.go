package commons

import (
	"strconv"
	"testing"
)

// The tables in this file are the output of a Java probe (OpenJDK 21.0.11,
// Unicode 15.0), not of the Go code. javaDigitRanges lists every BMP char c
// with Character.digit(c, 36) >= 0 as runs of consecutive chars whose digit
// values rise by one: {first char, last char, value of the first}.
// javaIntOracle lists, per input, Integer.parseInt, Integer.decode,
// Long.parseLong and Byte.parseByte, or javaErr when the call threw.
var javaDigitRanges = []struct {
	lo, hi rune
	value  int
}{
	{0x0030, 0x0039, 0},
	{0x0041, 0x005A, 10},
	{0x0061, 0x007A, 10},
	{0x0660, 0x0669, 0},
	{0x06F0, 0x06F9, 0},
	{0x07C0, 0x07C9, 0},
	{0x0966, 0x096F, 0},
	{0x09E6, 0x09EF, 0},
	{0x0A66, 0x0A6F, 0},
	{0x0AE6, 0x0AEF, 0},
	{0x0B66, 0x0B6F, 0},
	{0x0BE6, 0x0BEF, 0},
	{0x0C66, 0x0C6F, 0},
	{0x0CE6, 0x0CEF, 0},
	{0x0D66, 0x0D6F, 0},
	{0x0DE6, 0x0DEF, 0},
	{0x0E50, 0x0E59, 0},
	{0x0ED0, 0x0ED9, 0},
	{0x0F20, 0x0F29, 0},
	{0x1040, 0x1049, 0},
	{0x1090, 0x1099, 0},
	{0x17E0, 0x17E9, 0},
	{0x1810, 0x1819, 0},
	{0x1946, 0x194F, 0},
	{0x19D0, 0x19D9, 0},
	{0x1A80, 0x1A89, 0},
	{0x1A90, 0x1A99, 0},
	{0x1B50, 0x1B59, 0},
	{0x1BB0, 0x1BB9, 0},
	{0x1C40, 0x1C49, 0},
	{0x1C50, 0x1C59, 0},
	{0xA620, 0xA629, 0},
	{0xA8D0, 0xA8D9, 0},
	{0xA900, 0xA909, 0},
	{0xA9D0, 0xA9D9, 0},
	{0xA9F0, 0xA9F9, 0},
	{0xAA50, 0xAA59, 0},
	{0xABF0, 0xABF9, 0},
	{0xFF10, 0xFF19, 0},
	{0xFF21, 0xFF3A, 10},
	{0xFF41, 0xFF5A, 10},
}

const javaErr = "ERR"

var javaIntOracle = []struct{ in, parseInt, decode, parseLong, parseByte string }{
	{"12", "12", "12", "12", "12"},
	{"\u0661\u0662", "12", "12", "12", "12"},
	{"\uFF11\uFF12", "12", "12", "12", "12"},
	{"-\u0661\u0662", "-12", "-12", "-12", "-12"},
	{"+\uFF11\uFF12", "12", "12", "12", "12"},
	{"1\u0662", "12", "12", "12", "12"},
	{"\u0966\u0967", "1", "1", "1", "1"},
	{"\u0BE7\u0BE8\u0BE9", "123", "123", "123", "123"},
	{"\u1040", "0", "0", "0", "0"},
	{"\uABF9", "9", "9", "9", "9"},
	{"\u0F29", "9", "9", "9", "9"},
	{"\uFF10\uFF11\uFF10", "10", "10", "10", "10"},
	{"0\uFF11\uFF10", "10", "8", "10", "10"},
	{"0\u0668", "8", javaErr, "8", "8"},
	{"0\u0667", "7", "7", "7", "7"},
	{"\uFF10x10", javaErr, javaErr, javaErr, javaErr},
	{"0\uFF3810", javaErr, javaErr, javaErr, javaErr},
	{"0\uFF5810", javaErr, javaErr, javaErr, javaErr},
	{"#\uFF26\uFF26", javaErr, "255", javaErr, javaErr},
	{"#\uFF46\uFF46", javaErr, "255", javaErr, javaErr},
	{"0x\uFF21", javaErr, "10", javaErr, javaErr},
	{"0X\uFF41\u0661", javaErr, "161", javaErr, javaErr},
	{"\uFF11\uFF21", javaErr, javaErr, javaErr, javaErr},
	{"-#\uFF11\uFF10", javaErr, "-16", javaErr, javaErr},
	{"\uFF12\uFF11\uFF14\uFF17\uFF14\uFF18\uFF13\uFF16\uFF14\uFF17", "2147483647", "2147483647", "2147483647", javaErr},
	{"\uFF12\uFF11\uFF14\uFF17\uFF14\uFF18\uFF13\uFF16\uFF14\uFF18", javaErr, javaErr, "2147483648", javaErr},
	{"-\uFF12\uFF11\uFF14\uFF17\uFF14\uFF18\uFF13\uFF16\uFF14\uFF18", "-2147483648", "-2147483648", "-2147483648", javaErr},
	{"\uFF0D\uFF11", javaErr, javaErr, javaErr, javaErr},
	{"\uFF0B\uFF11", javaErr, javaErr, javaErr, javaErr},
	{"\uFF11\u00A0", javaErr, javaErr, javaErr, javaErr},
	{"\u00A0\uFF11", javaErr, javaErr, javaErr, javaErr},
	{"\u00B2", javaErr, javaErr, javaErr, javaErr},
	{"\u00B9\u00B2", javaErr, javaErr, javaErr, javaErr},
	{"\u19DA", javaErr, javaErr, javaErr, javaErr},
	{"\u2460", javaErr, javaErr, javaErr, javaErr},
	{"\u2167", javaErr, javaErr, javaErr, javaErr},
	{"\U0001D7CF", // U+1D7CF, a surrogate pair in Java
		javaErr, javaErr, javaErr, javaErr},
	{"\u0661_\u0662", javaErr, javaErr, javaErr, javaErr},
	{"\u0661\u066B\u0662", javaErr, javaErr, javaErr, javaErr},
	{"\u0661,\u0662", javaErr, javaErr, javaErr, javaErr},
	{"\u0660", "0", "0", "0", "0"},
	{"\u0661\u0662\u0663\u0664\u0665\u0666\u0667\u0668\u0669\u0660", "1234567890", "1234567890", "1234567890", javaErr},
	{"\uFF19\uFF12\uFF12\uFF13\uFF13\uFF17\uFF12\uFF10\uFF13\uFF16\uFF18\uFF15\uFF14\uFF17\uFF17\uFF15\uFF18\uFF10\uFF17", javaErr, javaErr, "9223372036854775807", javaErr},
	{"\uFF19\uFF12\uFF12\uFF13\uFF13\uFF17\uFF12\uFF10\uFF13\uFF16\uFF18\uFF15\uFF14\uFF17\uFF17\uFF15\uFF18\uFF10\uFF18", javaErr, javaErr, javaErr, javaErr},
	{"\uFF11\uFF12\uFF17", "127", "127", "127", "127"},
	{"\uFF11\uFF12\uFF18", "128", "128", "128", javaErr},
	{"-\uFF11\uFF12\uFF18", "-128", "-128", "-128", "-128"},
	{"\uFF41", javaErr, javaErr, javaErr, javaErr},
	{"\u0663a", javaErr, javaErr, javaErr, javaErr},
}

// TestASCIIDigitMatchesReferenceDigitSet checks every BMP char: the ones
// the reference reads as a digit map to the ASCII digit or letter of the
// same value, and no other non-ASCII char maps at all.
func TestASCIIDigitMatchesReferenceDigitSet(t *testing.T) {
	t.Parallel()
	want := make(map[rune]byte)
	for _, rg := range javaDigitRanges {
		for r := rg.lo; r <= rg.hi; r++ {
			v := rg.value + int(r-rg.lo)
			if v < 10 {
				want[r] = byte('0' + v)
			} else {
				want[r] = byte('a' + v - 10)
			}
		}
	}
	for r := rune(0x80); r <= 0xFFFF; r++ {
		got, ok := asciiDigit(r)
		w, wok := want[r]
		if ok != wok || got != w {
			t.Errorf("asciiDigit(%U) = (%q, %v), want (%q, %v)", r, got, ok, w, wok)
		}
	}
	for _, r := range []rune{0x1D7CF, 0x104A1, 0x10FFFF} {
		if d, ok := asciiDigit(r); ok {
			t.Errorf("asciiDigit(%U) = %q, want no digit outside the BMP", r, d)
		}
	}
}

func TestIntParsersMatchReferenceOracle(t *testing.T) {
	t.Parallel()
	result := func(n int64, err error) string {
		if err != nil {
			return javaErr
		}
		return strconv.FormatInt(n, 10)
	}
	for _, tc := range javaIntOracle {
		if got := result(ParseInt(tc.in, 32)); got != tc.parseInt {
			t.Errorf("ParseInt(%q, 32) = %s, want Integer.parseInt %s", tc.in, got, tc.parseInt)
		}
		if got := result(ParseInt(tc.in, 64)); got != tc.parseLong {
			t.Errorf("ParseInt(%q, 64) = %s, want Long.parseLong %s", tc.in, got, tc.parseLong)
		}
		if got := result(ParseInt(tc.in, 8)); got != tc.parseByte {
			t.Errorf("ParseInt(%q, 8) = %s, want Byte.parseByte %s", tc.in, got, tc.parseByte)
		}
		n, err := DecodeInt32(tc.in)
		if got := result(int64(n), err); got != tc.decode {
			t.Errorf("DecodeInt32(%q) = %s, want Integer.decode %s", tc.in, got, tc.decode)
		}
		// Atoi is ParseInt at the platform int size; on a 64-bit build that
		// is Long.parseLong.
		a, err := Atoi(tc.in)
		if got := result(int64(a), err); strconv.IntSize == 64 && got != tc.parseLong {
			t.Errorf("Atoi(%q) = %s, want Long.parseLong %s", tc.in, got, tc.parseLong)
		}
	}
}

func TestParseIntErrorNamesTheInput(t *testing.T) {
	t.Parallel()
	in := "\uFF11\uFF12\uFF21"
	_, err := ParseInt(in, 32)
	ne, ok := err.(*strconv.NumError)
	if !ok || ne.Num != in || ne.Err != strconv.ErrSyntax {
		t.Fatalf("ParseInt(%q) error = %#v, want a syntax *strconv.NumError naming the input", in, err)
	}
	_, err = Atoi("\uFF19\uFF19\uFF19\uFF19\uFF19\uFF19\uFF19\uFF19\uFF19\uFF19\uFF19\uFF19\uFF19\uFF19\uFF19\uFF19\uFF19\uFF19\uFF19\uFF19")
	if ne, ok := err.(*strconv.NumError); !ok || ne.Err != strconv.ErrRange {
		t.Fatalf("Atoi(overflow) error = %#v, want a range *strconv.NumError", err)
	}
}
