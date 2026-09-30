package skills

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// charInfoSpeeds reads a CharInfo frame's object id, template run speed and
// movement speed multiplier (CharInfo.java:42-112).
func charInfoSpeeds(t *testing.T, frame []byte) (objectID, runSpd int32, moveMult float64) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeCharInfo, "CharInfo")
	r := wire.NewReader(frame[1:])
	for range 4 { // x, y, z, boat
		r.ReadInt32()
	}
	objectID = r.ReadInt32()
	r.ReadString()
	for range 3 + 12 { // race, sex, class, paperdoll ids
		r.ReadInt32()
	}
	for i, n := range []int{4, 12, 4} { // augmentation pairs, with the hand ids between
		for range n {
			r.ReadUint16()
		}
		if i < 2 {
			r.ReadInt32()
		}
	}
	for range 6 { // PvP flag, karma, cast and attack speed, PvP flag, karma
		r.ReadInt32()
	}
	runSpd = r.ReadInt32()
	for range 7 { // walk, swim x2, run/walk again, fly x2
		r.ReadInt32()
	}
	moveMult = r.ReadFloat64()
	if err := r.Err(); err != nil {
		t.Fatalf("decode CharInfo speeds: %v", err)
	}
	return objectID, runSpd, moveMult
}

// TestRunSpeedBuffOnWatchedPlayerCompletes pins #2854: a Buff whose stat
// func changes RUN_SPEED landing on a player another player can see used to
// hang the player's queue, because the appearance refresh the change
// triggers (Creature.broadcastModifiedStats -> updateAndBroadcastStatus(2),
// Creature.java:1208-1253) read the effect list under its own lock. The add
// completes, the Watcher gets the Target's CharInfo with the raised move
// speed, and the buff wearing off sends the plain speed again.
func TestRunSpeedBuffOnWatchedPlayerCompletes(t *testing.T) {
	t.Parallel()
	const mul = 1.5
	p := bootAppearancePair(t)

	e, err := effect.New(effect.Skill{ID: appearanceSkillID, Level: 1}, modelskill.EffectTemplate{
		Name: "Buff", Count: 1, Time: 1, Icon: true, StackType: "speed_up", StackOrder: 1,
		Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncMul, Stat: "runSpd", Value: mul}},
	})
	if err != nil {
		t.Fatalf("effect.New(Buff): %v", err)
	}
	e.Effector, e.Effected = p.target, p.target
	done := make(chan struct{})
	if !p.target.Queue().Post(func() { p.target.EffectList().Add(e); close(done) }) {
		t.Fatal("post to target queue: queue closed")
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("run-speed buff on a watched player did not complete: the Target's queue is hung")
	}
	buffed := p.watcherCharInfoMoveMult(t, "buff landed")

	p.expire(t, e, "run-speed buff wearing off")
	plain := p.watcherCharInfoMoveMult(t, "buff wore off")

	if plain < 0.5 || plain > 2 {
		t.Fatalf("plain move multiplier = %v, want a live multiplier near 1 (speed block misread?)", plain)
	}
	if want := plain * mul; math.Abs(buffed-want) > 1e-5 {
		t.Fatalf("Watcher's CharInfo move multiplier = %v with the buff, want %v (plain %v x %v)", buffed, want, plain, mul)
	}
}

// watcherCharInfoMoveMult reads the Watcher's frames until quiet and returns
// the move multiplier of the one CharInfo of the Target among them.
func (p *appearancePair) watcherCharInfoMoveMult(t *testing.T, when string) float64 {
	t.Helper()
	seen := readQuiet(t, p.wc)
	var mults []float64
	for _, f := range framesWithOpcode(seen, serverpackets.OpcodeCharInfo) {
		if id, _, mult := charInfoSpeeds(t, f); id == p.targetID {
			mults = append(mults, mult)
		}
	}
	if len(mults) != 1 {
		t.Fatalf("%s: Watcher got %d CharInfo frames of the Target, want 1 (opcodes % x)", when, len(mults), opcodeList(seen))
	}
	return mults[0]
}
