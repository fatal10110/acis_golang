package script

import "testing"

// TestIsDigit: one or more ASCII digits and nothing else.
func TestIsDigit(t *testing.T) {
	for text, want := range map[string]bool{
		"0": true, "5": true, "0123456789": true, "99999999999": true,
		"": false, " 5": false, "5 ": false, "-1": false, "+1": false, "x": false, "1x": false, "٣": false, "１": false,
	} {
		if got := IsDigit(text); got != want {
			t.Errorf("IsDigit(%q) = %t, want %t", text, got, want)
		}
	}
}
