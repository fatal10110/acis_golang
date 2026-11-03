package network

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// The reuse and remaining seconds of SkillCoolTime are the reference's
// (int) ms / 1000: the millisecond count narrows to an int before the division.
func TestSkillCoolTimeEntriesSeconds(t *testing.T) {
	now := time.UnixMilli(1_000_000)
	timers := []effect.ReuseTimer{
		{Delay: 15_999, ExpiresAt: 1_000_000 + 1_999},
		{Delay: 1<<32 + 5_000, ExpiresAt: 1_000_000 + 1<<32 + 7_000},
		{Delay: 60_000, ExpiresAt: 1_000_000}, // run out: not sent
	}
	got := skillCoolTimeEntries(timers, now)
	want := []serverpackets.SkillCoolTimeEntry{
		{ReuseSeconds: 15, RemainingSeconds: 1},
		{ReuseSeconds: 5, RemainingSeconds: 7},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("skillCoolTimeEntries() = %+v, want %+v", got, want)
	}
}
