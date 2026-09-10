package effect

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestListWithClockSchedulesAndTicksAtItsClockTime(t *testing.T) {
	clock := scheduler.NewManualClock(time.Unix(100, 0))
	list := NewList(nil, WithClock(clock))
	e, err := New(Skill{ID: 1, Level: 1}, modelskill.EffectTemplate{Name: "Buff", Count: 1, Time: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	list.Add(e)

	clock.Advance(time.Second)
	list.Tick()
	if got := list.All(); len(got) != 0 {
		t.Fatalf("effects after clock deadline = %d, want 0", len(got))
	}
}
