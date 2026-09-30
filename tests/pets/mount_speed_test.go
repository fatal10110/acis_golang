package pets

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// riderUserInfo is the speed block of a UserInfo frame.
type riderUserInfo struct {
	atkSpd                  int32
	run, walk, swim         int32
	flyRun, flyWalk         int32
	moveMult, attackSpdMult float64
}

func decodeRiderUserInfo(t *testing.T, frame []byte) riderUserInfo {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeUserInfo, "UserInfo")
	r := wire.NewReader(frame[1:])
	for range 5 { // x, y, z, heading, object id
		r.ReadInt32()
	}
	r.ReadString()
	for range 4 { // race, sex, class, level
		r.ReadInt32()
	}
	r.ReadInt64()
	for range 6 + 8 + 2*17 { // STR..MEN, vitals/SP/weight/slots, paperdoll ids
		r.ReadInt32()
	}
	for _, n := range []int{14, 12, 4} { // augmentation pairs, with the hand ids between
		for range n {
			r.ReadUint16()
		}
		if n != 4 {
			r.ReadInt32()
		}
	}
	var s riderUserInfo
	r.ReadInt32() // P.Atk.
	s.atkSpd = r.ReadInt32()
	for range 10 { // P.Def. .. karma
		r.ReadInt32()
	}
	s.run, s.walk, s.swim = r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	for range 3 { // swim again, unused x2
		r.ReadInt32()
	}
	s.flyRun, s.flyWalk = r.ReadInt32(), r.ReadInt32()
	s.moveMult, s.attackSpdMult = r.ReadFloat64(), r.ReadFloat64()
	if err := r.Err(); err != nil {
		t.Fatalf("read UserInfo: %v", err)
	}
	return s
}

// TestWyvernRiderMovesAtTheWyvernsSpeed pins PlayerStatus.getBaseRunSpeed,
// getBaseSwimSpeed and getPAtkSpd for a wyvern rider through the mount's
// UserInfo (UserInfo.java:134-153): the base run speed is the wyvern's fly
// speed, the swim speeds its water speed, the flying run/walk pair the fly
// and class walk speeds, P.Atk. speed a flat 300. The server moves the
// rider at the move speed the client derives from them. Once the wyvern
// falls below hungryLimit of its max meal the server halves the rider's
// speed with no packet, as setCurrentFeed sends only the gauge.
func TestWyvernRiderMovesAtTheWyvernsSpeed(t *testing.T) {
	t.Parallel()
	h, mountFrames := bootWyvernRider(t)
	mounted := decodeRiderUserInfo(t, mountFrames[len(mountFrames)-1])
	if mounted.run != wyvernFlySpeed || mounted.swim != wyvernWaterSpeed {
		t.Fatalf("mounted UserInfo run/swim = %d/%d, want the wyvern's %d/%d", mounted.run, mounted.swim, wyvernFlySpeed, wyvernWaterSpeed)
	}
	if mounted.flyRun != mounted.run || mounted.flyWalk != mounted.walk {
		t.Fatalf("mounted UserInfo fly run/walk = %d/%d, want the run/walk pair %d/%d", mounted.flyRun, mounted.flyWalk, mounted.run, mounted.walk)
	}
	if mounted.atkSpd != 300 || mounted.attackSpdMult != float64(float32(1.1)) {
		t.Fatalf("mounted UserInfo P.Atk. speed = %d (multiplier %v), want 300 (1.1)", mounted.atkSpd, mounted.attackSpdMult)
	}
	drainFrames(t, h.client)

	rider := h.character(t)
	speed := rider.Move().Speed()
	if want := float64(float32(float64(wyvernFlySpeed) * mounted.moveMult)); math.Abs(speed-want) > 1e-3 {
		t.Fatalf("simulated move speed = %v, want %v (the fly speed scaled by the UserInfo multiplier)", speed, want)
	}
	// The server lands a walk once its distance is covered at speed/10 a
	// 100 ms tick.
	const walk = 1000
	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	h.client.Send(encodeMoveBackwardToLocation(int32(x+walk), int32(y), int32(z)))
	h.handled(t)
	wantTicks := int(math.Ceil(walk / (speed / 10)))
	ticks := 0
	for ; ticks <= wantTicks+1; ticks++ {
		if ox, _, _ := h.srv.PlayerPosition(t, h.ownerID); ox == x+walk {
			break
		}
		h.srv.Advance(t, 100*time.Millisecond)
	}
	if ticks < wantTicks-1 || ticks > wantTicks+1 {
		t.Fatalf("rider covered %d units in %d move ticks, want about %d at speed %v", walk, ticks, wantTicks, speed)
	}
	drainFrames(t, h.client)

	// 508 - 10n < 508 * 0.5 = 254 first holds at n = 26 (meal 248); the
	// rider carries no food, so nothing refills it.
	const hungryTick = 26
	for _, f := range h.advanceTicks(t, hungryTick-1) {
		if f[0] == serverpackets.OpcodeUserInfo {
			t.Fatal("feed ticks before hunger sent UserInfo")
		}
	}
	if got := rider.Move().Speed(); got != speed {
		t.Fatalf("move speed at meal 258 = %v, want the fed %v", got, speed)
	}
	for _, f := range h.advanceTicks(t, 1) {
		if f[0] == serverpackets.OpcodeUserInfo {
			t.Fatal("the tick that made the wyvern hungry sent UserInfo")
		}
	}
	if got, want := rider.Move().Speed(), speed/2; math.Abs(got-want) > 1e-3 {
		t.Fatalf("hungry move speed = %v, want %v (half the fly speed)", got, want)
	}
	if got := rider.AttackSpeed(); got != 150 {
		t.Fatalf("hungry P.Atk. speed = %d, want 150", got)
	}
}
