package commons

import (
	"testing"
	"time"
)

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
