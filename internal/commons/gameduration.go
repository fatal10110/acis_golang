package commons

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseGameDuration parses a data-file duration string: a decimal number
// immediately followed by one unit suffix "hour", "min", or "sec", or the
// literal "no" (case-insensitive) meaning the timed event never fires,
// returned as a negative duration so callers can keep treating it as a
// single ordered value rather than a separate sentinel. A string with none
// of those suffixes parses to zero rather than an error, matching the
// data format's own tolerance for a malformed or empty value; a recognized
// suffix with a non-numeric prefix is still an error.
func ParseGameDuration(s string) (time.Duration, error) {
	if strings.EqualFold(s, "no") {
		return -time.Second, nil
	}

	units := []struct {
		suffix string
		unit   time.Duration
	}{
		{"hour", time.Hour},
		{"min", time.Minute},
		{"sec", time.Second},
	}
	for _, u := range units {
		n, ok := strings.CutSuffix(s, u.suffix)
		if !ok {
			continue
		}
		v, err := strconv.Atoi(n)
		if err != nil {
			return 0, fmt.Errorf("commons: game duration %q: %w", s, err)
		}
		return time.Duration(v) * u.unit, nil
	}
	return 0, nil
}
