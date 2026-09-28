package pets

import (
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// wallGeo is passable geodata with a wall at x = wallX: a straight-line
// destination beyond it resolves to the wall, the way the geodata valid-
// location walk stops at the last reachable cell. It records every request.
type wallGeo struct {
	gameservertest.Geo
	wallX int

	mu    sync.Mutex
	calls [][6]int
}

func (g *wallGeo) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	g.mu.Lock()
	g.calls = append(g.calls, [6]int{ox, oy, oz, tx, ty, tz})
	g.mu.Unlock()
	return location.Location{X: min(tx, g.wallX), Y: ty, Z: tz}
}

func (g *wallGeo) requests() [][6]int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([][6]int(nil), g.calls...)
}

// TestThrowUpKnocksBackPet knocks the pet back with a ThrowUp effect cast
// by another player. The landing point is the effector-to-pet line pushed
// out by the skill's fly radius, cut short by geodata: every observer sees
// the pet fly there, and when the effect ends the pet stands on the landing
// point and every observer receives the position correction.
func TestThrowUpKnocksBackPet(t *testing.T) {
	t.Parallel()
	geo := &wallGeo{wallX: 250}
	h := bootOwnerWithCollarAndGeo(t, geo)
	pet, _ := h.spawnWolf(t)
	thrower := h.joinSecondPlayer(t, "Thrower")
	placePet(t, pet, location.Location{X: 110, Y: 20, Z: 30})
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, thrower.client)

	// The thrower stands at (10, 20, 30), 100 units from the pet. The push
	// is that distance plus the 200 fly radius, measured from the thrower:
	// (310, 20, 30) straight-line, which the wall at x = 250 cuts short.
	throwUp, err := effect.New(
		effect.Skill{ID: 4107, Level: 1, Debuff: true, FlyRadius: 200},
		modelskill.EffectTemplate{Name: "ThrowUp", Time: 1},
	)
	if err != nil {
		t.Fatalf("effect.New(ThrowUp): %v", err)
	}
	throwUp.Effector, throwUp.Effected = thrower.actor.(effect.Actor), pet
	runOn(t, pet.Queue(), func() { pet.EffectList().Add(throwUp) })
	if !petHasEffect(pet, effect.TypeThrowUp) {
		t.Fatal("ThrowUp did not land on the pet")
	}

	origin := location.Location{X: 110, Y: 20, Z: 30}
	landing := location.Location{X: 250, Y: 20, Z: 30}
	if got := geo.requests(); len(got) == 0 || got[len(got)-1] != [6]int{110, 20, 30, 310, 20, 30} {
		t.Fatalf("geodata valid-location requests = %v, want last from the pet (110,20,30) to (310,20,30)", got)
	}
	for _, c := range []*testsupport.ScriptedClient{h.client, thrower.client} {
		frame := findPetFrame(t, drainFrames(t, c), serverpackets.OpcodeFlyToLocation, pet.ObjectID(), "FlyToLocation")
		r := wire.NewReader(frame[5:])
		dest := location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
		at := location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
		flight := r.ReadInt32()
		if dest != landing || at != origin || flight != int32(modelskill.FlightThrowUp) {
			t.Fatalf("FlyToLocation = dest %v from %v type %d, want dest %v from %v type %d",
				dest, at, flight, landing, origin, modelskill.FlightThrowUp)
		}
	}
	if x, y, z := pet.Position(); (location.Location{X: x, Y: y, Z: z}) != origin {
		t.Fatalf("pet position = (%d,%d,%d) while still in flight, want unchanged %v", x, y, z, origin)
	}

	h.srv.Advance(t, 1100*time.Millisecond)
	h.srv.TickEffects()
	if petHasEffect(pet, effect.TypeThrowUp) {
		t.Fatal("ThrowUp still on the pet after its time ran out")
	}
	if x, y, z := pet.Position(); (location.Location{X: x, Y: y, Z: z}) != landing {
		t.Fatalf("pet position = (%d,%d,%d) after landing, want %v", x, y, z, landing)
	}
	if got := pet.Move().Position(); got != landing {
		t.Fatalf("pet movement origin = %v after landing, want %v", got, landing)
	}
	for _, c := range []*testsupport.ScriptedClient{h.client, thrower.client} {
		frame := findPetFrame(t, drainFrames(t, c), serverpackets.OpcodeValidateLocation, pet.ObjectID(), "ValidateLocation")
		r := wire.NewReader(frame[5:])
		at := location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
		if heading := int(r.ReadInt32()); at != landing || heading != pet.Heading() {
			t.Fatalf("ValidateLocation = %v heading %d, want %v heading %d", at, heading, landing, pet.Heading())
		}
	}
}

// findPetFrame returns the one frame among frames with opcode whose leading
// object id is petID.
func findPetFrame(t *testing.T, frames [][]byte, opcode byte, petID int32, what string) []byte {
	t.Helper()
	var found []byte
	for _, frame := range frames {
		if len(frame) < 5 || frame[0] != opcode || wire.NewReader(frame[1:]).ReadInt32() != petID {
			continue
		}
		if found != nil {
			t.Fatalf("pet %s sent twice: opcodes %x", what, frameOpcodes(frames))
		}
		found = frame
	}
	if found == nil {
		t.Fatalf("pet %s never arrived: opcodes %x", what, frameOpcodes(frames))
	}
	return found
}
