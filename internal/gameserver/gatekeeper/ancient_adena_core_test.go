package gatekeeper

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
)

// TestAncientAdenaPrice pins TeleportLocation.getCalculatedPriceCount for a
// price in ancient adena: (int) (priceCount * 1.6) for anyone but a Gnosis
// follower, who pays priceCount; the weekend half price never applies to
// it. Expected values are the reference's double arithmetic truncated.
func TestAncientAdenaPrice(t *testing.T) {
	saturdayNight := time.Date(2026, time.July, 11, 21, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		count, surcharged int
	}{
		{0, 0}, {1, 1}, {7, 11}, {15, 24}, {100, 160}, {333, 532}, {1250, 2000},
	} {
		dest := travel.Teleport{Kind: travel.KindStandard, PriceID: int(item.AncientAdenaID), PriceCount: tc.count}
		s := NewService(nil, nil, false, func() time.Time { return saturdayNight })
		if got := s.price(dest, saturdayNight, 7); got != tc.surcharged {
			t.Errorf("price(%d) without a follower rule = %d, want %d", tc.count, got, tc.surcharged)
		}
		s.SetGnosisFollower(func(objectID int32) bool { return objectID == 7 })
		if got := s.price(dest, saturdayNight, 8); got != tc.surcharged {
			t.Errorf("price(%d) for a non-follower = %d, want %d", tc.count, got, tc.surcharged)
		}
		if got := s.price(dest, saturdayNight, 7); got != tc.count {
			t.Errorf("price(%d) for a follower = %d, want %d", tc.count, got, tc.count)
		}
	}
}
