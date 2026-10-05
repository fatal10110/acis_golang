package network

import "testing"

// TestParseJavaIntReadsUnicodeDigits pins parseJavaInt to the reference's
// Integer.parseInt: any Basic Multilingual Plane decimal digit reads as its
// value (a Java probe on OpenJDK 21.0.11, recorded in #3091, prints
// Integer.parseInt("１２") and Integer.parseInt("١٢") as 12), the sign
// stays ASCII, the range stays int32, and a fullwidth letter, a digit
// outside the plane, an empty string and a lone sign read nothing.
func TestParseJavaIntReadsUnicodeDigits(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]int32{
		"１２":          12,
		"١٢":          12,
		"-１２":         -12,
		"+٣":          3,
		"1２":          12,
		"２１４７４８３６４７":  2147483647,
		"-２１４７４８３６４８": -2147483648,
	} {
		if got, ok := parseJavaInt(in); !ok || got != want {
			t.Errorf("parseJavaInt(%q) = (%d, %t), want %d", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "-", "+", "ａ", "\U0001D7CF", "２１４７４８３６４８", "－１", "1 2"} {
		if got, ok := parseJavaInt(in); ok {
			t.Errorf("parseJavaInt(%q) = %d, want no number", in, got)
		}
	}
}
