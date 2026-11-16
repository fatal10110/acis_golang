package skills

import (
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// bluffSkillID is Bluff, whose effect turns its target to face the way its
// caster faces.
const bluffSkillID = 358

func newBluff(t *testing.T, effector, effected effect.Actor) *effect.Effect {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: bluffSkillID, Level: 1, Debuff: true},
		modelskill.EffectTemplate{Name: "Bluff", Count: 1, Time: 3, Icon: true})
	if err != nil {
		t.Fatalf("effect.New(Bluff): %v", err)
	}
	e.Effector, e.Effected = effector, effected
	return e
}

// assertForcedTurn finds the StartRotation of objectID in frames and checks
// it spins from the old heading at side 1, speed 65535, and that the very
// next frame is the StopRotation on the new heading at speed 65535
// (EffectBluff.onStart).
func assertForcedTurn(t *testing.T, frames [][]byte, who string, objectID int32, from, to int) {
	t.Helper()
	i := opcodeIndex(frames, serverpackets.OpcodeStartRotation)
	if i < 0 {
		t.Fatalf("%s: no StartRotation in %x", who, opcodeList(frames))
	}
	start := frames[i]
	le := binary.LittleEndian
	if got := []int32{int32(le.Uint32(start[1:])), int32(le.Uint32(start[5:])), int32(le.Uint32(start[9:])), int32(le.Uint32(start[13:]))}; got[0] != objectID || got[1] != int32(from) || got[2] != 1 || got[3] != 65535 {
		t.Fatalf("%s: StartRotation object/degree/side/speed = %v, want [%d %d 1 65535]", who, got, objectID, from)
	}
	if i+1 >= len(frames) || frames[i+1][0] != serverpackets.OpcodeStopRotation {
		t.Fatalf("%s: frames %x, want StopRotation right after StartRotation", who, opcodeList(frames))
	}
	stop := frames[i+1]
	if got := []int32{int32(le.Uint32(stop[1:])), int32(le.Uint32(stop[5:])), int32(le.Uint32(stop[9:]))}; got[0] != objectID || got[1] != int32(to) || got[2] != 65535 {
		t.Fatalf("%s: StopRotation object/degree/speed = %v, want [%d %d 65535]", who, got, objectID, to)
	}
	if n := len(framesWithOpcode(frames, serverpackets.OpcodeStartRotation)); n != 1 {
		t.Fatalf("%s: %d StartRotation frames, want 1", who, n)
	}
}

type headingSetter interface{ SetHeading(int) }

// TestBluffOnPlayerShowsTurn pins #4928 on a player: Bluff landing on the
// Target shows the Target itself and a Watcher a full-speed spin from the
// Target's heading, then a stop on the heading of the monster that cast it,
// and leaves the Target facing that way.
func TestBluffOnPlayerShowsTurn(t *testing.T) {
	t.Parallel()
	p := bootAppearancePair(t)
	px, py, pz := p.srv.PlayerPosition(t, p.targetID)
	hostile := p.srv.SpawnHostileNPCAt(t, location.Location{X: px + 20, Y: py, Z: pz})
	const from, to = 30000, 1000
	onHostileQueue(t, hostile, func() { hostile.SetHeading(to) })
	p.onQueue(t, func() { p.target.(headingSetter).SetHeading(from) })
	drainUntilQuiet(t, p.tc)
	drainUntilQuiet(t, p.wc)

	bluff := newBluff(t, hostile, p.target)
	p.onQueue(t, func() { p.target.EffectList().Add(bluff) })

	assertForcedTurn(t, readQuiet(t, p.tc), "target", p.targetID, from, to)
	assertForcedTurn(t, readQuiet(t, p.wc), "watcher", p.targetID, from, to)
	if got := p.target.Heading(); got != to {
		t.Fatalf("target heading = %d after Bluff, want %d", got, to)
	}
}

// TestBluffOnNPCShowsTurn pins #4928 on a monster: the players around it see
// the same spin and stop, from its heading to its caster's.
func TestBluffOnNPCShowsTurn(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Watcher", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	px, py, pz := srv.PlayerPosition(t, objID)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: px + 20, Y: py, Z: pz})
	caster := srv.SpawnHostileNPCAt(t, location.Location{X: px - 20, Y: py, Z: pz})
	const from, to = 12000, 50000
	onHostileQueue(t, hostile, func() { hostile.SetHeading(from) })
	onHostileQueue(t, caster, func() { caster.SetHeading(to) })
	drainUntilQuiet(t, c)

	bluff := newBluff(t, caster, hostile)
	onHostileQueue(t, hostile, func() { hostile.EffectList().Add(bluff) })

	assertForcedTurn(t, readQuiet(t, c), "watcher", hostile.ObjectID(), from, to)
	if got := hostile.Heading(); got != to {
		t.Fatalf("monster heading = %d after Bluff, want %d", got, to)
	}
}
