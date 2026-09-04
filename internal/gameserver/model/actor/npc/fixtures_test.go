package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// zeroRoll always returns 0, pinning MakeAttackHit's hit/crit/damage-spread
// rolls to a deterministic outcome: with any positive hit rate and
// critical rate, a roll of 0 always hits and always crits.
func zeroRoll(int) int { return 0 }

func newCombatHostile(t testing.TB, id int32, tpl *Template) *Hostile {
	t.Helper()
	h, err := NewHostile(&Instance{ObjectID: id, Template: tpl, Kind: "Monster"}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatal(err)
	}
	h.SetFrameBuilder(serverpackets.NpcFrameBuilder{})
	return h
}
func newTestHostile(t *testing.T, move ai.MoveController, strike ai.AttackController) *Hostile {
	t.Helper()
	hostile, err := NewHostile(&Instance{
		ObjectID: 101,
		Template: &Template{
			ID:              9001,
			Type:            "Monster",
			BaseAttackRange: 80,
			CanMove:         true,
		},
		Kind: "Monster",
	}, newHostileLive(t), move, strike)
	if err != nil {
		t.Fatal(err)
	}
	return hostile
}

func newHostileLive(t testing.TB) *creature.Live {
	t.Helper()
	live, err := creature.NewLive(location.Location{}, 100, hostileGeo{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return live
}

type hostileGeo struct{}

func (hostileGeo) CanMove(_, _, _, _, _, _ int) bool { return true }
func (hostileGeo) Height(_, _, _ int) int16          { return 0 }

// hostileGeo never blocks in these tests, so pathfinding and fall-back
// queries never need a useful answer: return no path and reflect the origin.
func (hostileGeo) FindPath(_, _ location.Location) ([]location.Location, bool) { return nil, false }
func (hostileGeo) Walkable(int, int, int) bool                                 { return true }
func (hostileGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

type hostileTarget struct {
	world.Presence
	id int32
}

func (t *hostileTarget) ObjectID() int32  { return t.id }
func (t *hostileTarget) SiegeGuard() bool { return false }
func (t *hostileTarget) AlikeDead() bool  { return false }

type hostileMove struct {
	followTarget attackable.Combatant
	followRange  int
	home         location.Location
	locations    []location.Location
	moved        chan location.Location
	stopCount    int

	// followResult and followErr let a test script
	// MaybeStartOffensiveFollow's return value, simulating the follow
	// controller either starting/maintaining a follow (true) or finding the
	// target already close enough (false), without needing real world
	// geometry.
	followResult bool
	followErr    error
	denyMove     bool
}

func (m *hostileMove) MaybeStartOffensiveFollow(target attackable.Combatant, attackRange int) (bool, error) {
	m.followTarget = target
	m.followRange = attackRange
	return m.followResult, m.followErr
}

func (m *hostileMove) MoveHome(home location.Location) error {
	m.home = home
	return nil
}

func (m *hostileMove) CanMoveTo(location.Location) bool { return !m.denyMove }

func (m *hostileMove) MoveToLocation(target location.Location) (bool, error) {
	m.locations = append(m.locations, target)
	if m.moved != nil {
		m.moved <- target
	}
	return true, nil
}

func (m *hostileMove) Stop() error { m.stopCount++; return nil }

type hostileAttack struct {
	canAttack bool
	target    attackable.Combatant
}

func (a *hostileAttack) BowCoolingDown() bool { return false }
func (a *hostileAttack) AttackingNow() bool   { return false }
func (a *hostileAttack) CanAttack(attackable.Combatant) bool {
	return a.canAttack
}
func (a *hostileAttack) DoAttack(target attackable.Combatant) error {
	a.target = target
	return nil
}

// hostileEffectTarget satisfies the flee hook a Fear effect's runtime needs,
// so it activates regardless of what its actual effected actor is.
type hostileEffectTarget struct{}

func (hostileEffectTarget) ObjectID() int32                                    { return 0 }
func (hostileEffectTarget) Dead() bool                                         { return false }
func (hostileEffectTarget) FleeFrom(effector effect.Participant, distance int) {}

func addHostileEffect(t *testing.T, hostile *Hostile, name string) *effect.Effect {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: name})
	if err != nil {
		t.Fatalf("effect.New(%q) error: %v", name, err)
	}
	e.Effected = hostileEffectTarget{}
	hostile.EffectList().Add(e)
	return e
}

type frameReceiver struct {
	world.Presence
	trackedID int32
	frames    [][]byte
}

func (f *frameReceiver) ObjectID() int32 { return f.trackedID }

func (f *frameReceiver) SendFrame(frame wire.Frame) bool {
	defer frame.Release()
	raw := frame.Bytes()
	payload := make([]byte, len(raw)-2)
	copy(payload, raw[2:])
	f.frames = append(f.frames, payload)
	return true
}

func (f *frameReceiver) BroadcastFrame(frame wire.Frame) bool { return f.SendFrame(frame) }
