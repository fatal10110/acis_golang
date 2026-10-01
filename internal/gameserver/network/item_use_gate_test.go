package network

import "testing"

func TestParseConditionIntLiteralBases(t *testing.T) {
	cases := []struct {
		raw    string
		want   int
		wantOK bool
	}{
		{"10", 10, true},
		{"0x1F", 31, true},
		{"010", 8, true},
		{"", 0, false},
		{"abc", 0, false},
		{"0x100000000", 0, false},
		// Reference Integer.decode outcomes (Java probe): "#" hex is read,
		// while the base-0 extras and a sign after the prefix are rejected.
		{"#1F", 31, true},
		{"-0x10", -16, true},
		{"0b1", 0, false},
		{"0o10", 0, false},
		{"1_0", 0, false},
		{"0x-1", 0, false},
	}
	for _, c := range cases {
		got, ok := parseConditionInt(c.raw)
		if ok != c.wantOK || (ok && got != c.want) {
			t.Errorf("parseConditionInt(%q) = (%d, %v), want (%d, %v)", c.raw, got, ok, c.want, c.wantOK)
		}
	}
}

// TestParseConditionBoolMatchesReference: the expected column is the output
// of a Java probe of Boolean.parseBoolean (OpenJDK 21.0.11), the reader the
// reference uses for every boolean <player> use-condition attribute
// (DocumentBase.parsePlayerCondition).
func TestParseConditionBoolMatchesReference(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"true", true},
		{"TRUE", true},
		{"TrUe", true},
		{"false", false},
		{"1", false},
		{"0", false},
		{"t", false},
		{"T", false},
		{"yes", false},
		{"on", false},
		{"", false},
		{" true", false},
		{"true ", false},
	}
	for _, c := range cases {
		if got := parseConditionBool(c.raw); got != c.want {
			t.Errorf("parseConditionBool(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}

// TestUnreadableBoolConditionReadsFalse: a value that is not "true" is a
// false requirement, never a failed condition. moving/riding/olympiad pass
// while the player is not in that state, so "1" and "yes" let the item be
// used, and only a case-insensitive "true" blocks it.
func TestUnreadableBoolConditionReadsFalse(t *testing.T) {
	for _, attr := range []string{"moving", "riding", "olympiad"} {
		for _, raw := range []string{"1", "t", "yes", "", "false"} {
			if !playerUseConditionHolds(nil, map[string]string{attr: raw}) {
				t.Errorf("<player %s=%q> denied use, want it read as false", attr, raw)
			}
		}
		if playerUseConditionHolds(nil, map[string]string{attr: "TrUe"}) {
			t.Errorf("<player %s=\"TrUe\"> allowed use, want it read as true", attr)
		}
	}
}
