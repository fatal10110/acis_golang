package commons

import (
	"strconv"
	"testing"
)

// TestDecodeInt32MatchesReferenceOracle pins DecodeInt32 to the reference's
// Integer.decode. Expected values are the output of a Java probe (OpenJDK
// 21.0.11) that printed Integer.decode(s), or "ERR" when it threw, for each
// input below; they are not derived from DecodeInt32. The set covers each
// radix prefix with and without sign, octal-vs-decimal leading zeros, the
// int32 bounds in every radix, and the syntax strconv's base-0 mode accepts
// but the reference does not (0b/0o prefixes, "_" separators, signs after
// the prefix, surrounding space).
func TestDecodeInt32MatchesReferenceOracle(t *testing.T) {
	t.Parallel()

	const errResult = "ERR"
	oracle := []struct{ in, want string }{
		{"0", "0"},
		{"12", "12"},
		{"-12", "-12"},
		{"+12", "12"},
		{"010", "8"},
		{"-010", "-8"},
		{"08", errResult},
		{"00", "0"},
		{"-0", "0"},
		{"+0", "0"},
		{"0x10", "16"},
		{"0X10", "16"},
		{"#10", "16"},
		{"-0x10", "-16"},
		{"+0x10", "16"},
		{"-#10", "-16"},
		{"0x", errResult},
		{"#", errResult},
		{"-", errResult},
		{"+", errResult},
		{"", errResult},
		{" 10", errResult},
		{"10 ", errResult},
		{"0b1", errResult},
		{"0o10", errResult},
		{"1_0", errResult},
		{"0x1_0", errResult},
		{"0x-10", errResult},
		{"0x+10", errResult},
		{"#-1", errResult},
		{"--1", errResult},
		{"+-1", errResult},
		{"-+1", errResult},
		{"0-1", errResult},
		{"2147483647", "2147483647"},
		{"2147483648", errResult},
		{"-2147483648", "-2147483648"},
		{"-2147483649", errResult},
		{"0x7fffffff", "2147483647"},
		{"0x7FFFFFFF", "2147483647"},
		{"0x80000000", errResult},
		{"-0x80000000", "-2147483648"},
		{"-0x80000001", errResult},
		{"0xffffffff", errResult},
		{"017777777777", "2147483647"},
		{"020000000000", errResult},
		{"-020000000000", "-2147483648"},
		{"0xG", errResult},
		{"1e3", errResult},
		{"1.0", errResult},
		{"abc", errResult},
		{"0x0", "0"},
		{"#0", "0"},
		{"00x10", errResult},
		{"0xx1", errResult},
		{"0177", "127"},
		{"0200", "128"},
		{"0x7f", "127"},
		{"0x80", "128"},
	}
	for _, tc := range oracle {
		got := errResult
		if n, err := DecodeInt32(tc.in); err == nil {
			got = strconv.Itoa(int(n))
		}
		if got != tc.want {
			t.Errorf("DecodeInt32(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}
