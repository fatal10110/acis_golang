package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// petInfoStats is the part of a PetInfo a stat change moves.
type petInfoStats struct {
	pAtkSpd       int32
	atkMultiplier float64
	pAtk          int32
}

// readPetInfoStats returns PetInfo's P.Atk. speed, attack speed multiplier
// and P.Atk.
func readPetInfoStats(t *testing.T, frame []byte) petInfoStats {
	t.Helper()
	r := wire.NewReader(frame[1:])
	for range 10 { // type, id, template, attackable, x, y, z, heading, 0, cast speed
		r.ReadInt32()
	}
	var got petInfoStats
	got.pAtkSpd = r.ReadInt32()
	for range 8 { // run/walk speed pairs
		r.ReadInt32()
	}
	r.ReadFloat64() // movement speed multiplier
	got.atkMultiplier = r.ReadFloat64()
	r.ReadFloat64() // collision radius
	r.ReadFloat64() // collision height
	for range 3 {
		r.ReadInt32()
	}
	for range 5 {
		r.ReadUint8()
	}
	r.ReadString() // name
	r.ReadString() // title
	for range 11 { // 1, pvp flag, karma, fed/max, hp/max, mp/max, sp, level
		r.ReadInt32()
	}
	for range 3 { // exp, this-level exp, next-level exp
		r.ReadInt64()
	}
	r.ReadInt32() // weight
	r.ReadInt32() // weight limit
	got.pAtk = r.ReadInt32()
	if err := r.Err(); err != nil {
		t.Fatalf("read PetInfo stats: %v", err)
	}
	return got
}

// readNpcInfoAttackSpeed returns NpcInfo's P.Atk. speed and attack speed
// multiplier.
func readNpcInfoAttackSpeed(t *testing.T, frame []byte) (pAtkSpd int32, multiplier float64) {
	t.Helper()
	r := wire.NewReader(frame[1:])
	for range 9 { // id, template, attackable, x, y, z, heading, 0, cast speed
		r.ReadInt32()
	}
	pAtkSpd = r.ReadInt32()
	for range 8 { // run/walk speed pairs
		r.ReadInt32()
	}
	r.ReadFloat64() // movement speed multiplier
	multiplier = r.ReadFloat64()
	if err := r.Err(); err != nil {
		t.Fatalf("read NpcInfo attack speed: %v", err)
	}
	return pAtkSpd, multiplier
}

// requireRepublished checks one stat change's packets: the owner gets a
// PetInfo for the pet and then a PetStatusUpdate, the observer gets an
// NpcInfo for it and no PetInfo. It returns the owner's PetInfo and the
// observer's NpcInfo.
func requireRepublished(t *testing.T, what string, owner, observer [][]byte, petID int32) (petInfo, npcInfo []byte) {
	t.Helper()
	info, status := -1, -1
	for i, frame := range owner {
		switch {
		case frame[0] == serverpackets.OpcodePetInfo && info < 0 && wire.NewReader(frame[5:]).ReadInt32() == petID:
			info = i
		case frame[0] == serverpackets.OpcodePetStatusUpdate && status < 0:
			status = i
		}
	}
	if info < 0 || status < info {
		t.Fatalf("%s: owner opcodes %x, want the pet's PetInfo, then PetStatusUpdate", what, frameOpcodes(owner))
	}
	if _, ok := firstOpcode(observer, serverpackets.OpcodePetInfo); ok {
		t.Fatalf("%s: observer got a PetInfo, which only the owner may", what)
	}
	for _, frame := range observer {
		if frame[0] == serverpackets.OpcodeNPCInfo && wire.NewReader(frame[1:]).ReadInt32() == petID {
			npcInfo = frame
		}
	}
	if npcInfo == nil {
		t.Fatalf("%s: observer opcodes %x, want the pet's NpcInfo", what, frameOpcodes(observer))
	}
	return owner[info], npcInfo
}

// addToPet adds e to the pet's effect list from its own queue.
func addToPet(t *testing.T, petActor *summon.Actor, e *effect.Effect) {
	t.Helper()
	onPetQueue(t, petActor, func() { petActor.EffectList().Add(e) })
}

