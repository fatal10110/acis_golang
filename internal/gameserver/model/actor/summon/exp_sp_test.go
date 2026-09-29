package summon

import (
	"math"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// TestPetAddExpAndSpGuards pins the per-amount guards of a pet exp/SP grant:
// a negative amount is skipped, SP stops at the 32-bit ceiling, and a grant
// where neither amount applies sends nothing. A grant where either amount
// applies reports the (rate-scaled) exp, even a negative one, as the
// pet-earned message does, and nothing else: PlayableStatus.addExpAndSp
// binds to addExp(long), so PetStatus.addExp(int)'s status refresh never
// runs on this path (PetStatus.java:25-42, PlayableStatus.java:70-130).
func TestPetAddExpAndSpGuards(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		startExp   int64
		startSP    int
		exp        int64
		sp         int
		wantExp    int64
		wantSP     int
		wantEvents []event.Event
	}{
		{
			name:       "sp clamps at int32 max",
			startSP:    math.MaxInt32 - 10,
			sp:         100,
			wantSP:     math.MaxInt32,
			wantEvents: []event.Event{event.ExpGained{Exp: 0}},
		},
		{
			name:       "negative sp leaves sp unchanged",
			startSP:    500,
			sp:         -100,
			wantSP:     500,
			wantEvents: []event.Event{event.ExpGained{Exp: 0}},
		},
		{
			name:       "exp still lands while sp is at max",
			startExp:   1000,
			startSP:    math.MaxInt32,
			exp:        100,
			sp:         50,
			wantExp:    1100,
			wantSP:     math.MaxInt32,
			wantEvents: []event.Event{event.ExpGained{Exp: 100}},
		},
		{
			name:       "negative exp is skipped while sp lands",
			startExp:   1000,
			startSP:    10,
			exp:        -40,
			sp:         25,
			wantExp:    1000,
			wantSP:     35,
			wantEvents: []event.Event{event.ExpGained{Exp: -40}},
		},
		{
			name:     "negative exp and full sp pool is silent",
			startExp: 1000,
			startSP:  math.MaxInt32,
			exp:      -40,
			sp:       25,
			wantExp:  1000,
			wantSP:   math.MaxInt32,
		},
		{
			name:     "negative exp and negative sp is silent",
			startExp: 1000,
			startSP:  10,
			exp:      -40,
			sp:       -5,
			wantExp:  1000,
			wantSP:   10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pet := mustPet(t, PetConfig{ObjectID: 1, Exp: tt.startExp, SP: tt.startSP})
			rec := &event.Recorder{}
			pet.Attach(Runtime{Sink: rec})

			pet.AddExpAndSp(tt.exp, tt.sp)

			if got := pet.Exp(); got != tt.wantExp {
				t.Errorf("Exp() = %d, want %d", got, tt.wantExp)
			}
			if got := pet.SP(); got != tt.wantSP {
				t.Errorf("SP() = %d, want %d", got, tt.wantSP)
			}
			if got := rec.Events(); !reflect.DeepEqual(got, tt.wantEvents) {
				t.Errorf("events = %+v, want %+v", got, tt.wantEvents)
			}
		})
	}
}
