package cast

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestPlanReuseScalesByThePrecomputedSpeedRatio pins the reuse a cast plans
// against values worked in IEEE-754 double arithmetic as
// (int)(reuse * (333.0 / atkSpd)): the ratio is divided out first, then
// multiplied in. Multiplying by 333 before dividing rounds differently for
// these shipped reuse delays and lands one millisecond high (#4865). The
// 3,600,000 rows are the issue's probe values.
func TestPlanReuseScalesByThePrecomputedSpeedRatio(t *testing.T) {
	for _, tc := range []struct {
		reuse, spd int
		want       int
	}{
		{reuse: 700, spd: 525, want: 443},
		{reuse: 7000, spd: 1295, want: 1799},
		{reuse: 10500, spd: 2100, want: 1664},
		{reuse: 12000, spd: 160, want: 24974},
		{reuse: 90000, spd: 162, want: 184999},
		{reuse: 390000, spd: 130, want: 998999},
		{reuse: 1260000, spd: 1296, want: 323749},
		{reuse: 3600000, spd: 160, want: 7492499},
		{reuse: 3600000, spd: 162, want: 7399999},
		{reuse: 21600000, spd: 1280, want: 5619374},
		// Speeds where both orders agree stay as they were.
		{reuse: 6000, spd: 133, want: 15022},
		{reuse: 6000, spd: 333, want: 6000},
	} {
		for _, magic := range []bool{true, false} {
			actor := &testActor{mp: 100, hp: 100}
			if magic {
				actor.mAtkSpd = tc.spd
			} else {
				actor.pAtkSpd = tc.spd
			}
			def := modelskill.Definition{ID: 10, Level: 1, Magic: magic, HitTime: 1000, StaticHitTime: true, ReuseDelay: tc.reuse}
			plan := NewController(actor, nil, nil).buildPlan(&def)
			if want := time.Duration(tc.want) * time.Millisecond; plan.ReuseDelay != want {
				t.Errorf("reuse %d at speed %d (magic %v) = %v, want %v", tc.reuse, tc.spd, magic, plan.ReuseDelay, want)
			}
		}
	}
}

// TestPlanReuseTruncatesAfterTheRateThenTheSpeed pins the two truncations of
// the reuse delay: the reuse-rate stat result is cut to whole milliseconds
// before the speed ratio applies. Quick Recovery lvl 3 (mReuse x0.8) on a
// 6000 ms magic reuse at casting speed 133 plans 12018 ms.
func TestPlanReuseTruncatesAfterTheRateThenTheSpeed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		actor *testActor
		def   modelskill.Definition
		want  int
	}{
		{
			name:  "magic uses mReuse",
			actor: &testActor{mAtkSpd: 133, magicReuseRate: 0.8, physicalReuseRate: 3},
			def:   modelskill.Definition{Magic: true, ReuseDelay: 6000},
			want:  12018,
		},
		{
			name:  "physical uses pReuse",
			actor: &testActor{pAtkSpd: 300, magicReuseRate: 3, physicalReuseRate: 0.9},
			// int(1111 * 0.9) = 999, int(999 * (333.0/300)) = 1108.
			def:  modelskill.Definition{ReuseDelay: 1111},
			want: 1108,
		},
		{
			name:  "static reuse ignores both",
			actor: &testActor{mAtkSpd: 133, magicReuseRate: 0.8},
			def:   modelskill.Definition{Magic: true, ReuseDelay: 6000, StaticReuse: true},
			want:  6000,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := tc.def
			def.ID, def.Level, def.HitTime, def.StaticHitTime = 10, 1, 1000, true
			plan := NewController(tc.actor, nil, nil).buildPlan(&def)
			if want := time.Duration(tc.want) * time.Millisecond; plan.ReuseDelay != want {
				t.Fatalf("ReuseDelay = %v, want %v", plan.ReuseDelay, want)
			}
		})
	}
}