// TestPAtkBuffRepublishesPetInfoAndSummonInfo lands a P.Atk. buff on a pet
// with a second player in view. Any stat func added to or removed from a
// summon with an owner republishes it (Creature.broadcastModifiedStats ->
// Summon.updateAndBroadcastStatusAndInfos): PetInfo to the owner, then
// PetStatusUpdate, then NpcInfo to every other player that knows it. The
// PetInfo carries the doubled P.Atk., and removing the buff sends it all
// again with the base P.Atk.
func TestPAtkBuffRepublishesPetInfoAndSummonInfo(t *testing.T) {
	t.Parallel()
	h, petActor, _ := bootWolfStriker(t)
	observer := h.joinSecondPlayer(t, "Watcher")
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, observer.client)

	base := int32(petActor.PAtk())
	might, err := effect.New(effect.Skill{ID: 1068, Level: 1}, modelskill.EffectTemplate{
		Name: "Buff", Time: 30, StackType: "pa_up", StackOrder: 1,
		Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncMul, Stat: "pAtk", Value: 2}},
	})
	if err != nil {
		t.Fatalf("effect.New(might): %v", err)
	}
	might.Effector, might.Effected = petActor, petActor

	addToPet(t, petActor, might)
	info, _ := requireRepublished(t, "buff landing", drainFrames(t, h.client), drainFrames(t, observer.client), petActor.ObjectID())
	if got := readPetInfoStats(t, info).pAtk; got < 2*base || got > 2*base+1 {
		t.Fatalf("buffed PetInfo P.Atk. = %d, want twice the base %d", got, base)
	}

	onPetQueue(t, petActor, func() { petActor.EffectList().Remove(might) })
	info, _ = requireRepublished(t, "buff ending", drainFrames(t, h.client), drainFrames(t, observer.client), petActor.ObjectID())
	if got := readPetInfoStats(t, info).pAtk; got != base {
		t.Fatalf("PetInfo P.Atk. after the buff = %d, want the base %d", got, base)
	}
}

// TestAttackSpeedDebuffMovesPetAttackSpeedMultiplier pins the attack speed
// multiplier a summon is shown with, in its owner's PetInfo and its
// observers' NpcInfo: (float) (1.1 * P.Atk.Spd / template base)
// (CreatureStatus.getAttackSpeedMultiplier). The wolf's 300 base times its
// DEX 30 bonus 1.1 is 330 (multiplier 1.21); a halving debuff takes it to
// 165 (0.605), and the republish the debuff triggers carries it.
func TestAttackSpeedDebuffMovesPetAttackSpeedMultiplier(t *testing.T) {
	t.Parallel()
	h, petActor, hostile := bootWolfStriker(t)
	observer := h.joinSecondPlayer(t, "Watcher")
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, observer.client)

	multiplier := func(pAtkSpd, base float64) float64 { return float64(float32(1.1 * pAtkSpd / base)) }
	require := func(what string, wantSpd int32) {
		t.Helper()
		info, npcInfo := requireRepublished(t, what, drainFrames(t, h.client), drainFrames(t, observer.client), petActor.ObjectID())
		want := multiplier(float64(wantSpd), 300)
		if got := readPetInfoStats(t, info); got.pAtkSpd != wantSpd || got.atkMultiplier != want {
			t.Fatalf("%s: PetInfo P.Atk.Spd/multiplier = %d/%v, want %d/%v", what, got.pAtkSpd, got.atkMultiplier, wantSpd, want)
		}
		if spd, got := readNpcInfoAttackSpeed(t, npcInfo); spd != wantSpd || got != want {
			t.Fatalf("%s: NpcInfo P.Atk.Spd/multiplier = %d/%v, want %d/%v", what, spd, got, wantSpd, want)
		}
	}

	slow, err := effect.New(effect.Skill{ID: 1160, Level: 1, Debuff: true}, modelskill.EffectTemplate{
		Name: "Debuff", Time: 30, StackType: "attack_time_up", StackOrder: 1,
		Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncMul, Stat: "pAtkSpd", Value: 0.5}},
	})
	if err != nil {
		t.Fatalf("effect.New(slow): %v", err)
	}
	slow.Effector, slow.Effected = hostile, petActor
	addToPet(t, petActor, slow)
	require("debuff landing", 165)

	onPetQueue(t, petActor, func() { petActor.EffectList().Remove(slow) })
	require("debuff ending", 330)
}
