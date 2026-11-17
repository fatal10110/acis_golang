package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestShadowSenseReappliesAtEachDayNightCrossingInWorld crosses the day/night
// boundary twice while the dark elf is in the world: each crossing announces
// the new state once, night first or day first as the clock stands.
func TestShadowSenseReappliesAtEachDayNightCrossingInWorld(t *testing.T) {
	t.Parallel()
	srv, c := bootShadowSensor(t)
	selectShadowSensor(t, c)
	c.Send(encodeEnterWorld())
	readUntilQuiet(t, c)

	for crossing := range 2 {
		night := srv.CrossDayNight(t)
		srv.Settle(t)
		want := int32(serverpackets.SystemMessageDaySkillEffectDisappears)
		if night {
			want = serverpackets.SystemMessageNightSkillEffectApplies
		}
		// A crossing more than 720 game minutes in also carries the activity
		// reminder; it is not what this test is about.
		var got []int32
		for _, frame := range readUntilQuiet(t, c) {
			if frame[0] != serverpackets.OpcodeSystemMessage {
				continue
			}
			r := wireReader(frame[1:])
			id := r.ReadInt32()
			if id == serverpackets.SystemMessagePlayingForLongTime {
				continue
			}
			got = append(got, id)
			// The announcement names Shadow Sense: one skill-name parameter
			// carrying skill 294 at level 1.
			if params, kind, skill, level := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); params != 1 || kind != serverpackets.SystemMessageParamSkillName || skill != 294 || level != 1 {
				t.Errorf("crossing %d message %d params = (count %d, type %d, skill %d, level %d), want one skill-name parameter 294/1", crossing+1, id, params, kind, skill, level)
			}
		}
		if len(got) != 1 || got[0] != want {
			t.Fatalf("crossing %d (night=%v) system messages = %v, want exactly [%d]", crossing+1, night, got, want)
		}
	}
}

// TestPlayingForLongTimeEvery720GameMinutes pins the activity reminder: it
// arrives on the 720th in-game minute after the player entered the world and
// again on every 720th after that, and on no other minute.
func TestPlayingForLongTimeEvery720GameMinutes(t *testing.T) {
	t.Parallel()
	srv, c := bootShadowSensor(t)
	selectShadowSensor(t, c)
	c.Send(encodeEnterWorld())
	readUntilQuiet(t, c)

	reminders := func() int {
		srv.Settle(t)
		n := 0
		for _, id := range systemMessageIDs(readUntilQuiet(t, c)) {
			if id == serverpackets.SystemMessagePlayingForLongTime {
				n++
			}
		}
		return n
	}
	for round := 1; round <= 2; round++ {
		srv.TickGameMinutes(t, 719)
		if n := reminders(); n != 0 {
			t.Fatalf("round %d: %d reminders before the 720th game minute, want 0", round, n)
		}
		srv.TickGameMinutes(t, 1)
		if n := reminders(); n != 1 {
			t.Fatalf("round %d: %d reminders on the 720th game minute, want 1", round, n)
		}
	}
}
