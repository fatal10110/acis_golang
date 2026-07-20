package npc

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
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

func TestHostileAttackTypeAndWeaponReuseDelay(t *testing.T) {
	const bowItemID = 500
	const bowReuseMillis = 1500

	bow := &item.Template{
		ID:     bowItemID,
		Kind:   item.KindWeapon,
		Weapon: &item.WeaponDetail{Type: item.WeaponBow, ReuseDelay: bowReuseMillis},
	}
	notAWeapon := &item.Template{ID: 501, Kind: item.KindEtcItem}
	items := item.NewTable([]*item.Template{bow, notAWeapon})

	tests := []struct {
		name           string
		rightHand      int
		items          *item.Table
		wantAttackType item.WeaponType
		wantReuseDelay time.Duration
	}{
		{
			name:           "no right-hand item id stays unarmed",
			rightHand:      0,
			items:          items,
			wantAttackType: item.WeaponFist,
		},
		{
			name:           "unknown right-hand item id stays unarmed",
			rightHand:      999,
			items:          items,
			wantAttackType: item.WeaponFist,
		},
		{
			name:           "right-hand item that isn't a weapon stays unarmed",
			rightHand:      int(notAWeapon.ID),
			items:          items,
			wantAttackType: item.WeaponFist,
		},
		{
			name:           "nil item table stays unarmed",
			rightHand:      bowItemID,
			items:          nil,
			wantAttackType: item.WeaponFist,
		},
		{
			name:           "right-hand weapon item resolves its type and reuse delay",
			rightHand:      bowItemID,
			items:          items,
			wantAttackType: item.WeaponBow,
			wantReuseDelay: bowReuseMillis * time.Millisecond,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", RightHand: tc.rightHand})
			h.SetWeapon(tc.items)

			if got := h.AttackType(); got != tc.wantAttackType {
				t.Fatalf("AttackType() = %v, want %v", got, tc.wantAttackType)
			}
			if got := h.WeaponReuseDelay(); got != tc.wantReuseDelay {
				t.Fatalf("WeaponReuseDelay() = %v, want %v", got, tc.wantReuseDelay)
			}
		})
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

func TestHostileDieBroadcastsDieToKnownReceivers(t *testing.T) {
	state := world.New()
	victim := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", HPMax: 10})
	victim.SetWorld(state)
	state.Spawn(victim, 100, 100, 0, 0)

	observer := &frameReceiver{trackedID: 55}
	state.Spawn(observer, 100, 100, 0, 0)

	if !victim.Die(&hostileTarget{id: 2}, nil) {
		t.Fatal("Die() = false on a live target, want true")
	}

	if len(observer.frames) != 1 {
		t.Fatalf("observer received %d frames, want 1", len(observer.frames))
	}
	if observer.frames[0][0] != serverpackets.OpcodeDie {
		t.Fatalf("frame opcode = %#x, want %#x", observer.frames[0][0], serverpackets.OpcodeDie)
	}

	// A repeated kill is a no-op per Die's once-only contract: no second
	// Die packet.
	if victim.Die(&hostileTarget{id: 2}, nil) {
		t.Fatal("Die() = true on an already-dead target, want false")
	}
	if len(observer.frames) != 1 {
		t.Fatalf("observer received %d frames after a repeat kill, want still 1", len(observer.frames))
	}
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

// fakeHostileLineOfSight is a LineOfSight double that records the query it
// received and returns a fixed result.
type fakeHostileLineOfSight struct {
	result bool
	got    struct {
		ox, oy, oz       int
		oCollisionHeight float64
		tx, ty, tz       int
		tCollisionHeight float64
	}
}

func (f *fakeHostileLineOfSight) CanSeeActor(ox, oy, oz int, oCollisionHeight float64, tx, ty, tz int, tCollisionHeight float64) bool {
	f.got.ox, f.got.oy, f.got.oz, f.got.oCollisionHeight = ox, oy, oz, oCollisionHeight
	f.got.tx, f.got.ty, f.got.tz, f.got.tCollisionHeight = tx, ty, tz, tCollisionHeight
	return f.result
}

func TestHostileCanSeeDefaultsToVisibleWithoutLineOfSight(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", CollisionHeight: 30})
	target := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", CollisionHeight: 30})

	if !h.CanSee(target) {
		t.Fatal("CanSee() = false with no line-of-sight query attached, want true")
	}
}

func TestHostileCanSeeQueriesLineOfSightWithActorHeights(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", CollisionHeight: 30})
	target := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", CollisionHeight: 40})

	los := &fakeHostileLineOfSight{result: false}
	h.SetLineOfSight(los)

	if got := h.CanSee(target); got != false {
		t.Fatalf("CanSee() = %v, want false (from line-of-sight query result)", got)
	}

	ox, oy, oz := h.Position()
	tx, ty, tz := target.Position()
	if los.got.ox != ox || los.got.oy != oy || los.got.oz != oz {
		t.Fatalf("CanSeeActor() origin = (%d,%d,%d), want (%d,%d,%d)", los.got.ox, los.got.oy, los.got.oz, ox, oy, oz)
	}
	if los.got.tx != tx || los.got.ty != ty || los.got.tz != tz {
		t.Fatalf("CanSeeActor() target = (%d,%d,%d), want (%d,%d,%d)", los.got.tx, los.got.ty, los.got.tz, tx, ty, tz)
	}
	if los.got.oCollisionHeight != h.CollisionHeight() {
		t.Fatalf("CanSeeActor() origin collision height = %v, want %v", los.got.oCollisionHeight, h.CollisionHeight())
	}
	if los.got.tCollisionHeight != target.CollisionHeight() {
		t.Fatalf("CanSeeActor() target collision height = %v, want %v", los.got.tCollisionHeight, target.CollisionHeight())
	}
}
