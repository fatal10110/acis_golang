package commons

import (
	"testing"
	"time"
)

// ---- from gameduration_test.go ----
func TestParseGameDuration(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    time.Duration
		wantErr bool
	}{
		{name: "seconds", in: "120sec", want: 120 * time.Second},
		{name: "minutes", in: "5min", want: 5 * time.Minute},
		{name: "hours", in: "2hour", want: 2 * time.Hour},
		{name: "no sentinel", in: "no", want: -time.Second},
		{name: "no sentinel case-insensitive", in: "NO", want: -time.Second},
		{name: "unrecognized suffix defaults to zero", in: "120", want: 0},
		{name: "empty defaults to zero", in: "", want: 0},
		{name: "malformed number is an error", in: "abcsec", wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseGameDuration(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("ParseGameDuration(%q) error = nil, want error", c.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseGameDuration(%q) error = %v", c.in, err)
			}
			if got != c.want {
				t.Fatalf("ParseGameDuration(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// ---- from legacyhash_test.go ----
// cases are known-good vectors for the algorithm, not values re-derived
// from the formula under test.
func TestLegacyStringHash(t *testing.T) {
	cases := []struct {
		in   string
		want int32
	}{
		{"", 0},
		{"a", 97},
		{"hello", 99162322},
		{"StatSet", -232503986},
		{"multisell/1.xml", 1568074198},
		{"Hello, World!", 1498789909},
		{"The quick brown fox jumps over the lazy dog", -609428141},
		{"123456", 1450575459},
		{"日本語", 25921943},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", -1203646688},
	}

	for _, c := range cases {
		if got := LegacyStringHash(c.in); got != c.want {
			t.Errorf("LegacyStringHash(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
