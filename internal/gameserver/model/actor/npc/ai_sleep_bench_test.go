package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// BenchmarkAITickIdleWorld is one AI task cycle over an idle world: 20k
// monsters spread over the grid, each homed where it stands, with no player
// anywhere, so every region is quiet and every monster sleeps in its
// territory. The cycle ticks none of them; its cost is what it pays to find
// that out.
func BenchmarkAITickIdleWorld(b *testing.B) {
	const total = 20000
	state := world.New()
	ai := task.NewAI(state, zerolog.Nop())
	tmpl := &Template{ID: 1, Type: "Monster"}
	for i := range total {
		at := location.Location{X: world.MinX + 1000 + (i%200)*1300, Y: world.MinY + 1000 + (i/200)*2600, Z: 0}
		h, err := NewHostile(&Instance{ObjectID: int32(i + 1), Template: tmpl, Kind: "Monster", HasHome: true, Home: at},
			newHostileLive(b), &hostileMove{}, &hostileAttack{})
		if err != nil {
			b.Fatal(err)
		}
		h.Attach(Runtime{World: state})
		state.Spawn(h, at.X, at.Y, at.Z, 0)
		ai.Add(h)
	}

	b.ReportAllocs()
	for b.Loop() {
		if err := ai.Tick(); err != nil {
			b.Fatal(err)
		}
	}
}
