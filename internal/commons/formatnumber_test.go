package commons

import (
	"math"
	"testing"
)

func TestFormatNumber(t *testing.T) {
	for _, tt := range []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{1234567, "1,234,567"},
		{-1234, "-1,234"},
		{math.MaxInt32, "2,147,483,647"},
		{math.MinInt64, "-9,223,372,036,854,775,808"},
	} {
		if got := FormatNumber(tt.n); got != tt.want {
			t.Errorf("FormatNumber(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
