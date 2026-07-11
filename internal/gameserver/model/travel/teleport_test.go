package travel

import (
	"testing"
	"time"
)

func TestTeleportCalculatedPrice(t *testing.T) {
	tests := []struct {
		name string
		kind Kind
		now  time.Time
		want int
	}{
		{
			name: "weekday evening full price",
			kind: KindStandard,
			now:  time.Date(2026, time.July, 10, 21, 0, 0, 0, time.UTC), // Friday
			want: 1000,
		},
		{
			name: "saturday before core time full price",
			kind: KindStandard,
			now:  time.Date(2026, time.July, 11, 19, 59, 0, 0, time.UTC), // Saturday
			want: 1000,
		},
		{
			name: "saturday core time half price",
			kind: KindStandard,
			now:  time.Date(2026, time.July, 11, 20, 0, 0, 0, time.UTC), // Saturday
			want: 500,
		},
		{
			name: "sunday late core time half price",
			kind: KindStandard,
			now:  time.Date(2026, time.July, 12, 23, 59, 0, 0, time.UTC), // Sunday
			want: 500,
		},
		{
			name: "core time rounds down with a floor of one",
			kind: KindStandard,
			now:  time.Date(2026, time.July, 11, 20, 0, 0, 0, time.UTC),
			want: 1,
		},
		{
			name: "non-standard kind ignores core time",
			kind: KindNewbieToken,
			now:  time.Date(2026, time.July, 11, 20, 0, 0, 0, time.UTC), // Saturday
			want: 1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priceCount := 1000
			if tt.name == "core time rounds down with a floor of one" {
				priceCount = 1
			}
			tp := Teleport{Kind: tt.kind, PriceCount: priceCount}
			if got := tp.CalculatedPrice(tt.now); got != tt.want {
				t.Fatalf("CalculatedPrice() = %d, want %d", got, tt.want)
			}
		})
	}
}
