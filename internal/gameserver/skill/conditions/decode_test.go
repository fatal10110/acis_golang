package conditions

import "testing"

// TestParseBoolMatchesReference: the expected column is the output of a Java
// probe of Boolean.parseBoolean (OpenJDK 21.0.11), the reader of every
// boolean condition attribute.
func TestParseBoolMatchesReference(t *testing.T) {
	for raw, want := range map[string]bool{
		"true": true, "TRUE": true, "TrUe": true,
		"false": false, "1": false, "0": false, "t": false, "T": false, "yes": false, "on": false,
		"": false, " true": false, "true ": false,
	} {
		if got := parseBool(raw); got != want {
			t.Errorf("parseBool(%q) = %v, want %v", raw, got, want)
		}
	}
}
