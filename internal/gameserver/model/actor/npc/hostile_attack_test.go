package npc

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// zeroRoll always returns 0, pinning MakeAttackHit's hit/crit/damage-spread
// rolls to a deterministic outcome: with any positive hit rate and
// critical rate, a roll of 0 always hits and always crits.
func zeroRoll(int) int { return 0 }

func newCombatHostile(t *testing.T, id int32, tpl *Template) *Hostile {
	t.Helper()
	h, err := NewHostile(&Instance{ObjectID: id, Template: tpl, Kind: "Monster"}, &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestHostileMakeAttackHitResolvesDamage(t *testing.T) {
	attacker := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", PAtk: 100, DEX: 30, Level: 1, CritRate: 4})
	attacker.SetRollSource(zeroRoll)
	defender := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", PDef: 50, DEX: 30, Level: 1, HPMax: 1000})

	hit := attacker.MakeAttackHit(defender, false)

	// With attacker.PAtk=100, defender.PDef=50, an even accuracy/evasion
	// match (same DEX/level on both sides) and a guaranteed critical hit
	// (zeroRoll), the physical-attack formula (already verified against
	// the reference implementation) resolves to:
	//   (100*2 * 1(posMul) * 1(randomMul) + 0) * 77/50 = 308
	const wantDamage = 308
	if hit.Miss || hit.Damage != wantDamage {
		t.Fatalf("MakeAttackHit() = %+v, want %d damage", hit, wantDamage)
	}
	if got := defender.CurrentHP(); got != defender.MaxHP() {
		t.Fatalf("defender HP = %d, want unchanged %d", got, defender.MaxHP())
	}
}

func TestHostileMakeAttackHitMissesUnknownTargetType(t *testing.T) {
	attacker := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", PAtk: 100})
	attacker.SetRollSource(zeroRoll)

	hit := attacker.MakeAttackHit(&hostileTarget{id: 99}, false)
	if !hit.Miss {
		t.Fatal("MakeAttackHit().Miss = false, want true for a target with no physical-damage surface")
	}
}

func TestHostileAttackableByLiveAttacker(t *testing.T) {
	target := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", PDef: 50, DEX: 30, Level: 1, HPMax: 100})
	attacker := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", PAtk: 100, AtkSpd: 300, DEX: 30, Level: 1, HPMax: 100})

	if !target.AttackableBy(attacker) {
		t.Fatal("live hostile target is not attackable")
	}
	if target.AttackableBy(target) {
		t.Fatal("hostile target is attackable by itself")
	}
	target.MarkDead()
	if target.AttackableBy(attacker) {
		t.Fatal("dead hostile target is attackable")
	}
}

func TestHostileTakeDamageReachingZeroTriggersDieAndDecayChain(t *testing.T) {
	attacker := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", PAtk: 100, DEX: 30, Level: 1, CritRate: 4})
	attacker.SetRollSource(zeroRoll)
	defender := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", PDef: 50, DEX: 30, Level: 1, HPMax: 100, CorpseTime: 7})

	state := world.New()
	state.Spawn(defender, 100, 100, 0, 0)

	if defender.Dead() {
		t.Fatal("defender.Dead() = true before any damage, want false")
	}

	hit := attacker.MakeAttackHit(defender, false) // 308 damage vs 100 HP: lethal in one hit.
	defender.TakeDamage(hit.Damage, attacker)

	if !defender.Dead() {
		t.Fatal("defender.Dead() = false after a lethal hit, want true")
	}

	// The kill itself only latches the dead state (Die, exercised above via
	// TakeDamage); registering the corpse with the decay task is the
	// orchestration layer's job per Hostile.Die's own doc comment. Exercise
	// that same handoff here with the existing corpse-decay task.
	respawned := false
	effects := decayEffectsFunc(func(actor task.DecayActor) {
		h, ok := actor.(*Hostile)
		if !ok {
			t.Fatalf("decay actor = %T, want *Hostile", actor)
		}
		h.Decay(state, func() { respawned = true })
	})
	decay, err := task.NewDecay(effects, func() time.Time { return time.Unix(0, 0) })
	if err != nil {
		t.Fatal(err)
	}
	decay.Add(defender, 0)
	decay.Tick()

	if !defender.Decayed() {
		t.Fatal("defender.Decayed() = false after the decay task fired, want true")
	}
	if !respawned {
		t.Fatal("respawn hook was not called through the decay chain")
	}
	if _, ok := state.Object(defender.ObjectID()); ok {
		t.Fatal("defender is still tracked in the world after decay")
	}
}

type decayEffectsFunc func(task.DecayActor)

func (f decayEffectsFunc) Decay(actor task.DecayActor) { f(actor) }

func TestHostileBroadcastAttackSendsFrameToKnownReceivers(t *testing.T) {
	state := world.New()
	attacker := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster"})
	attacker.SetWorld(state)
	state.Spawn(attacker, 100, 100, 0, 0)

	observer := &frameReceiver{trackedID: 55}
	state.Spawn(observer, 100, 100, 0, 0)

	nonReceiver := &hostileTarget{id: 56}
	state.Spawn(nonReceiver, 100, 100, 0, 0)

	attacker.BroadcastAttack(serverpackets.AttackSnapshot{
		AttackerID: attacker.ObjectID(),
		Hits:       []serverpackets.AttackHit{{TargetID: 2, Damage: 10}},
	})

	if len(observer.frames) != 1 {
		t.Fatalf("observer received %d frames, want 1", len(observer.frames))
	}
	if observer.frames[0][0] != serverpackets.OpcodeAttack {
		t.Fatalf("frame opcode = %#x, want %#x", observer.frames[0][0], serverpackets.OpcodeAttack)
	}
}

func TestHostileBroadcastAttackNoopsWithoutWorld(t *testing.T) {
	attacker := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster"})
	// SetWorld was never called; this must not panic.
	attacker.BroadcastAttack(serverpackets.AttackSnapshot{AttackerID: 1})
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

var _ creature.DeathActor = (*Hostile)(nil)
