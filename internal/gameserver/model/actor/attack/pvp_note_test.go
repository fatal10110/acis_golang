package attack

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// pvpNotingPlayer is a timing player that records every PvP note its
// swings take, by target id.
type pvpNotingPlayer struct {
	timingPlayer
	notes []int32
}

func (p *pvpNotingPlayer) NotePvPAttack(target attackable.Combatant) {
	p.notes = append(p.notes, target.ObjectID())
}

// TestControllerNotesPvPOncePerSwingFromTheMainTarget pins the PvP note to
// the swing's hit timer: one note, for the main target, however many hits
// the swing lands. A pole sweep's splash target and a dual swing's second
// hit note nothing more.
func TestControllerNotesPvPOncePerSwingFromTheMainTarget(t *testing.T) {
	tests := []struct {
		name   string
		weapon item.WeaponType
		splash bool
		until  time.Duration
		hits   int
	}{
		// attackTime 1000ms: the sweep lands at 500ms.
		{"pole", item.WeaponPole, true, 500 * time.Millisecond, 2},
		// attackTime 1000ms: hits at 250ms and 500ms, finished at 750ms.
		{"dual", item.WeaponDual, false, 750 * time.Millisecond, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			main := &timingTarget{id: 2, x: 40, attackable: true}
			actor := &pvpNotingPlayer{timingPlayer: timingPlayer{timingActor: timingActor{
				attackType:  tt.weapon,
				attackSpeed: 500,
			}}}
			if tt.splash {
				actor.poleMax = 2
				actor.known = []attackable.Combatant{&timingTarget{id: 3, x: 50, attackable: true}}
			}
			clock := newTimingClock()
			ctrl := NewPlayer(actor, nil)
			ctrl.SetQueue(clock.q)
			rec := &event.Recorder{}
			ctrl.sink = rec

			ctrl.DoAttack(main)
			clock.fire(tt.until)

			if got := event.Count[event.HitDealt](rec); got != tt.hits {
				t.Fatalf("hits dealt = %d, want %d", got, tt.hits)
			}
			if want := []int32{2}; !slices.Equal(actor.notes, want) {
				t.Fatalf("PvP notes = %v, want %v (once, for the main target)", actor.notes, want)
			}
		})
	}
}
