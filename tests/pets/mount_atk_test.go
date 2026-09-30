package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// riderRideAtk is a pAtkOnRide / mAtkOnRide the shipped wyvern never has
// (its rows all carry 0.0): the level 20 Wind Strider's 23.5759209699509
// (aCis_datapack/data/xml/npcs/12000-12999.xml, npc 12526), lent to the
// fixture wyvern so the UserInfo shows a base that came from the pet data.
const riderRideAtk = 23.5759209699509

// userInfoAtk is the P.Atk., M.Atk. and cast speed fields of a UserInfo
// frame.
type userInfoAtk struct{ pAtk, mAtk, castSpd int32 }

func decodeUserInfoAtk(t *testing.T, frame []byte) userInfoAtk {
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
	var s userInfoAtk
	s.pAtk = r.ReadInt32()
	for range 5 { // P.Atk. speed, P.Def., evasion, accuracy, critical
		r.ReadInt32()
	}
	s.mAtk, s.castSpd = r.ReadInt32(), r.ReadInt32()
	if err := r.Err(); err != nil {
		t.Fatalf("read UserInfo: %v", err)
	}
	return s
}

// TestRiderUserInfoCarriesTheMountsAtk pins PlayerStatus.getPAtk, getMAtk
// and getMAtkSpd (PlayerStatus.java:985-1031) through the UserInfo a wyvern
// mount sends (UserInfo.java: P.Atk., M.Atk. and cast speed fields): the
// rider's P.Atk. and M.Atk. are its mount's pAtkOnRide / mAtkOnRide through
// POWER_ATTACK and MAGIC_ATTACK (a collar mount takes its rider's level, so
// no level-gap reduction), not its class and weapon; its cast speed is the
// fed 333 base. Once the mount turns hungry the cast speed base halves with
// no packet, as setCurrentFeed sends only the gauge.
func TestRiderUserInfoCarriesTheMountsAtk(t *testing.T) {
	t.Parallel()
	wyvern := fedWyvernTemplate()
	row := wyvern.Pet.Levels[1]
	row.MountPAtk, row.MountMAtk = riderRideAtk, riderRideAtk
	wyvern.Pet.Levels[1] = row
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolfTemplate(), treeTemplate(), wyvern})),
		gameservertest.WithRestartPoints(mountRestartTable()),
	}, seedItem{TemplateID: wyvernCollarID, Count: 1})
	if !h.srv.DrivesClock() {
		t.Skip("counting 10-second feed ticks needs the driven clock")
	}
	h.client.Send(encodeUseItem(h.seededItem(t, wyvernCollarID), false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeRide, "mount Ride")
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeUserInfo, "mounted UserInfo")
	mounted := decodeUserInfoAtk(t, frames[len(frames)-1])

	rider := h.character(t)
	lvlMod := (100.0 - 11 + float64(rider.Level())) / 100.0
	intMod := statbonus.INTBonus[rider.INT()]
	wit := statbonus.WITBonus[rider.WIT()]
	want := userInfoAtk{
		pAtk:    int32(riderRideAtk * statbonus.STRBonus[rider.STR()] * lvlMod),
		mAtk:    int32(riderRideAtk * ((lvlMod * lvlMod) * (intMod * intMod))),
		castSpd: int32(333 * wit),
	}
	if mounted != want {
		t.Fatalf("mounted UserInfo P.Atk./M.Atk./cast speed = %+v, want %+v", mounted, want)
	}
	drainFrames(t, h.client)

	// 508 - 10n < 508 * 0.5 = 254 first holds at n = 26 (meal 248).
	for _, f := range h.advanceTicks(t, 26) {
		if f[0] == serverpackets.OpcodeUserInfo {
			t.Fatal("the feed ticks up to hunger sent UserInfo")
		}
	}
	if got, want := rider.MagicAttackSpeed(), int(166.5*wit); got != want {
		t.Fatalf("hungry mount cast speed = %d, want %d (half the 333 base)", got, want)
	}
}
