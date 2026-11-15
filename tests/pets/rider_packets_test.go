package pets

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// bodyFloats is the run UserInfo and CharInfo write after the speeds: the
// movement and attack speed multipliers, then the collision radius and
// height.
func bodyFloats(c *player.Character, radius, height float64) []byte {
	var b []byte
	for _, v := range []float64{float64(c.MovementSpeedMultiplier()), float64(c.AttackSpeedMultiplier()), radius, height} {
		b = binary.LittleEndian.AppendUint64(b, math.Float64bits(v))
	}
	return b
}

// userInfoBonusSlots reads the bonus-slot word UserInfo writes after the
// weight limit.
func userInfoBonusSlots(t *testing.T, frame []byte) int32 {
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
	for range 6 + 7 { // STR..MEN, HP/MP/SP/weight/weight limit
		r.ReadInt32()
	}
	slots := r.ReadInt32()
	if err := r.Err(); err != nil {
		t.Fatalf("read UserInfo: %v", err)
	}
	return slots
}

// TestWyvernRiderFramesCarryTheWyvernsBody mounts an unarmed owner on a
// wyvern next to an observer: the rider's UserInfo and the observer's
// CharInfo after the Ride both write the wyvern's collision radius and
// height, not the rider's own body, and every UserInfo of the unarmed rider
// reports 40 bonus slots.
func TestWyvernRiderFramesCarryTheWyvernsBody(t *testing.T) {
	t.Parallel()
	h := bootWyvernOwner(t)
	observer := h.joinSecondPlayer(t, "Watcher")
	rider := h.character(t)
	ownRadius, ownHeight := rider.CollisionRadius(), rider.CollisionHeight()
	if ownRadius == wyvernRadius || ownHeight == wyvernHeight {
		t.Fatalf("unmounted body = %v/%v, want it to differ from the wyvern's", ownRadius, ownHeight)
	}

	h.client.Send(encodeUseItem(h.seededItem(t, wyvernCollarID), false))
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeSetupGauge, "feed gauge")
	var userInfos [][]byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeUserInfo {
			userInfos = append(userInfos, f)
		}
	}
	if len(userInfos) == 0 {
		t.Fatal("mount sent no UserInfo")
	}
	for i, f := range userInfos {
		if got := userInfoBonusSlots(t, f); got != 40 {
			t.Fatalf("UserInfo %d of the unarmed rider: bonus slots = %d, want 40", i, got)
		}
	}
	mounted := bodyFloats(rider, wyvernRadius, wyvernHeight)
	if !bytes.Contains(userInfos[len(userInfos)-1], mounted) {
		t.Fatalf("mounted UserInfo does not carry the wyvern body %v/%v", wyvernRadius, wyvernHeight)
	}

	readUntilOpcode(t, observer.client, serverpackets.OpcodeRide, "observer Ride")
	seen := readUntilOpcode(t, observer.client, serverpackets.OpcodeCharInfo, "observer CharInfo after the Ride")
	charInfo := seen[len(seen)-1]
	if !bytes.Contains(charInfo, mounted) {
		t.Fatalf("observer CharInfo does not carry the wyvern body %v/%v", wyvernRadius, wyvernHeight)
	}
	if bytes.Contains(charInfo, bodyFloats(rider, ownRadius, ownHeight)) {
		t.Fatal("observer CharInfo still carries the rider's own body")
	}
}
